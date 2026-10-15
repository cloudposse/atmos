package process

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/interp"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// pkg/process installs its grouped command handler into pkg/utils, so the scripts custom commands,
// workflows, !exec, and hook commands run end their whole process tree on a timeout.
func TestShellRunnerWithWritersUsesTheProcessRunner(t *testing.T) {
	var stdout bytes.Buffer
	err := u.ShellRunnerWithWriters(&u.ShellRunnerSpec{
		Command: `echo before; "$BIN"; echo "status=$?"`,
		Name:    "installed",
		Env:     append(os.Environ(), "BIN="+testBinary(t), "_ATMOS_TEST_EXIT_ZERO=1"),
		Stdout:  &stdout,
	})
	require.NoError(t, err)
	assert.Equal(t, "before\nstatus=0\n", stdout.String())
}

// A backgrounded grandchild that keeps the output pipe open used to block the run forever after
// the timeout killed its parent: os/exec waits for the pipe to close, and the interpreter's default
// handler signalled only the direct child. The run must return promptly and leave nothing behind.
func TestShellRunnerWithWritersTimeoutWithPipeHoldingGrandchildReturns(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "parent", "child")
	env := append(os.Environ(), "BIN="+testBinary(t), helperModeEnv+"="+modeHolder, helperPIDDirEnv+"="+dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	var stdout bytes.Buffer // not an *os.File, so the child's output goes through a pipe
	go func() {
		result <- u.ShellRunnerWithWriters(&u.ShellRunnerSpec{Context: ctx, Command: `"$BIN"`, Name: "holder", Env: env, Dir: dir, Stdout: &stdout})
	}()
	parent := waitForPID(t, dir, "parent")
	child := waitForPID(t, dir, "child")

	cancel()
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(goneWait + 5*time.Second):
		t.Fatal("the run did not return after cancellation: a grandchild holding the output pipe blocked it")
	}
	requireGone(t, parent, "command started by the script")
	requireGone(t, child, "pipe-holding grandchild")
}

func TestSetShellExecMiddlewareReplacesAndRestores(t *testing.T) {
	t.Cleanup(func() { u.SetShellExecMiddleware(groupedMiddleware) })

	var seen []string
	u.SetShellExecMiddleware(func(_ interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(_ context.Context, args []string) error {
			seen = append(seen, args...)
			return nil
		}
	})
	require.NoError(t, u.ShellRunnerWithWriters(&u.ShellRunnerSpec{Command: "wp5-external one two", Name: "custom", Env: os.Environ()}))
	assert.Equal(t, []string{"wp5-external", "one", "two"}, seen, "the installed middleware receives the external command")

	// A second registration replaces the first.
	var replaced bool
	u.SetShellExecMiddleware(func(_ interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(context.Context, []string) error { replaced = true; return nil }
	})
	require.NoError(t, u.ShellRunnerWithWriters(&u.ShellRunnerSpec{Command: "wp5-external", Name: "replaced", Env: os.Environ()}))
	assert.True(t, replaced)
	assert.Len(t, seen, 3, "the first middleware is no longer called")

	// nil restores the interpreter's default handler, which reports a missing command as status 127.
	u.SetShellExecMiddleware(nil)
	var stderr bytes.Buffer
	err := u.ShellRunnerWithWriters(&u.ShellRunnerSpec{Command: "wp5-no-such-command-anywhere", Name: "default", Env: os.Environ(), Stderr: &stderr})
	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 127, exitErr.Code)
}

func testBinary(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return exe
}
