package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const processHelperEnv = "GO_WANT_ATMOS_PROCESS_HELPER"

func TestHelperProcess(t *testing.T) {
	if os.Getenv(processHelperEnv) != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 {
		os.Exit(2)
	}

	switch args[1] {
	case "stdout-stderr":
		fmt.Fprint(os.Stdout, "stdout")
		fmt.Fprint(os.Stderr, "stderr")
	case "exit":
		os.Exit(7)
	case "sleep":
		time.Sleep(2 * time.Second)
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func TestDefaultRunnerUsesInjectedStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command, args, env := processHelperCommand(t, "stdout-stderr")
	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: command,
		Args:    args,
		Env:     env,
		Streams: Streams{
			Stdout: &stdout,
			Stderr: &stderr,
		},
	})

	require.NoError(t, result.Err)
	assert.True(t, result.Success())
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "stdout", stdout.String())
	assert.Equal(t, "stderr", stderr.String())
}

func TestDefaultRunnerPreservesExitCode(t *testing.T) {
	command, args, env := processHelperCommand(t, "exit")
	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: command,
		Args:    args,
		Env:     env,
	})

	require.Error(t, result.Err)
	assert.Equal(t, 7, result.ExitCode)
	assert.ErrorIs(t, result.Err, errUtils.ErrProcessWaitFailed)

	var exitErr *exec.ExitError
	assert.True(t, errors.As(result.Err, &exitErr))
}

func TestDefaultRunnerCancelsWithContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	command, args, env := processHelperCommand(t, "sleep")
	result := DefaultRunner{}.Run(ctx, TaskSpec{
		Command: command,
		Args:    args,
		Env:     env,
	})

	require.Error(t, result.Err)
	assert.True(t, result.Canceled)
	assert.NotEqual(t, 0, result.ExitCode)
	assert.ErrorIs(t, result.Err, errUtils.ErrProcessWaitFailed)
	assert.ErrorIs(t, result.Err, context.DeadlineExceeded)
}

func TestDefaultRunnerDryRunDoesNotStartProcess(t *testing.T) {
	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: "command-that-should-not-exist",
		DryRun:  true,
	})

	require.NoError(t, result.Err)
	assert.False(t, result.Started)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.StartedAt.IsZero())
	assert.False(t, result.FinishedAt.IsZero())
}

func TestDefaultRunnerReportsStartFailureWithoutStartedAt(t *testing.T) {
	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: "command-that-should-not-exist",
	})

	require.Error(t, result.Err)
	assert.ErrorIs(t, result.Err, errUtils.ErrProcessStartFailed)
	assert.False(t, result.Started)
	assert.Equal(t, -1, result.ExitCode)
	assert.True(t, result.StartedAt.IsZero())
	assert.False(t, result.FinishedAt.IsZero())
}

func processHelperCommand(t *testing.T, command string) (string, []string, []string) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)

	return exe,
		[]string{"-test.run=TestHelperProcess", "--", command},
		append(os.Environ(), processHelperEnv+"=1")
}
