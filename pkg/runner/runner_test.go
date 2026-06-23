package runner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
)

func TestRun_ShellTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Name:    "echo-task",
		Command: "echo hello",
		Type:    "shell",
	}
	opts := Options{
		Dir:    "/app",
		DryRun: true,
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_ShellTaskWithDryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Command: "rm -rf /",
		Type:    "shell",
	}
	opts := Options{
		DryRun: true,
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_AtmosTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Name:    "deploy-vpc",
		Command: "terraform apply vpc",
		Type:    "atmos",
		Stack:   "dev-us-east-1",
	}
	opts := Options{
		Dir:    "/infra",
		DryRun: true,
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_AtmosTaskWithStackOverride(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Command: "terraform plan vpc",
		Type:    "atmos",
		Stack:   "dev-us-east-1",
	}
	opts := Options{
		Stack:  "prod-us-west-2", // Override.
		DryRun: true,
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_WorkingDirectoryOverride(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Command:          "make build",
		Type:             "shell",
		WorkingDirectory: "/custom/dir", // Task-specific override.
	}
	opts := Options{
		Dir:    "/default/dir",
		DryRun: true,
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_DefaultsToShellType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Command: "echo hello",
		// Type is empty - should default to shell.
	}
	opts := Options{DryRun: true}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_UnknownType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Command: "something",
		Type:    "unknown",
	}
	opts := Options{}

	err := Run(ctx, &task, mockRunner, opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownTaskType)
}

func TestRun_PropagatesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Command: "exit 1",
		Type:    "shell",
	}
	opts := Options{}

	err := Run(ctx, &task, mockRunner, opts)
	require.Error(t, err)
	// Non-zero subprocess exits surface as ExitCodeError ("subcommand exited
	// with code N"), matching the legacy shell-interpreter behavior.
	assert.Contains(t, err.Error(), "subcommand exited with code 1")
}

func TestRun_Timeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Command: "sleep 10",
		Type:    "shell",
		Timeout: 100 * time.Millisecond,
	}
	opts := Options{DryRun: true}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRunAll_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{Command: "exit 0", Type: "shell"},
		{Command: "exit 0", Type: "shell"},
		{Command: "exit 0", Type: "shell"},
	}
	opts := Options{DryRun: true}

	err := RunAll(ctx, tasks, mockRunner, opts)
	require.NoError(t, err)
}

func TestRunAll_StopsOnFirstError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{Command: "exit 0", Type: "shell"},
		{Name: "failing-task", Command: "exit 1", Type: "shell"},
		{Command: "exit 0", Type: "shell"}, // Should not be called.
	}
	opts := Options{}

	err := RunAll(ctx, tasks, mockRunner, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task 1 (failing-task) failed")
}

func TestRunAll_EmptyTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{}
	opts := Options{}

	err := RunAll(ctx, tasks, mockRunner, opts)
	require.NoError(t, err)
}

func TestAppendStackArg_NoSeparator(t *testing.T) {
	args := []string{"terraform", "plan", "vpc"}
	result := appendStackArg(args, "dev")
	assert.Equal(t, []string{"terraform", "plan", "vpc", "-s", "dev"}, result)
}

func TestAppendStackArg_WithSeparator(t *testing.T) {
	args := []string{"terraform", "plan", "vpc", "--", "-var", "foo=bar"}
	result := appendStackArg(args, "dev")
	assert.Equal(t, []string{"terraform", "plan", "vpc", "-s", "dev", "--", "-var", "foo=bar"}, result)
}

func TestRun_AtmosTaskWithoutStack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	// Task without stack, opts without stack - should not add -s flag.
	task := Task{
		Command: "terraform plan vpc",
		Type:    "atmos",
		// No stack specified.
	}
	opts := Options{
		Dir:    "/infra",
		DryRun: true,
		// No stack override.
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_AtmosTaskUsesTaskStackWhenNoOverride(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	// Task with stack, opts without stack - should use task's stack.
	task := Task{
		Command: "terraform plan vpc",
		Type:    "atmos",
		Stack:   "dev-us-east-1",
	}
	opts := Options{
		DryRun: true,
		// No stack override - should use task.Stack.
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_AtmosTaskShellParsingFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	// Command with unclosed quote - shell.Fields will fail, fallback to strings.Fields.
	// Note: This is an edge case where the command has malformed quoting.
	// shell.Fields returns error for unclosed quotes, strings.Fields just splits on whitespace.
	task := Task{
		Command: `echo "unclosed`,
		Type:    "atmos",
	}
	opts := Options{DryRun: true}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_AtmosTaskWithEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Command: "terraform plan vpc",
		Type:    "atmos",
	}
	opts := Options{
		Dir:    "/infra",
		Env:    []string{"TF_LOG=DEBUG", "AWS_REGION=us-east-1"},
		DryRun: true,
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_AtmosTaskWithAtmosConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/base",
	}

	task := Task{
		Command: "terraform plan vpc",
		Type:    "atmos",
	}
	opts := Options{
		AtmosConfig: atmosConfig,
		DryRun:      true,
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_AtmosTaskWithDryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Command: "terraform apply vpc",
		Type:    "atmos",
	}
	opts := Options{
		DryRun: true,
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRunAll_UsesTaskName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	// Tasks without names - should use step1, step2 naming.
	tasks := Tasks{
		{Command: "exit 1", Type: "shell"},
	}
	opts := Options{}

	err := RunAll(ctx, tasks, mockRunner, opts)
	require.Error(t, err)
	// Should include step1 since no name was provided.
	assert.Contains(t, err.Error(), "task 0 (step1) failed")
}

func TestTaskName_WithName(t *testing.T) {
	task := &Task{Name: "my-task"}
	assert.Equal(t, "my-task", taskName(task, 0))
}

func TestTaskName_WithoutName(t *testing.T) {
	task := &Task{}
	assert.Equal(t, "step1", taskName(task, 0))
	assert.Equal(t, "step5", taskName(task, 4))
}

func TestRun_ShellTaskTty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Name:        "session-task",
		Command:     "aws ssm start-session",
		Type:        "shell",
		Tty:         true,
		Interactive: true,
	}
	// Dry-run exercises the terminal-session routing without executing.
	// The CommandRunner must NOT be called for tty tasks.
	opts := Options{DryRun: true}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRun_ShellTaskInteractiveUsesSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Name:        "interactive-task",
		Command:     "read answer",
		Type:        "shell",
		Interactive: true,
	}
	// Interactive tasks route to the terminal session path, NOT the
	// CommandRunner (no mock expectations). Dry-run avoids real execution;
	// suspension/attachment behavior is covered by pkg/process tests.
	opts := Options{Dir: "/app", DryRun: true}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
	assert.False(t, signals.InterruptExitSuspended(), "suspension must be released after the task")
}

func TestRun_ExecTaskDryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Name:    "session-task",
		Command: "aws ssm start-session",
		Type:    schema.TaskTypeExec,
	}
	// Dry-run exercises the exec routing without replacing the process.
	// The CommandRunner must NOT be called for exec tasks.
	err := Run(ctx, &task, mockRunner, Options{DryRun: true})
	require.NoError(t, err)
}

func TestRunAll_RejectsNonFinalExecTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{Name: "session", Command: "ssh host", Type: schema.TaskTypeExec},
		{Name: "after", Command: "echo never runs", Type: schema.TaskTypeShell},
	}

	// Validation must fail before any task executes (no mock expectations).
	err := RunAll(ctx, tasks, mockRunner, Options{DryRun: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, schema.ErrExecStepNotLast)
}
