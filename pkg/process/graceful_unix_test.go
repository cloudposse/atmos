//go:build !windows

package process

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/signals"
)

// runGracefulFamily starts a grouped graceful child with a grandchild and delivers sig as the first signal.
func runGracefulFamily(t *testing.T, sig os.Signal) (string, Result) {
	t.Helper()
	stops := useGracefulStops(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "child", "grandchild")
	done := runAsync(context.Background(), gracefulSpec(t, modeGracefulFamily, dir))
	child := waitForPID(t, dir, "child")
	waitForPID(t, dir, "grandchild")
	require.NotEqual(t, currentProcessGroup(), processGroupOf(t, child), "prerequisite: the child leads its own group")

	beginOnceRegistered(t, stops, sig)
	res := awaitRun(t, done)
	require.FileExists(t, filepath.Join(dir, "done"), "the child must finish saving state")
	return dir, res
}

func TestGracefulRunForwardsCtrlCOnceToTheChildGroup(t *testing.T) {
	dir, res := runGracefulFamily(t, os.Interrupt)
	assert.True(t, res.Success(), "the child exits on its own after the interrupt: %+v", res)
	assert.False(t, res.Canceled)
	assert.Equal(t, []string{"interrupt"}, recordedSignals(dir, "child"), "exactly one SIGINT reaches the child")
	assert.Equal(t, []string{"interrupt"}, recordedSignals(dir, "grandchild"),
		"Ctrl-C reaches the whole group, as a terminal would deliver it")
	assert.Empty(t, liveChildren())
}

func TestGracefulRunForwardsSIGTERMOnlyToTheChild(t *testing.T) {
	dir, res := runGracefulFamily(t, syscall.SIGTERM)
	assert.True(t, res.Success(), "the child exits on its own after SIGTERM: %+v", res)
	assert.Equal(t, []string{"terminated"}, recordedSignals(dir, "child"), "exactly one SIGTERM reaches the child")
	assert.Empty(t, recordedSignals(dir, "grandchild"),
		"SIGTERM must not reach provider plugins that Terraform still needs")
}

func TestGracefulRunDoesNotRepeatTheTerminalCtrlC(t *testing.T) {
	stops := useGracefulStops(t)
	original := stdinIsTerminal
	stdinIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinIsTerminal = original })
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	done := runAsync(context.Background(), gracefulSpec(t, modeGracefulTrap, dir))
	child := waitForPID(t, dir, "child")
	require.Equal(t, currentProcessGroup(), processGroupOf(t, child),
		"prerequisite: the child shares the foreground group")

	beginOnceRegistered(t, stops, os.Interrupt)
	requireStillRunning(t, done, "the terminal, not Atmos, delivers Ctrl-C to a foreground child")
	assert.Empty(t, recordedSignals(dir, "child"))

	// Simulate the terminal's own delivery to the foreground group.
	require.NoError(t, syscall.Kill(child, syscall.SIGINT))
	res := awaitRun(t, done)
	assert.True(t, res.Success(), "%+v", res)
	assert.Equal(t, []string{"interrupt"}, recordedSignals(dir, "child"), "a second SIGINT would make Terraform abort")
}

func TestGracefulRunForwardsSIGTERMToAForegroundChild(t *testing.T) {
	stops := useGracefulStops(t)
	original := stdinIsTerminal
	stdinIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinIsTerminal = original })
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	done := runAsync(context.Background(), gracefulSpec(t, modeGracefulTrap, dir))
	waitForPID(t, dir, "child")

	// SIGTERM sent to the Atmos process alone never reaches the child unless it is forwarded.
	beginOnceRegistered(t, stops, syscall.SIGTERM)
	res := awaitRun(t, done)
	assert.True(t, res.Success(), "%+v", res)
	assert.Equal(t, []string{"terminated"}, recordedSignals(dir, "child"))
}

func TestGracefulRunSecondSignalForcesTermination(t *testing.T) {
	stops := useGracefulStops(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	done := runAsync(context.Background(), gracefulSpec(t, modeGracefulStuck, dir))
	child := waitForPID(t, dir, "child")

	beginOnceRegistered(t, stops, os.Interrupt)
	require.Eventually(t, func() bool { return len(recordedSignals(dir, "child")) == 1 }, pidWait, 10*time.Millisecond)
	requireStillRunning(t, done, "the first signal must not kill the child")

	// The main signal handler forces the exit on the second signal: the registry stops
	// waiting and the exit cleanups terminate the child's group as before.
	_, ok := stops.Begin(os.Interrupt)
	require.False(t, ok)
	stops.Wait()
	signals.RunExitCleanups()
	res := awaitRun(t, done)
	assert.False(t, res.Success())
	assert.True(t, res.Signaled, "the forced child is killed: %+v", res)
	requireGone(t, child, "forced child")
	assert.Empty(t, liveChildren())
}

func TestInterruptTarget(t *testing.T) {
	tests := []struct {
		name       string
		grouped    bool
		sig        os.Signal
		wantTarget int
		wantSignal syscall.Signal
		wantSend   bool
	}{
		{
			name: "grouped Ctrl-C goes to the group", grouped: true, sig: os.Interrupt,
			wantTarget: -42, wantSignal: syscall.SIGINT, wantSend: true,
		},
		{name: "foreground Ctrl-C is already delivered", grouped: false, sig: os.Interrupt, wantSend: false},
		{
			name: "grouped SIGTERM goes to the child only", grouped: true, sig: syscall.SIGTERM,
			wantTarget: 42, wantSignal: syscall.SIGTERM, wantSend: true,
		},
		{
			name: "foreground SIGTERM is forwarded", grouped: false, sig: syscall.SIGTERM,
			wantTarget: 42, wantSignal: syscall.SIGTERM, wantSend: true,
		},
		{
			name: "unknown signals are treated as Ctrl-C", grouped: true, sig: fakeSignal{},
			wantTarget: -42, wantSignal: syscall.SIGINT, wantSend: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, value, send := interruptTarget(42, tt.grouped, tt.sig)
			assert.Equal(t, tt.wantSend, send)
			assert.Equal(t, tt.wantTarget, target)
			assert.Equal(t, tt.wantSignal, value)
		})
	}
}

// fakeSignal is an os.Signal that is not a syscall.Signal.
type fakeSignal struct{}

func (fakeSignal) String() string { return "fake" }
func (fakeSignal) Signal()        {}
