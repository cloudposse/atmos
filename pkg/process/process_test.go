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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
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

func TestDefaultRunnerPopulatesMetricsOnSuccess(t *testing.T) {
	before := metricsprocess.AccumulatedTotal()

	command, args, env := processHelperCommand(t, "stdout-stderr")
	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: command,
		Args:    args,
		Env:     env,
	})

	require.NoError(t, result.Err)
	require.NotNil(t, result.Metrics)
	assert.GreaterOrEqual(t, result.Metrics.WallTime, time.Duration(0))
	assert.GreaterOrEqual(t, result.Metrics.UserCPUTime, time.Duration(0))

	// Accumulate must have been invoked for this run. Global accumulator
	// state can be shared with other tests in this (or other) packages, so
	// assert the total is non-decreasing rather than an exact value.
	after := metricsprocess.AccumulatedTotal()
	assert.GreaterOrEqual(t, after.UserCPUTime, before.UserCPUTime)
}

func TestDefaultRunnerPopulatesMetricsOnFailure(t *testing.T) {
	command, args, env := processHelperCommand(t, "exit")
	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: command,
		Args:    args,
		Env:     env,
	})

	require.Error(t, result.Err)
	require.NotNil(t, result.Metrics, "metrics must be collected even when the subprocess exits non-zero")
	assert.GreaterOrEqual(t, result.Metrics.WallTime, time.Duration(0))
}

func TestDefaultRunnerMetricsNilWhenStartFails(t *testing.T) {
	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: "command-that-should-not-exist",
	})

	require.Error(t, result.Err)
	assert.Nil(t, result.Metrics)
}

func processHelperCommand(t *testing.T, command string) (string, []string, []string) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)

	return exe,
		[]string{"-test.run=TestHelperProcess", "--", command},
		append(os.Environ(), processHelperEnv+"=1")
}
