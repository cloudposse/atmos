package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Compile-time guard for the opt-in field the Terraform component sets.
var _ = TaskSpec{Interrupt: InterruptGraceful}

// signalCanceledContext returns a context canceled the way signals.InterruptContext cancels it.
func signalCanceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(fmt.Errorf("%w: interrupt", errUtils.ErrSignalReceived))
	return ctx
}

// inputSpec runs the stdin-draining helper, which exits 0 once the returned writer is closed.
func inputSpec(t *testing.T, dir string, policy InterruptPolicy) (*TaskSpec, io.WriteCloser) {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	spec := helperSpec(t, modeInput, dir, nil)
	spec.Streams.Stdin = reader
	spec.Interrupt = policy
	return &spec, writer
}

func requireStillRunning(t *testing.T, done <-chan runOutcome, why string) {
	t.Helper()
	select {
	case out := <-done:
		t.Fatalf("%s, but Run returned: %+v", why, out.result)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestGracefulRunRefusesToStartAfterStopBegan(t *testing.T) {
	stops := useGracefulStops(t)
	release := stops.Register("terraform", func(os.Signal) {})
	defer release()
	_, ok := stops.Begin(os.Interrupt)
	require.True(t, ok)

	dir := t.TempDir()
	res := NewDefaultRunner().Run(context.Background(), *gracefulSpec(t, modeSleep, dir))
	assert.False(t, res.Started, "no new Terraform command may start after the first signal")
	assert.ErrorIs(t, res.Err, errUtils.ErrProcessStartFailed)
	assert.ErrorIs(t, res.Err, errUtils.ErrProcessInterrupted)
	assert.Equal(t, -1, res.ExitCode)
	assert.Zero(t, readPID(dir, "child"))

	// Subprocesses that did not opt in keep their existing behavior.
	plain := helperSpec(t, modeInput, dir, nil)
	assert.True(t, NewDefaultRunner().Run(context.Background(), plain).Success())
}

func TestGracefulRunRefusesToStartOnCanceledContext(t *testing.T) {
	useGracefulStops(t)
	dir := t.TempDir()
	res := NewDefaultRunner().Run(signalCanceledContext(t), *gracefulSpec(t, modeSleep, dir))
	assert.False(t, res.Started, "a component scheduled after the interrupt must not start")
	assert.ErrorIs(t, res.Err, errUtils.ErrProcessStartFailed)
	assert.ErrorIs(t, res.Err, context.Canceled)
	assert.Zero(t, readPID(dir, "child"))
}

func TestGracefulRunWaitsForTheChildAfterTheFirstSignal(t *testing.T) {
	stops := useGracefulStops(t)
	// A terminal child received Ctrl-C from the terminal; Atmos only waits for it.
	original := stdinIsTerminal
	stdinIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinIsTerminal = original })
	dir := t.TempDir()
	spec, stdin := inputSpec(t, dir, InterruptGraceful)
	done := runAsync(context.Background(), spec)
	waitForPID(t, dir, "child")

	names := beginOnceRegistered(t, stops, os.Interrupt)
	require.Len(t, names, 1)
	assert.Equal(t, commandDisplayName(spec.Command), names[0])
	requireStillRunning(t, done, "Atmos must not kill a graceful child on the first signal")

	waited := make(chan struct{})
	go func() {
		stops.Wait()
		close(waited)
	}()
	require.NoError(t, stdin.Close())
	res := awaitRun(t, done)
	assert.True(t, res.Success(), "the child decides its own exit status: %+v", res)
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("the graceful stop did not finish after the child exited")
	}
	assert.Empty(t, liveChildren())
}

func TestDefaultRunDoesNotJoinGracefulStop(t *testing.T) {
	stops := useGracefulStops(t)
	dir := t.TempDir()
	spec, stdin := inputSpec(t, dir, InterruptKill)
	done := runAsync(context.Background(), spec)
	waitForPID(t, dir, "child")
	require.Eventually(t, func() bool { return len(liveChildren()) == 1 }, pidWait, 10*time.Millisecond)

	_, ok := stops.Begin(os.Interrupt)
	assert.False(t, ok, "a subprocess that did not opt in must not defer the exit")
	require.NoError(t, stdin.Close())
	assert.True(t, awaitRun(t, done).Success())
}

func TestGracefulRunStillKillsOnNonSignalCancellation(t *testing.T) {
	useGracefulStops(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	ctx, cancel := context.WithTimeout(context.Background(), 2*pidWait)
	defer cancel()
	done := runAsync(ctx, gracefulSpec(t, modeSleep, dir))
	waitForPID(t, dir, "child")

	cancel()
	res := awaitRun(t, done)
	assert.True(t, res.Canceled, "a timeout or caller cancellation still terminates the child")
	assert.False(t, res.Success())
	assert.ErrorIs(t, res.Err, context.Canceled)
	assert.Empty(t, liveChildren())
}

func TestGracefulRunIgnoresSignalCancellation(t *testing.T) {
	useGracefulStops(t)
	dir := t.TempDir()
	spec, stdin := inputSpec(t, dir, InterruptGraceful)
	ctx, cancel := context.WithCancelCause(context.Background())
	done := runAsync(ctx, spec)
	waitForPID(t, dir, "child")
	require.Eventually(t, func() bool { return len(liveChildren()) == 1 }, pidWait, 10*time.Millisecond)

	// A multi-component run cancels its scheduler context when the signal arrives.
	cancel(fmt.Errorf("%w: interrupt", errUtils.ErrSignalReceived))
	requireStillRunning(t, done, "an interrupt-canceled context must not kill a graceful child")
	require.NoError(t, stdin.Close())
	res := awaitRun(t, done)
	assert.True(t, res.Success(), "the child exited on its own and decides the outcome: %+v", res)
}

func TestCommandDisplayName(t *testing.T) {
	tests := map[string]string{
		"terraform":                    "terraform",
		"tofu":                         "tofu",
		"terraform-1.5.7":              "terraform-1.5.7",
		"TERRAFORM.EXE":                "TERRAFORM",
		"terraform.exe":                "terraform",
		"/opt/homebrew/bin/terragrunt": "terragrunt",
		filepath.Join("bin", "tofu"):   "tofu",
	}
	for command, want := range tests {
		assert.Equal(t, want, commandDisplayName(command), command)
	}
}
