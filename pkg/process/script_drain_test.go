package process

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunScriptSharedOutputWriter(t *testing.T) {
	helper := helperSpec(t, "script-output", t.TempDir(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var output bytes.Buffer
	err := RunScript(ctx, &ScriptSpec{Interpreter: helper.Command, Env: helper.Env}, &output, &output)
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("STDOUT\nSTDERR\n", 100), output.String())
}

type scriptFailedWriter struct{ err error }

func (w scriptFailedWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRunScriptOutputFailureCancelsProducer(t *testing.T) {
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	helper := helperSpec(t, "script-writer-failure", dir, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	want := errors.New("destination failed")
	err := RunScript(ctx, &ScriptSpec{Interpreter: helper.Command, Env: helper.Env}, scriptFailedWriter{want}, nil)
	require.ErrorIs(t, err, want)
	assert.NoError(t, ctx.Err(), "writer failure must cancel the producer without waiting for the deadline")
	assert.Empty(t, liveChildren())
}

func TestRunScriptDrainsOutputAfterInterpreterExit(t *testing.T) {
	for _, mode := range []string{"late-output-parent", "late-output-failure"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			killAtCleanup(t, dir, "child")
			helper := helperSpec(t, mode, dir, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var stdout, stderr bytes.Buffer
			err := RunScript(ctx, &ScriptSpec{Interpreter: helper.Command, Env: helper.Env}, &stdout, &stderr)
			if mode == "late-output-failure" {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)
				assert.Equal(t, 23, exitErr.ExitCode())
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, "LATE-STDOUT\n", stdout.String())
			assert.Equal(t, "LATE-STDERR\n", stderr.String())
		})
	}
}

func TestRunScriptCancellationWhileDrainingOutput(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "parent", "child")
	helper := helperSpec(t, "drain-cancel-parent", dir, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunScript(ctx, &ScriptSpec{Interpreter: helper.Command, Env: helper.Env}, &stdout, &stderr)
	}()
	parent := waitForPID(t, dir, "parent")
	child := waitForPID(t, dir, "child")
	requireGone(t, parent, "interpreter")
	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, os.ErrClosed, "closing our own read handle during cancellation is not a failure")
		assert.NotContains(t, err.Error(), "file already closed")
	case <-time.After(childExitCleanupWait + time.Second):
		t.Fatal("cancellation must interrupt output draining after the interpreter exits")
	}
	requireGone(t, child, "output holder")
	assert.Empty(t, liveChildren())
}
