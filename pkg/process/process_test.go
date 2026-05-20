package process

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRunnerUsesInjectedStreams(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}

	var stdout, stderr bytes.Buffer
	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: "/bin/sh",
		Args:    []string{"-c", "printf stdout; printf stderr >&2"},
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
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}

	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: "/bin/sh",
		Args:    []string{"-c", "exit 7"},
	})

	require.Error(t, result.Err)
	assert.Equal(t, 7, result.ExitCode)

	var exitErr *exec.ExitError
	assert.True(t, errors.As(result.Err, &exitErr))
}

func TestDefaultRunnerCancelsWithContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result := DefaultRunner{}.Run(ctx, TaskSpec{
		Command: "/bin/sh",
		Args:    []string{"-c", "sleep 2"},
	})

	require.Error(t, result.Err)
	assert.True(t, result.Canceled)
	assert.NotEqual(t, 0, result.ExitCode)
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
}
