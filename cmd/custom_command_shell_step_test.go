package cmd

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
)

func TestExecuteShellStep_TtyRoutesToShellSession(t *testing.T) {
	origFn := runShellSessionFn
	defer func() { runShellSessionFn = origFn }()

	var gotSpec *process.ShellSessionSpec
	runShellSessionFn = func(ctx context.Context, spec *process.ShellSessionSpec) error {
		gotSpec = spec
		return nil
	}

	step := &schema.Task{Type: schema.TaskTypeShell, Tty: true, Interactive: true}
	err := executeShellStep(step, "aws ssm start-session", "ssh-step-0", "/work", []string{"FOO=bar"})
	require.NoError(t, err)

	require.NotNil(t, gotSpec, "tty step must route to RunShellSession")
	assert.True(t, gotSpec.TTY)
	assert.True(t, gotSpec.Interactive)
	assert.Equal(t, "aws ssm start-session", gotSpec.Command)
	assert.Equal(t, "ssh-step-0", gotSpec.Name)
	assert.Equal(t, "/work", gotSpec.Dir)
	assert.Equal(t, []string{"FOO=bar"}, gotSpec.Env)
}

func TestExecuteShellStep_TtyWithoutInteractive(t *testing.T) {
	origFn := runShellSessionFn
	defer func() { runShellSessionFn = origFn }()

	var gotSpec *process.ShellSessionSpec
	runShellSessionFn = func(ctx context.Context, spec *process.ShellSessionSpec) error {
		gotSpec = spec
		return nil
	}

	step := &schema.Task{Type: schema.TaskTypeShell, Tty: true}
	err := executeShellStep(step, "top -b -n 1", "step-0", "", nil)
	require.NoError(t, err)

	require.NotNil(t, gotSpec)
	assert.True(t, gotSpec.TTY)
	assert.False(t, gotSpec.Interactive, "tty without interactive must not forward stdin")
}

func TestExecuteShellStep_PlainStepDoesNotUseShellSession(t *testing.T) {
	origFn := runShellSessionFn
	defer func() { runShellSessionFn = origFn }()

	called := false
	runShellSessionFn = func(ctx context.Context, spec *process.ShellSessionSpec) error {
		called = true
		return nil
	}

	step := &schema.Task{Type: schema.TaskTypeShell}
	err := executeShellStep(step, "echo plain", "step-0", "", nil)
	require.NoError(t, err)

	assert.False(t, called, "plain shell steps must keep the existing execution path")
	assert.False(t, signals.InterruptExitSuspended())
}

func TestExecuteShellStep_InteractiveSuspendsInterruptExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep-based timing check requires a Unix sleep binary")
	}
	require.False(t, signals.InterruptExitSuspended())

	done := make(chan error, 1)
	step := &schema.Task{Type: schema.TaskTypeShell, Interactive: true}
	go func() {
		done <- executeShellStep(step, "sleep 0.5", "step-0", "", nil)
	}()

	// The suspension must be active while the step runs and released after.
	assert.Eventually(t, signals.InterruptExitSuspended, time.Second, 10*time.Millisecond)
	require.NoError(t, <-done)
	assert.False(t, signals.InterruptExitSuspended())
}
