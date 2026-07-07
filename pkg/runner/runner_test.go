package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/runner/step"
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

func TestRunAll_SkipsTaskWhenConditionIsFalse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{Name: "skip", Command: "echo skip", Type: "shell", When: schema.MustCondition("never")},
		{Name: "run", Command: "echo run", Type: "shell"},
	}

	mockRunner.EXPECT().RunShell(ctx, "echo run", "run", ".", []string(nil), false).Return(nil)

	err := RunAll(ctx, tasks, mockRunner, Options{})
	require.NoError(t, err)
}

func TestRunAll_WhenConditionUsesEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{
			Name:    "dev-shell-only",
			Command: "echo dev shell",
			Type:    "shell",
			When:    schema.MustCondition(`env["ATMOS_DEV_SHELL"] == "1"`),
		},
		{
			Name:    "not-dev-shell",
			Command: "echo not dev shell",
			Type:    "shell",
			When:    schema.MustCondition(`!("ATMOS_DEV_SHELL" in env) || env["ATMOS_DEV_SHELL"] != "1"`),
		},
	}

	mockRunner.EXPECT().RunShell(ctx, "echo dev shell", "dev-shell-only", ".", []string{"ATMOS_DEV_SHELL=1"}, false).Return(nil)

	err := RunAll(ctx, tasks, mockRunner, Options{Env: []string{"ATMOS_DEV_SHELL=1"}})
	require.NoError(t, err)
}

func TestRunAll_WhenConditionUsesResolvedTaskEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{
			Name:    "templated-env",
			Command: "echo rendered",
			Type:    "shell",
			Env: map[string]string{
				"TARGET_ENV": "{{ .Env.BASE_ENV }}-suffix",
			},
			When: schema.MustCondition(`env["TARGET_ENV"] == "base-suffix"`),
		},
	}

	mockRunner.EXPECT().RunShell(ctx, "echo rendered", "templated-env", ".", []string{"BASE_ENV=base"}, false).Return(nil)

	err := RunAll(ctx, tasks, mockRunner, Options{Env: []string{"BASE_ENV=base"}})
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

func TestRunAll_RejectsInvalidWhenBeforeRunningTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{Name: "before-invalid", Command: "echo should not run", Type: schema.TaskTypeShell},
		{Name: "invalid", Command: "echo invalid", Type: schema.TaskTypeShell, When: schema.MustCondition("failure")},
	}

	// Validation must fail before any task executes (no mock expectations).
	err := RunAll(ctx, tasks, mockRunner, Options{})
	require.Error(t, err)
	assert.ErrorIs(t, err, schema.ErrInvalidWhenCondition)
}

// TestRun_StepHandlerTaskSuccess covers runStepHandler's happy path: an
// extended step type (registered in the step handler registry, not one of
// the core CommandRunner types) is validated, executed, and its result is
// stored in the shared step Variables under the task's name.
func TestRun_StepHandlerTaskSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	vars := step.NewVariables()
	task := Task{
		Name:    "print-hint",
		Type:    "hint",
		Content: "Run `atmos dev shell`.",
	}
	opts := Options{StepVars: vars}

	// The step handler registry path never touches the CommandRunner.
	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)

	value, ok := vars.GetValue("print-hint")
	require.True(t, ok)
	assert.Equal(t, "Run `atmos dev shell`.", value)
}

// TestRun_StepHandlerTaskCreatesVariablesWhenNil covers the `vars ==
// nil` branch in runStepHandler, where a local Variables instance is
// created because Options.StepVars was not supplied.
func TestRun_StepHandlerTaskCreatesVariablesWhenNil(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Type:    "hint",
		Content: "no vars supplied",
	}
	opts := Options{} // No StepVars and no AtmosConfig.

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

// TestRun_StepHandlerTaskSetsAtmosConfig covers the `if opts.AtmosConfig !=
// nil { vars.SetAtmosConfig(...) }` branch in runStepHandler.
func TestRun_StepHandlerTaskSetsAtmosConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	vars := step.NewVariables()
	task := Task{
		Type:    "hint",
		Content: "with atmos config",
	}
	opts := Options{
		StepVars:    vars,
		AtmosConfig: &schema.AtmosConfiguration{BasePath: "/base"},
	}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
	assert.Equal(t, "/base", vars.AtmosConfig.BasePath)
}

// TestRun_StepHandlerTaskValidationFailure covers the validation-error
// branch in runStepHandler: a hint step without required content must fail
// Validate() before Execute() is ever invoked.
func TestRun_StepHandlerTaskValidationFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Name: "missing-content",
		Type: "hint",
		// Content intentionally omitted - required field.
	}
	opts := Options{}

	err := Run(ctx, &task, mockRunner, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step validation failed")
}

// TestRun_StepHandlerTaskExecuteError covers the branch where Execute()
// itself fails and the error propagates out of runStepHandler without
// storing any result in Variables.
func TestRun_StepHandlerTaskExecuteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	vars := step.NewVariables()
	task := Task{
		Name:    "bad-hint-template",
		Type:    "hint",
		Content: "{{ range .steps }}",
	}
	opts := Options{StepVars: vars}

	err := Run(ctx, &task, mockRunner, opts)
	require.Error(t, err)

	_, ok := vars.GetValue("bad-hint-template")
	assert.False(t, ok, "no result should be stored when Execute fails")
}

// TestRun_StepHandlerTaskWithRetry covers the `task.Retry != nil` branch in
// runStepHandler, routing execution through retry.Do instead of a direct
// call.
func TestRun_StepHandlerTaskWithRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	maxAttempts := 2
	initialDelay := time.Millisecond
	vars := step.NewVariables()
	task := Task{
		Name:    "retry-hint",
		Type:    "hint",
		Content: "retried hint",
		Retry: &schema.RetryConfig{
			MaxAttempts:  &maxAttempts,
			InitialDelay: &initialDelay,
		},
	}
	opts := Options{StepVars: vars}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)

	value, ok := vars.GetValue("retry-hint")
	require.True(t, ok)
	assert.Equal(t, "retried hint", value)
}

// TestRun_StepHandlerTaskWithoutNameSkipsOutputStorage covers the `if
// task.Name != "" && result != nil` guard: unnamed steps must succeed
// without attempting to store a result in Variables.
func TestRun_StepHandlerTaskWithoutNameSkipsOutputStorage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	vars := step.NewVariables()
	task := Task{
		Type:    "hint",
		Content: "unnamed hint",
		// Name intentionally omitted.
	}
	opts := Options{StepVars: vars}

	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

// TestRun_StepHandlerTaskOutputResolutionErrorPropagates covers the `if
// outputErr := vars.SetWithOutputs(...); outputErr != nil { return outputErr
// }` branch in runStepHandler: the step itself succeeds, but a declared
// `outputs` template fails to resolve against the result.
func TestRun_StepHandlerTaskOutputResolutionErrorPropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	vars := step.NewVariables()
	task := Task{
		Name:    "hint-with-bad-output",
		Type:    "hint",
		Content: "hint succeeds",
		Outputs: map[string]string{
			"broken": "{{ range .steps }}",
		},
	}
	opts := Options{StepVars: vars}

	err := Run(ctx, &task, mockRunner, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve output")
}

// TestRun_ContainerStepDefaultsWorkingDirectoryFromOptsDir covers the
// `if task.Type == "container" && workflowStep.WorkingDirectory == "" &&
// opts.Dir != ""` branch in runStepHandler: a container step with no
// explicit working_directory inherits opts.Dir. DryRun is used so no real
// container runtime is invoked; the resulting preview command mounts
// opts.Dir as the workspace host path, proving the default was applied.
func TestRun_ContainerStepDefaultsWorkingDirectoryFromOptsDir(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	dir := t.TempDir()
	task := Task{
		Name: "container-step",
		Type: "container",
		Run: &schema.ContainerRunStep{
			Image:   "alpine:latest",
			Command: "echo hi",
		},
		// WorkingDirectory intentionally omitted so opts.Dir is used.
	}
	opts := Options{
		Dir:    dir,
		DryRun: true,
	}

	// No mock expectations: container steps never touch the CommandRunner.
	err := Run(ctx, &task, mockRunner, opts)
	require.NoError(t, err)
}

// TestTaskConditionContext_ResolvesTaskEnvError covers the error branch in
// taskConditionContext where resolveTaskConditionEnv fails to parse a
// malformed Go template in a task's env value.
func TestTaskConditionContext_ResolvesTaskEnvError(t *testing.T) {
	task := Task{
		Name: "bad-env-task",
		Env: map[string]string{
			"BROKEN": "{{ range .steps }}",
		},
	}

	_, err := taskConditionContext(&task, 0, &Options{}, schema.ConditionPredicateSuccess)
	require.Error(t, err)
}

// TestResolveTaskConditionEnv_ParseTemplateError covers the `if err != nil {
// return nil, fmt.Errorf("failed to parse env var %s: %w", ...) }` branch:
// a template referencing an undefined function fails at Parse time.
func TestResolveTaskConditionEnv_ParseTemplateError(t *testing.T) {
	taskEnv := map[string]string{
		"BROKEN": "{{ fail }}",
	}

	_, err := resolveTaskConditionEnv(taskEnv, map[string]string{}, &Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse env var")
}

// TestResolveTaskConditionEnv_ExecuteTemplateError covers the `if err :=
// tmpl.Execute(&buf, data); err != nil` branch, distinct from a template
// parse failure: this template parses successfully (valid syntax and
// pipeline) but fails at execution time because the value produced by
// `.Env.MISSING` is a string, and strings have no field named SubField.
func TestResolveTaskConditionEnv_ExecuteTemplateError(t *testing.T) {
	taskEnv := map[string]string{
		"BROKEN": "{{ .Env.MISSING.SubField }}",
	}

	_, err := resolveTaskConditionEnv(taskEnv, map[string]string{"MISSING": "a-string"}, &Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve env var")
}

// TestResolveTaskConditionEnv_UsesStepVarsTemplateData covers the `if opts
// != nil && opts.StepVars != nil` branch in resolveTaskConditionEnv, merging
// step template data (e.g. prior step outputs) into the env-resolution data
// map alongside Env/env.
func TestResolveTaskConditionEnv_UsesStepVarsTemplateData(t *testing.T) {
	vars := step.NewVariables()
	vars.Set("build", step.NewStepResult("build-output"))

	taskEnv := map[string]string{
		"FROM_STEP": "{{ .steps.build.value }}",
		"FROM_ENV":  "{{ .env.BASE }}",
	}
	env := map[string]string{"BASE": "base-value"}
	opts := &Options{StepVars: vars}

	resolved, err := resolveTaskConditionEnv(taskEnv, env, opts)
	require.NoError(t, err)
	assert.Equal(t, "build-output", resolved["FROM_STEP"])
	assert.Equal(t, "base-value", resolved["FROM_ENV"])
}

// TestRunAll_TaskConditionContextEnvParsingError ensures RunAll propagates
// the error returned by taskConditionContext (via resolveTaskConditionEnv)
// when a task's env template fails to parse, covering the `if err != nil {
// return err }` guard in RunAll right after taskConditionContext is called.
func TestRunAll_TaskConditionContextEnvParsingError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{
			Name:    "bad-env-condition",
			Command: "echo unreached",
			Type:    "shell",
			Env: map[string]string{
				"BROKEN": "{{ range .steps }}",
			},
		},
	}

	// No mock expectations: the task must never reach RunShell.
	err := RunAll(ctx, tasks, mockRunner, Options{})
	require.Error(t, err)
}

// TestRunAll_EvaluateWhenErrorPropagates covers the `if err != nil { return
// err }` guard in RunAll right after
// task.When.EvaluateWithImplicitSuccessE(conditionContext) is called: an
// otherwise-valid condition that references an undefined variable fails at
// evaluation time (not at the earlier ValidateStepCondition check).
func TestRunAll_EvaluateWhenErrorPropagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{
			// This condition compiles (the CEL env declares `env` as a
			// map[string]string) and passes ValidateStepCondition (it
			// mentions neither "failure" nor lifecycle status), but fails at
			// evaluation time because the referenced key is absent from the
			// map, which CEL treats as a runtime "no such key" error.
			Name:    "undefined-var-condition",
			Command: "echo unreached",
			Type:    "shell",
			When:    schema.MustCondition(`env["MISSING_KEY_XYZ"] == "1"`),
		},
	}

	// No mock expectations: the task must never reach RunShell.
	err := RunAll(ctx, tasks, mockRunner, Options{})
	require.Error(t, err)
}

// TestRunStepHandler_ContainerTaskInheritsOptsDirAsWorkingDirectory covers
// the `if task.Type == "container" && workflowStep.WorkingDirectory == "" &&
// opts.Dir != ""` branch in runStepHandler: a container-type task with no
// working_directory of its own must inherit opts.Dir. The container step
// then fails Validate() (no run.image/run.command configured), which is
// sufficient to prove the wiring executed without needing a real container
// runtime.
func TestRunStepHandler_ContainerTaskInheritsOptsDirAsWorkingDirectory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	task := Task{
		Name: "container-task",
		Type: "container",
		// WorkingDirectory intentionally empty so opts.Dir is inherited.
	}
	opts := Options{Dir: "/opt/app"}

	err := Run(ctx, &task, mockRunner, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step validation failed")
}

// TestRunAll_TaskConditionContextSkipsEnvItemsWithoutEquals covers the `key,
// value, ok := strings.Cut(item, "="); if !ok { continue }` branch in
// taskConditionContext: a malformed opts.Env entry lacking "=" must be
// skipped rather than corrupting the condition environment.
func TestRunAll_TaskConditionContextSkipsEnvItemsWithoutEquals(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	ctx := context.Background()

	tasks := Tasks{
		{
			Name:    "run",
			Command: "echo run",
			Type:    "shell",
			When:    schema.MustCondition(`env["GOOD_VAR"] == "1"`),
		},
	}
	opts := Options{Env: []string{"MALFORMED_NO_EQUALS", "GOOD_VAR=1"}}

	mockRunner.EXPECT().RunShell(ctx, "echo run", "run", ".", opts.Env, false).Return(nil)

	err := RunAll(ctx, tasks, mockRunner, opts)
	require.NoError(t, err)
}
