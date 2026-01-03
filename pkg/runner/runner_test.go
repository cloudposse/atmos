package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
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
		Dir: "/app",
	}

	mockRunner.EXPECT().
		RunShell(ctx, "echo hello", "echo-task", "/app", []string(nil), false).
		Return(nil)

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

	mockRunner.EXPECT().
		RunShell(ctx, "rm -rf /", "", ".", []string(nil), true).
		Return(nil)

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
		Dir: "/infra",
	}

	mockRunner.EXPECT().
		RunAtmos(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, params *AtmosExecParams) error {
			assert.Equal(t, []string{"terraform", "apply", "vpc", "-s", "dev-us-east-1"}, params.Args)
			assert.Equal(t, "/infra", params.Dir)
			assert.False(t, params.DryRun)
			return nil
		})

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
		Stack: "prod-us-west-2", // Override.
	}

	mockRunner.EXPECT().
		RunAtmos(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, params *AtmosExecParams) error {
			// Should use opts.Stack, not task.Stack.
			assert.Equal(t, []string{"terraform", "plan", "vpc", "-s", "prod-us-west-2"}, params.Args)
			return nil
		})

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
		Dir: "/default/dir",
	}

	mockRunner.EXPECT().
		RunShell(ctx, "make build", "", "/custom/dir", []string(nil), false).
		Return(nil)

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
	opts := Options{}

	mockRunner.EXPECT().
		RunShell(ctx, "echo hello", "", ".", []string(nil), false).
		Return(nil)

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

	expectedErr := errors.New("command failed with exit code 1")
	mockRunner.EXPECT().
		RunShell(ctx, "exit 1", "", ".", []string(nil), false).
		Return(expectedErr)

	err := Run(ctx, &task, mockRunner, opts)
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
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
	opts := Options{}

	mockRunner.EXPECT().
		RunShell(gomock.Any(), "sleep 10", "", ".", []string(nil), false).
		DoAndReturn(func(ctx context.Context, _, _, _ string, _ []string, _ bool) error {
			// Verify context has deadline.
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "context should have deadline")
			assert.WithinDuration(t, time.Now().Add(100*time.Millisecond), deadline, 50*time.Millisecond)
			return nil
		})

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

func TestRunAll_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{Command: "echo 1", Type: "shell"},
		{Command: "echo 2", Type: "shell"},
		{Command: "echo 3", Type: "shell"},
	}
	opts := Options{}

	gomock.InOrder(
		mockRunner.EXPECT().RunShell(ctx, "echo 1", "", ".", []string(nil), false).Return(nil),
		mockRunner.EXPECT().RunShell(ctx, "echo 2", "", ".", []string(nil), false).Return(nil),
		mockRunner.EXPECT().RunShell(ctx, "echo 3", "", ".", []string(nil), false).Return(nil),
	)

	err := RunAll(ctx, tasks, mockRunner, opts)
	require.NoError(t, err)
}

func TestRunAll_StopsOnFirstError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{Command: "echo 1", Type: "shell"},
		{Name: "failing-task", Command: "exit 1", Type: "shell"},
		{Command: "echo 3", Type: "shell"}, // Should not be called.
	}
	opts := Options{}

	gomock.InOrder(
		mockRunner.EXPECT().RunShell(ctx, "echo 1", "", ".", []string(nil), false).Return(nil),
		mockRunner.EXPECT().RunShell(ctx, "exit 1", "failing-task", ".", []string(nil), false).Return(errors.New("exit code 1")),
		// Third task should NOT be called.
	)

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
		Dir: "/infra",
		// No stack override.
	}

	mockRunner.EXPECT().
		RunAtmos(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, params *AtmosExecParams) error {
			// Should NOT include -s flag.
			assert.Equal(t, []string{"terraform", "plan", "vpc"}, params.Args)
			assert.Equal(t, "/infra", params.Dir)
			return nil
		})

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
		// No stack override - should use task.Stack.
	}

	mockRunner.EXPECT().
		RunAtmos(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, params *AtmosExecParams) error {
			assert.Equal(t, []string{"terraform", "plan", "vpc", "-s", "dev-us-east-1"}, params.Args)
			return nil
		})

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
	opts := Options{}

	mockRunner.EXPECT().
		RunAtmos(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, params *AtmosExecParams) error {
			// When shell.Fields fails, strings.Fields is used.
			// strings.Fields("echo \"unclosed") returns ["echo", "\"unclosed"]
			assert.Equal(t, []string{"echo", `"unclosed`}, params.Args)
			return nil
		})

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
		Dir: "/infra",
		Env: []string{"TF_LOG=DEBUG", "AWS_REGION=us-east-1"},
	}

	mockRunner.EXPECT().
		RunAtmos(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, params *AtmosExecParams) error {
			assert.Equal(t, []string{"TF_LOG=DEBUG", "AWS_REGION=us-east-1"}, params.Env)
			return nil
		})

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
	}

	mockRunner.EXPECT().
		RunAtmos(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, params *AtmosExecParams) error {
			assert.Equal(t, atmosConfig, params.AtmosConfig)
			return nil
		})

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

	mockRunner.EXPECT().
		RunAtmos(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, params *AtmosExecParams) error {
			assert.True(t, params.DryRun)
			return nil
		})

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
		{Command: "echo 1", Type: "shell"},
	}
	opts := Options{}

	mockRunner.EXPECT().
		RunShell(ctx, "echo 1", "", ".", []string(nil), false).
		Return(errors.New("task failed"))

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
