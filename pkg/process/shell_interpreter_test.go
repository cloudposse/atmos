package process

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
)

// interpreterEnv builds an environment that lets a script re-execute the test binary as $BIN in a
// helper mode.
func interpreterEnv(t *testing.T, extra ...string) []string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return append(append(os.Environ(), "BIN="+exe), extra...)
}

func TestRunShellInterpreter_BuiltinsAndExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunShellInterpreter(context.Background(), &ShellInterpreterSpec{
		Command: `echo "hello $WP5_NAME"; echo oops >&2; exit 3`,
		Name:    "builtins",
		Env:     append(os.Environ(), "WP5_NAME=world"),
		Stdout:  &stdout,
		Stderr:  &stderr,
	})

	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 3, exitErr.Code)
	assert.Equal(t, "hello world\n", stdout.String())
	assert.Equal(t, "oops\n", stderr.String())
}

func TestRunShellInterpreter_ParseErrorIsReturned(t *testing.T) {
	err := RunShellInterpreter(context.Background(), &ShellInterpreterSpec{Command: "if then", Name: "broken"})
	require.Error(t, err)
	var exitErr errUtils.ExitCodeError
	assert.NotErrorAs(t, err, &exitErr, "a syntax error is not an exit status")
}

func TestRunShellInterpreter_ExternalCommandExitStatusAndOutput(t *testing.T) {
	env := interpreterEnv(t, "_ATMOS_TEST_EXIT_ZERO=1")
	var stdout bytes.Buffer
	err := RunShellInterpreter(context.Background(), &ShellInterpreterSpec{
		Command: `"$BIN"; echo "status=$?"`,
		Name:    "external",
		Env:     env,
		Stdout:  &stdout,
	})
	require.NoError(t, err)
	assert.Equal(t, "status=0\n", stdout.String())
}

func TestRunShellInterpreter_UnknownCommandIsStatus127(t *testing.T) {
	var stderr bytes.Buffer
	err := RunShellInterpreter(context.Background(), &ShellInterpreterSpec{
		Command: "wp5-no-such-command-anywhere",
		Name:    "missing",
		Env:     os.Environ(),
		Stderr:  &stderr,
	})
	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 127, exitErr.Code)
	assert.Contains(t, stderr.String(), "wp5-no-such-command-anywhere")
}

func TestRunShellInterpreter_StdinIsPassedToCommands(t *testing.T) {
	// The test binary in input mode waits for stdin to reach EOF, then exits 0.
	env := interpreterEnv(t, helperModeEnv+"="+modeInput, helperPIDDirEnv+"="+t.TempDir())
	err := RunShellInterpreter(context.Background(), &ShellInterpreterSpec{
		Command: `"$BIN"`,
		Name:    "stdin",
		Env:     env,
		Stdin:   strings.NewReader("payload"),
	})
	require.NoError(t, err)
}

func TestRunShellInterpreter_DoesNotAddToTheInvocationSummary(t *testing.T) {
	env := interpreterEnv(t, "_ATMOS_TEST_EXIT_ZERO=1")
	before := metricsprocess.AccumulatedTotal()
	require.NoError(t, RunShellInterpreter(context.Background(), &ShellInterpreterSpec{Command: `"$BIN"`, Name: "metrics", Env: env}))
	assert.Equal(t, before, metricsprocess.AccumulatedTotal(), "shell steps never contributed to the summary")
}

// A shell command that spawns a worker must not leave the worker running when the context ends.
func TestRunShellInterpreter_ContextCancelKillsGrandchildren(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "parent", "child")
	env := interpreterEnv(t, helperModeEnv+"="+modeParent, helperPIDDirEnv+"="+dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	var runErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErr = RunShellInterpreter(ctx, &ShellInterpreterSpec{Command: `"$BIN" wp5-interpreter-marker & wait`, Name: "orphans", Env: env, Dir: dir})
	}()

	parent := waitForPID(t, dir, "parent")
	child := waitForPID(t, dir, "child")
	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(goneWait + 5*time.Second):
		t.Fatal("RunShellInterpreter did not return after cancellation")
	}

	// The interpreter may report a cancelled background job as success; the callers that care
	// (steps with a timeout) check their own deadline.
	if runErr != nil {
		require.ErrorIs(t, runErr, context.Canceled)
	}
	requireGone(t, parent, "command started by the shell")
	requireGone(t, child, "grandchild of the command")
	assert.Empty(t, liveChildren(), "finished children must be unregistered")
}

func TestRunShellInterpreter_DeadlineKillsForegroundGrandchildren(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "parent", "child")
	env := interpreterEnv(t, helperModeEnv+"="+modeParent, helperPIDDirEnv+"="+dir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*pidWait)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- RunShellInterpreter(ctx, &ShellInterpreterSpec{Command: `"$BIN"`, Name: "deadline", Env: env, Dir: dir})
	}()
	parent := waitForPID(t, dir, "parent")
	child := waitForPID(t, dir, "child")
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(goneWait + 5*time.Second):
		t.Fatal("RunShellInterpreter did not return after cancellation")
	}
	requireGone(t, parent, "command started by the shell")
	requireGone(t, child, "grandchild of the command")
}

func TestRunShellInterpreter_PositionalParameters(t *testing.T) {
	var stdout bytes.Buffer
	err := RunShellInterpreter(context.Background(), &ShellInterpreterSpec{
		Command: `echo "count=$# first=$1"; for arg in "$@"; do echo "arg=[$arg]"; done`,
		Name:    "params",
		Env:     os.Environ(),
		Params:  []string{"-e", "two words", "it's"},
		Stdout:  &stdout,
	})
	require.NoError(t, err)
	assert.Equal(t, "count=3 first=-e\narg=[-e]\narg=[two words]\narg=[it's]\n", stdout.String())
}

func TestRunShellInterpreter_NoParametersLeavesThemEmpty(t *testing.T) {
	var stdout bytes.Buffer
	require.NoError(t, RunShellInterpreter(context.Background(), &ShellInterpreterSpec{Command: `echo "count=$#"`, Name: "none", Env: os.Environ(), Stdout: &stdout}))
	assert.Equal(t, "count=0\n", stdout.String())
}
