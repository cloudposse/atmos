//go:build !windows

package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// startGracefulChild starts the test binary in mode as a GracefulChild, as the terminal UI does.
func startGracefulChild(
	t *testing.T, ctx context.Context, mode, dir string, notify func(os.Signal),
) (*GracefulChild, <-chan error) {
	t.Helper()
	child, err := NewGracefulChild(ctx)
	require.NoError(t, err)
	spec := helperSpec(t, mode, dir, nil)
	cmd := exec.CommandContext(child.Context(), spec.Command)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	require.NoError(t, child.Start(cmd, notify))
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		child.Finish()
		done <- err
	}()
	return child, done
}

// awaitChild waits for a GracefulChild's command to exit.
func awaitChild(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the graceful child did not exit")
		return nil
	}
}

// useTerminalStdin makes the next child look like it shares the terminal, as under the TUI.
func useTerminalStdin(t *testing.T) {
	t.Helper()
	original := stdinIsTerminal
	stdinIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinIsTerminal = original })
}

func TestGracefulChildInterruptDeliversCtrlCToATerminalChild(t *testing.T) {
	useGracefulStops(t)
	useTerminalStdin(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	child, done := startGracefulChild(t, context.Background(), modeGracefulTrap, dir, nil)
	pid := waitForPID(t, dir, "child")
	require.Equal(t, currentProcessGroup(), processGroupOf(t, pid),
		"prerequisite: the child shares the terminal's group")

	// In raw mode the terminal sends no SIGINT, so the keypress must be delivered by Atmos.
	require.NoError(t, child.Interrupt())
	require.NoError(t, awaitChild(t, done), "the child stops on its own after the interrupt")
	assert.Equal(t, []string{"interrupt"}, recordedSignals(dir, "child"))
	assert.FileExists(t, filepath.Join(dir, "done"), "the child must finish saving state")
}

func TestGracefulChildForwardsTheFirstSignalAndNotifies(t *testing.T) {
	tests := []struct {
		name     string
		terminal bool
		sig      os.Signal
		want     string
	}{
		{name: "Ctrl-C to a terminal child", terminal: true, sig: os.Interrupt, want: "interrupt"},
		{name: "SIGTERM to a terminal child", terminal: true, sig: syscall.SIGTERM, want: "terminated"},
		{name: "Ctrl-C to a grouped child", terminal: false, sig: os.Interrupt, want: "interrupt"},
		{name: "SIGTERM to a grouped child", terminal: false, sig: syscall.SIGTERM, want: "terminated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stops := useGracefulStops(t)
			if tt.terminal {
				useTerminalStdin(t)
			}
			dir := t.TempDir()
			killAtCleanup(t, dir, "child")
			notified := make(chan os.Signal, 1)
			_, done := startGracefulChild(t, context.Background(), modeGracefulTrap, dir, func(sig os.Signal) {
				notified <- sig
			})
			waitForPID(t, dir, "child")

			names := beginOnceRegistered(t, stops, tt.sig)
			assert.Len(t, names, 1)
			assert.Equal(t, tt.sig, <-notified, "the caller learns the signal was forwarded")
			require.NoError(t, awaitChild(t, done))
			assert.Equal(t, []string{tt.want}, recordedSignals(dir, "child"),
				"the child receives the signal exactly once")
			stops.Wait()
		})
	}
}

func TestGracefulChildIsNotCancelledBySignalContext(t *testing.T) {
	useGracefulStops(t)
	useTerminalStdin(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	ctx, cancel := context.WithCancelCause(context.Background())
	child, done := startGracefulChild(t, ctx, modeGracefulTrap, dir, nil)
	waitForPID(t, dir, "child")

	cancel(fmt.Errorf("%w: interrupt", errUtils.ErrSignalReceived))
	select {
	case err := <-done:
		t.Fatalf("a signal-caused cancellation must not kill the child: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, child.Interrupt())
	require.NoError(t, awaitChild(t, done))
}

func TestGracefulChildKillTerminatesAStuckChild(t *testing.T) {
	useGracefulStops(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	child, done := startGracefulChild(t, context.Background(), modeGracefulStuck, dir, nil)
	pid := waitForPID(t, dir, "child")

	require.NoError(t, child.Interrupt())
	require.Eventually(t, func() bool { return len(recordedSignals(dir, "child")) == 1 }, pidWait, 10*time.Millisecond)
	child.Kill()
	require.Error(t, awaitChild(t, done), "the killed child reports a failure")
	requireGone(t, pid, "killed child")
	assert.Empty(t, liveChildren(), "Finish leaves the exit-cleanup registry")
}

func TestGracefulChildTimeoutStillKills(t *testing.T) {
	useGracefulStops(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	ctx, cancel := context.WithCancel(context.Background())
	_, done := startGracefulChild(t, ctx, modeGracefulStuck, dir, nil)
	pid := waitForPID(t, dir, "child")

	cancel()
	require.Error(t, awaitChild(t, done), "a cancellation not caused by a signal terminates the child")
	requireGone(t, pid, "cancelled child")
}

func TestNewGracefulChildRefusesWhileStopping(t *testing.T) {
	stops := useGracefulStops(t)
	release := stops.Register("terraform", func(os.Signal) {})
	t.Cleanup(release)
	_, ok := stops.Begin(os.Interrupt)
	require.True(t, ok)

	child, err := NewGracefulChild(context.Background())
	require.ErrorIs(t, err, errUtils.ErrProcessInterrupted)
	assert.Nil(t, child)
}

func TestNewGracefulChildRefusesACancelledContext(t *testing.T) {
	useGracefulStops(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	child, err := NewGracefulChild(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, child)
}

func TestGracefulChildStartFailureLeavesNothingRegistered(t *testing.T) {
	stops := useGracefulStops(t)
	//nolint:staticcheck // A nil context must be accepted like Run accepts one.
	child, err := NewGracefulChild(nil)
	require.NoError(t, err)
	cmd := exec.CommandContext(child.Context(), filepath.Join(t.TempDir(), "missing-terraform"))

	require.Error(t, child.Start(cmd, nil))
	child.Finish()
	_, ok := stops.Begin(os.Interrupt)
	assert.False(t, ok, "a child that never started must not hold up a graceful stop")
	assert.ErrorIs(t, child.Context().Err(), context.Canceled)
}

func TestGracefulChildInterruptBeforeStart(t *testing.T) {
	useGracefulStops(t)
	child, err := NewGracefulChild(context.Background())
	require.NoError(t, err)
	t.Cleanup(child.Finish)

	assert.ErrorIs(t, child.Interrupt(), os.ErrProcessDone)
}

func TestTerminalInterruptTarget(t *testing.T) {
	tests := []struct {
		name       string
		grouped    bool
		sig        os.Signal
		wantTarget int
		wantSignal syscall.Signal
	}{
		{name: "terminal Ctrl-C goes to the child", sig: os.Interrupt, wantTarget: 42, wantSignal: syscall.SIGINT},
		{
			name: "grouped Ctrl-C goes to the group", grouped: true, sig: os.Interrupt,
			wantTarget: -42, wantSignal: syscall.SIGINT,
		},
		{name: "terminal SIGTERM goes to the child", sig: syscall.SIGTERM, wantTarget: 42, wantSignal: syscall.SIGTERM},
		{
			name: "grouped SIGTERM goes to the child only", grouped: true, sig: syscall.SIGTERM,
			wantTarget: 42, wantSignal: syscall.SIGTERM,
		},
		{name: "a non-syscall signal is an interrupt", sig: fakeSignal{}, wantTarget: 42, wantSignal: syscall.SIGINT},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, value := terminalInterruptTarget(42, tt.grouped, tt.sig)
			assert.Equal(t, tt.wantTarget, target)
			assert.Equal(t, tt.wantSignal, value)
		})
	}
}
