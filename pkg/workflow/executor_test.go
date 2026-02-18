package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newTestParams creates WorkflowParams for tests.
func newTestParams(workflowDef *schema.WorkflowDefinition, opts ExecuteOptions) *WorkflowParams {
	return &WorkflowParams{
		Ctx:                context.Background(),
		AtmosConfig:        &schema.AtmosConfiguration{},
		Workflow:           "test-workflow",
		WorkflowPath:       "test.yaml",
		WorkflowDefinition: workflowDef,
		Opts:               opts,
	}
}

// TestExecutor_Execute_BasicShellWorkflow tests executing a simple shell workflow.
func TestExecutor_Execute_BasicShellWorkflow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks.
	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect shell command to be called.
	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{}, false).
		Return(nil)

	// Create executor.
	executor := NewExecutor(mockRunner, nil, mockUI)

	// Define workflow.
	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo 'hello'",
				Type:    "shell",
			},
		},
	}

	// Execute.
	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.Steps, 1)
	assert.True(t, result.Steps[0].Success)
	assert.Equal(t, "step1", result.Steps[0].StepName)
}

// TestExecutor_Execute_BasicAtmosWorkflow tests executing a simple atmos workflow.
func TestExecutor_Execute_BasicAtmosWorkflow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect UI message and atmos command.
	mockUI.EXPECT().PrintMessage(gomock.Any(), gomock.Any()).AnyTimes()
	mockRunner.EXPECT().
		RunAtmos(gomock.Any()).
		DoAndReturn(func(params *AtmosExecParams) error {
			assert.Equal(t, []string{"terraform", "plan", "vpc"}, params.Args)
			assert.Equal(t, ".", params.Dir)
			assert.False(t, params.DryRun)
			return nil
		})

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Name:    "plan-vpc",
				Command: "terraform plan vpc",
				Type:    "atmos",
			},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.Steps, 1)
	assert.True(t, result.Steps[0].Success)
}

// TestExecutor_Execute_MultipleSteps tests executing a workflow with multiple steps.
func TestExecutor_Execute_MultipleSteps(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect commands in order.
	gomock.InOrder(
		mockRunner.EXPECT().RunShell("echo 'step 1'", "test-workflow-step-0", ".", []string{}, false).Return(nil),
		mockRunner.EXPECT().RunShell("echo 'step 2'", "test-workflow-step-1", ".", []string{}, false).Return(nil),
		mockRunner.EXPECT().RunShell("echo 'step 3'", "test-workflow-step-2", ".", []string{}, false).Return(nil),
	)

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'step 1'", Type: "shell"},
			{Name: "step2", Command: "echo 'step 2'", Type: "shell"},
			{Name: "step3", Command: "echo 'step 3'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.Steps, 3)
	for _, step := range result.Steps {
		assert.True(t, step.Success)
	}
}

// TestExecutor_Execute_EmptyWorkflow tests that empty workflows return error.
func TestExecutor_Execute_EmptyWorkflow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect error to be printed.
	mockUI.EXPECT().PrintError(gomock.Any(), workflowErrorTitle, gomock.Any())

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{},
	}

	params := &WorkflowParams{
		Ctx:                context.Background(),
		AtmosConfig:        &schema.AtmosConfiguration{},
		Workflow:           "empty-workflow",
		WorkflowPath:       "test.yaml",
		WorkflowDefinition: workflowDef,
		Opts:               ExecuteOptions{},
	}
	result, err := executor.Execute(params)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowNoSteps)
	assert.False(t, result.Success)
}

// TestExecutor_Execute_InvalidStepType tests that invalid step types return error.
func TestExecutor_Execute_InvalidStepType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect error to be printed once by handleStepError (which includes resume context).
	mockUI.EXPECT().PrintError(gomock.Any(), workflowErrorTitle, gomock.Any()).Times(1)

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'test'", Type: "invalid-type"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	// ErrInvalidWorkflowStepType is returned directly without wrapping in ErrWorkflowStepFailed.
	assert.ErrorIs(t, err, errUtils.ErrInvalidWorkflowStepType)
	assert.False(t, result.Success)
}

// TestExecutor_Execute_StepFailure tests handling of step failures.
func TestExecutor_Execute_StepFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	stepError := errors.New("command failed")

	// First step succeeds, second fails.
	gomock.InOrder(
		mockRunner.EXPECT().RunShell("echo 'step 1'", "test-workflow-step-0", ".", []string{}, false).Return(nil),
		mockRunner.EXPECT().RunShell("exit 1", "test-workflow-step-1", ".", []string{}, false).Return(stepError),
	)

	// Expect error to be printed for failed step.
	mockUI.EXPECT().PrintError(gomock.Any(), workflowErrorTitle, gomock.Any())

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'step 1'", Type: "shell"},
			{Name: "step2", Command: "exit 1", Type: "shell"},
			{Name: "step3", Command: "echo 'step 3'", Type: "shell"}, // Should not execute.
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowStepFailed)
	assert.False(t, result.Success)
	assert.Len(t, result.Steps, 2) // Only 2 steps executed.
	assert.True(t, result.Steps[0].Success)
	assert.False(t, result.Steps[1].Success)
	assert.NotEmpty(t, result.ResumeCommand)
}

// TestExecutor_Execute_FromStep tests starting execution from a specific step.
func TestExecutor_Execute_FromStep(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Only step2 and step3 should be executed.
	// Note: step indices are 0-based from the filtered list (after --from-step).
	gomock.InOrder(
		mockRunner.EXPECT().RunShell("echo 'step 2'", "test-workflow-step-0", ".", []string{}, false).Return(nil),
		mockRunner.EXPECT().RunShell("echo 'step 3'", "test-workflow-step-1", ".", []string{}, false).Return(nil),
	)

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'step 1'", Type: "shell"},
			{Name: "step2", Command: "echo 'step 2'", Type: "shell"},
			{Name: "step3", Command: "echo 'step 3'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{FromStep: "step2"}))

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.Steps, 3) // Includes skipped step.

	// First step should be marked as skipped.
	assert.True(t, result.Steps[0].Skipped)
	assert.Equal(t, "step1", result.Steps[0].StepName)

	// Other steps executed.
	assert.False(t, result.Steps[1].Skipped)
	assert.False(t, result.Steps[2].Skipped)
}

// TestExecutor_Execute_InvalidFromStep tests error when from-step doesn't exist.
func TestExecutor_Execute_InvalidFromStep(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect error to be printed.
	mockUI.EXPECT().PrintError(gomock.Any(), workflowErrorTitle, gomock.Any())

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'step 1'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{FromStep: "nonexistent"}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFromStep)
	assert.False(t, result.Success)
}

// TestExecutor_Execute_DryRun tests dry-run mode.
func TestExecutor_Execute_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect shell command with dryRun=true.
	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{}, true).
		Return(nil)

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{DryRun: true}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_StackOverride tests stack override precedence.
func TestExecutor_Execute_StackOverride(t *testing.T) {
	tests := []struct {
		name             string
		workflowStack    string
		stepStack        string
		commandLineStack string
		expectedStack    string
	}{
		{
			name:          "workflow stack only",
			workflowStack: "workflow-stack",
			expectedStack: "workflow-stack",
		},
		{
			name:          "step overrides workflow",
			workflowStack: "workflow-stack",
			stepStack:     "step-stack",
			expectedStack: "step-stack",
		},
		{
			name:             "command line overrides all",
			workflowStack:    "workflow-stack",
			stepStack:        "step-stack",
			commandLineStack: "cli-stack",
			expectedStack:    "cli-stack",
		},
		{
			name:             "command line only",
			commandLineStack: "cli-stack",
			expectedStack:    "cli-stack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRunner := NewMockCommandRunner(ctrl)
			mockUI := NewMockUIProvider(ctrl)

			// Capture the args to verify stack was added.
			mockUI.EXPECT().PrintMessage(gomock.Any(), gomock.Any()).AnyTimes()
			mockRunner.EXPECT().
				RunAtmos(gomock.Any()).
				DoAndReturn(func(params *AtmosExecParams) error {
					// Verify stack flag is included.
					if tt.expectedStack != "" {
						assert.Contains(t, params.Args, "-s")
						assert.Contains(t, params.Args, tt.expectedStack)
					}
					return nil
				})

			executor := NewExecutor(mockRunner, nil, mockUI)

			workflowDef := &schema.WorkflowDefinition{
				Stack: tt.workflowStack,
				Steps: []schema.WorkflowStep{
					{Name: "step1", Command: "terraform plan vpc", Type: "atmos", Stack: tt.stepStack},
				},
			}

			_, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{CommandLineStack: tt.commandLineStack}))

			require.NoError(t, err)
		})
	}
}

// TestExecutor_Execute_WithIdentity tests workflow execution with authentication.
func TestExecutor_Execute_WithIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockAuth := NewMockAuthProvider(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Setup auth expectations.
	// Note: baseEnv is now []string{} (from workflow/step env) instead of nil.
	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "test-identity").Return(nil, errors.New("no cache"))
	mockAuth.EXPECT().Authenticate(gomock.Any(), "test-identity").Return(nil)
	mockAuth.EXPECT().PrepareEnvironment(gomock.Any(), "test-identity", []string{}).Return([]string{"AWS_PROFILE=test"}, nil)

	// Expect shell command with auth env.
	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{"AWS_PROFILE=test"}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, mockAuth, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell", Identity: "test-identity"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_AuthenticationFailure tests handling of authentication failures.
func TestExecutor_Execute_AuthenticationFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockAuth := NewMockAuthProvider(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	authError := errors.New("authentication failed")

	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "bad-identity").Return(nil, errors.New("no cache"))
	mockAuth.EXPECT().Authenticate(gomock.Any(), "bad-identity").Return(authError)

	executor := NewExecutor(mockRunner, mockAuth, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell", Identity: "bad-identity"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.False(t, result.Success)
}

// TestExecutor_Execute_UserAborted tests handling of user abort during authentication.
func TestExecutor_Execute_UserAborted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockAuth := NewMockAuthProvider(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "test-identity").Return(nil, errors.New("no cache"))
	mockAuth.EXPECT().Authenticate(gomock.Any(), "test-identity").Return(errUtils.ErrUserAborted)

	executor := NewExecutor(mockRunner, mockAuth, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell", Identity: "test-identity"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
	assert.False(t, result.Success)
}

// TestExecutor_Execute_CommandLineIdentity tests command-line identity override.
func TestExecutor_Execute_CommandLineIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockAuth := NewMockAuthProvider(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Both steps should use the command-line identity.
	// Note: baseEnv is now []string{} (from workflow/step env) instead of nil.
	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "cli-identity").Return(nil, errors.New("no cache")).Times(2)
	mockAuth.EXPECT().Authenticate(gomock.Any(), "cli-identity").Return(nil).Times(2)
	mockAuth.EXPECT().PrepareEnvironment(gomock.Any(), "cli-identity", []string{}).Return([]string{"IDENTITY=cli"}, nil).Times(2)

	mockRunner.EXPECT().
		RunShell("echo 'step 1'", "test-workflow-step-0", ".", []string{"IDENTITY=cli"}, false).
		Return(nil)
	mockRunner.EXPECT().
		RunShell("echo 'step 2'", "test-workflow-step-1", ".", []string{"IDENTITY=cli"}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, mockAuth, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'step 1'", Type: "shell"}, // No identity.
			{Name: "step2", Command: "echo 'step 2'", Type: "shell"}, // No identity.
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{CommandLineIdentity: "cli-identity"}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_QuotedArguments tests that quoted arguments are parsed correctly.
func TestExecutor_Execute_QuotedArguments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Verify the query argument is properly parsed and not split on spaces.
	mockUI.EXPECT().PrintMessage(gomock.Any(), gomock.Any()).AnyTimes()
	mockRunner.EXPECT().
		RunAtmos(gomock.Any()).
		DoAndReturn(func(params *AtmosExecParams) error {
			// The query should be a single argument, not split.
			assert.Contains(t, params.Args, "--query")
			// Find the index of --query and check the next argument.
			for i, arg := range params.Args {
				if arg == "--query" && i+1 < len(params.Args) {
					// The entire query expression should be one argument.
					assert.Equal(t, `.metadata.component == "gcp/compute/v0.2.0"`, params.Args[i+1])
					break
				}
			}
			return nil
		})

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: `terraform plan --query '.metadata.component == "gcp/compute/v0.2.0"'`,
				Type:    "atmos",
			},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_DefaultStepType tests that empty step type defaults to "atmos".
func TestExecutor_Execute_DefaultStepType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Should call RunAtmos (not RunShell) for empty type.
	mockUI.EXPECT().PrintMessage(gomock.Any(), gomock.Any()).AnyTimes()
	mockRunner.EXPECT().
		RunAtmos(gomock.Any()).
		DoAndReturn(func(params *AtmosExecParams) error {
			assert.Equal(t, []string{"version"}, params.Args)
			return nil
		})

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "version"}, // No Type specified.
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_AutoGenerateStepNames tests automatic step name generation.
func TestExecutor_Execute_AutoGenerateStepNames(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Steps without names should get auto-generated names.
	mockRunner.EXPECT().RunShell("echo '1'", "test-workflow-step-0", ".", []string{}, false).Return(nil)
	mockRunner.EXPECT().RunShell("echo '2'", "test-workflow-step-1", ".", []string{}, false).Return(nil)

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Command: "echo '1'", Type: "shell"}, // No Name.
			{Command: "echo '2'", Type: "shell"}, // No Name.
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify step names were auto-generated.
	assert.Equal(t, "step1", result.Steps[0].StepName)
	assert.Equal(t, "step2", result.Steps[1].StepName)
}

// TestCheckAndGenerateWorkflowStepNames tests the step name generation function.
func TestCheckAndGenerateWorkflowStepNames(t *testing.T) {
	tests := []struct {
		name     string
		input    *schema.WorkflowDefinition
		expected []string
	}{
		{
			name: "all steps have names",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Name: "custom1"},
					{Name: "custom2"},
				},
			},
			expected: []string{"custom1", "custom2"},
		},
		{
			name: "no steps have names",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Command: "cmd1"},
					{Command: "cmd2"},
				},
			},
			expected: []string{"step1", "step2"},
		},
		{
			name: "mixed steps",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Name: "custom"},
					{Command: "cmd2"},
					{Name: "another"},
				},
			},
			expected: []string{"custom", "step2", "another"},
		},
		{
			name: "nil steps",
			input: &schema.WorkflowDefinition{
				Steps: nil,
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			CheckAndGenerateWorkflowStepNames(tt.input)

			if tt.expected == nil {
				assert.Nil(t, tt.input.Steps)
				return
			}

			for i, expectedName := range tt.expected {
				assert.Equal(t, expectedName, tt.input.Steps[i].Name)
			}
		})
	}
}

// TestExecutor_Execute_NilUIProvider tests that executor works without UI provider.
func TestExecutor_Execute_NilUIProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)

	// Expect shell command to be called.
	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{}, false).
		Return(nil)

	// Create executor with nil UI provider.
	executor := NewExecutor(mockRunner, nil, nil)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_NilUIProvider_Error tests that error handling works without UI provider.
func TestExecutor_Execute_NilUIProvider_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)

	// Create executor with nil UI provider.
	executor := NewExecutor(mockRunner, nil, nil)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowNoSteps)
	assert.False(t, result.Success)
}

// TestExecutor_Execute_StackBeforeDoubleDash tests stack insertion before -- separator.
func TestExecutor_Execute_StackBeforeDoubleDash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	mockUI.EXPECT().PrintMessage(gomock.Any(), gomock.Any()).AnyTimes()
	mockRunner.EXPECT().
		RunAtmos(gomock.Any()).
		DoAndReturn(func(params *AtmosExecParams) error {
			// Verify stack flag is inserted before --.
			// Expected: ["terraform", "plan", "vpc", "-s", "dev-stack", "--", "extra-arg"]
			assert.Equal(t, []string{"terraform", "plan", "vpc", "-s", "dev-stack", "--", "extra-arg"}, params.Args)
			return nil
		})

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "terraform plan vpc -- extra-arg", Type: "atmos"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{CommandLineStack: "dev-stack"}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_CachedCredentialsSuccess tests using cached credentials.
func TestExecutor_Execute_CachedCredentialsSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockAuth := NewMockAuthProvider(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Cached credentials are valid - no need to authenticate.
	// Note: baseEnv is now []string{} (from workflow/step env) instead of nil.
	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "cached-identity").Return("cached-creds", nil)
	mockAuth.EXPECT().PrepareEnvironment(gomock.Any(), "cached-identity", []string{}).Return([]string{"CACHED=true"}, nil)

	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{"CACHED=true"}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, mockAuth, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell", Identity: "cached-identity"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_PrepareEnvironmentFailure tests PrepareEnvironment failure.
func TestExecutor_Execute_PrepareEnvironmentFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockAuth := NewMockAuthProvider(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Auth succeeds but PrepareEnvironment fails.
	// Note: baseEnv is now []string{} (from workflow/step env) instead of nil.
	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "test-identity").Return(nil, errors.New("no cache"))
	mockAuth.EXPECT().Authenticate(gomock.Any(), "test-identity").Return(nil)
	mockAuth.EXPECT().PrepareEnvironment(gomock.Any(), "test-identity", []string{}).Return(nil, errors.New("prepare failed"))

	executor := NewExecutor(mockRunner, mockAuth, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell", Identity: "test-identity"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.False(t, result.Success)
}

// TestExecutor_Execute_AtmosCommandFailureWithStack tests atmos command type failure with stack.
func TestExecutor_Execute_AtmosCommandFailureWithStack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	stepError := errors.New("command failed")

	mockUI.EXPECT().PrintMessage(gomock.Any(), gomock.Any()).AnyTimes()
	mockRunner.EXPECT().RunAtmos(gomock.Any()).Return(stepError)
	mockUI.EXPECT().PrintError(gomock.Any(), workflowErrorTitle, gomock.Any())

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "terraform plan vpc", Type: "atmos"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{CommandLineStack: "dev-stack"}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkflowStepFailed)
	assert.False(t, result.Success)
	// Resume command should include the stack.
	assert.Contains(t, result.ResumeCommand, "-s dev-stack")
}

// TestExecutor_Execute_IdentityWithNoAuthProvider tests identity specified but no auth provider.
func TestExecutor_Execute_IdentityWithNoAuthProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// No auth provider, but step has identity - should still execute without auth.
	// Since authProvider is nil, prepareStepEnvironment returns empty env.
	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			// Identity is specified but since authProvider is nil, it's ignored.
			{Name: "step1", Command: "echo 'hello'", Type: "shell", Identity: "some-identity"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestExecutor_Execute_NilParams(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	executor := NewExecutor(mockRunner, nil, nil)

	result, err := executor.Execute(nil)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestExecutor_Execute_NilAtmosConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	executor := NewExecutor(mockRunner, nil, nil)

	// Create params with nil AtmosConfig.
	params := &WorkflowParams{
		Ctx:          context.Background(),
		AtmosConfig:  nil,
		Workflow:     "test-workflow",
		WorkflowPath: "test.yaml",
		WorkflowDefinition: &schema.WorkflowDefinition{
			Steps: []schema.WorkflowStep{
				{Name: "step1", Command: "echo hello", Type: "shell"},
			},
		},
	}

	result, err := executor.Execute(params)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

// TestExecutor_Execute_WorkingDirectory tests working_directory support.
func TestExecutor_Execute_WorkingDirectory(t *testing.T) {
	// Use OS-portable paths for cross-platform compatibility.
	// On Windows, paths like "/base" become "\base" which is NOT absolute.
	// We need to use proper absolute paths for each platform.
	var base, tmp, workflowDir, stepDir string
	if runtime.GOOS == "windows" {
		base = `C:\base`
		tmp = `C:\tmp`
		workflowDir = `C:\workflow-dir`
		stepDir = `C:\step-dir`
	} else {
		base = "/base"
		tmp = "/tmp"
		workflowDir = "/workflow-dir"
		stepDir = "/step-dir"
	}

	tests := []struct {
		name                 string
		workflowWorkDir      string
		stepWorkDir          string
		basePath             string
		expectedShellWorkDir string
		expectedAtmosWorkDir string
	}{
		{
			name:                 "no working directory",
			workflowWorkDir:      "",
			stepWorkDir:          "",
			basePath:             base,
			expectedShellWorkDir: ".",
			expectedAtmosWorkDir: ".",
		},
		{
			name:                 "workflow level absolute path",
			workflowWorkDir:      tmp,
			stepWorkDir:          "",
			basePath:             base,
			expectedShellWorkDir: tmp,
			expectedAtmosWorkDir: tmp,
		},
		{
			name:                 "workflow level relative path",
			workflowWorkDir:      "subdir",
			stepWorkDir:          "",
			basePath:             base,
			expectedShellWorkDir: filepath.Join(base, "subdir"),
			expectedAtmosWorkDir: filepath.Join(base, "subdir"),
		},
		{
			name:                 "step overrides workflow",
			workflowWorkDir:      workflowDir,
			stepWorkDir:          stepDir,
			basePath:             base,
			expectedShellWorkDir: stepDir,
			expectedAtmosWorkDir: stepDir,
		},
		{
			name:                 "step relative overrides workflow",
			workflowWorkDir:      workflowDir,
			stepWorkDir:          "step-subdir",
			basePath:             base,
			expectedShellWorkDir: filepath.Join(base, "step-subdir"),
			expectedAtmosWorkDir: filepath.Join(base, "step-subdir"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mocks.
			mockRunner := NewMockCommandRunner(ctrl)
			mockUI := NewMockUIProvider(ctrl)

			// Expect shell command with correct working directory.
			mockRunner.EXPECT().
				RunShell("echo shell", "test-workflow-step-0", tt.expectedShellWorkDir, []string{}, false).
				Return(nil)

			// Expect atmos command with correct working directory.
			mockRunner.EXPECT().
				RunAtmos(gomock.Any()).
				DoAndReturn(func(params *AtmosExecParams) error {
					assert.Equal(t, tt.expectedAtmosWorkDir, params.Dir, "atmos command working directory mismatch")
					return nil
				})

			mockUI.EXPECT().PrintMessage(gomock.Any(), gomock.Any()).AnyTimes()

			// Create executor.
			executor := NewExecutor(mockRunner, nil, mockUI)

			// Define workflow with working_directory.
			workflowDef := &schema.WorkflowDefinition{
				WorkingDirectory: tt.workflowWorkDir,
				Steps: []schema.WorkflowStep{
					{
						Name:             "shell-step",
						Command:          "echo shell",
						Type:             "shell",
						WorkingDirectory: tt.stepWorkDir,
					},
					{
						Name:             "atmos-step",
						Command:          "version",
						Type:             "atmos",
						WorkingDirectory: tt.stepWorkDir,
					},
				},
			}

			params := &WorkflowParams{
				Ctx:                context.Background(),
				AtmosConfig:        &schema.AtmosConfiguration{BasePath: tt.basePath},
				Workflow:           "test-workflow",
				WorkflowPath:       "test.yaml",
				WorkflowDefinition: workflowDef,
				Opts:               ExecuteOptions{},
			}

			// Execute.
			result, err := executor.Execute(params)

			require.NoError(t, err)
			assert.True(t, result.Success)
			assert.Len(t, result.Steps, 2)
		})
	}
}

// TestCalculateWorkingDirectory tests the calculateWorkingDirectory function directly.
func TestCalculateWorkingDirectory(t *testing.T) {
	// Use OS-portable paths for cross-platform compatibility.
	// On Windows, paths like "/base" become "\base" which is NOT absolute.
	// We need to use proper absolute paths for each platform.
	var base, absolutePath, workflowDirPath, stepDirPath string
	if runtime.GOOS == "windows" {
		base = `C:\base`
		absolutePath = `C:\absolute\path`
		workflowDirPath = `C:\workflow\dir`
		stepDirPath = `C:\step\dir`
	} else {
		base = "/base"
		absolutePath = "/absolute/path"
		workflowDirPath = "/workflow/dir"
		stepDirPath = "/step/dir"
	}

	tests := []struct {
		name        string
		workflowDir string
		stepDir     string
		basePath    string
		expected    string
	}{
		{
			name:        "empty returns empty",
			workflowDir: "",
			stepDir:     "",
			basePath:    base,
			expected:    "",
		},
		{
			name:        "workflow absolute path",
			workflowDir: absolutePath,
			stepDir:     "",
			basePath:    base,
			expected:    absolutePath,
		},
		{
			name:        "workflow relative path resolved against base",
			workflowDir: filepath.FromSlash("relative/path"),
			stepDir:     "",
			basePath:    base,
			expected:    filepath.Join(base, filepath.FromSlash("relative/path")),
		},
		{
			name:        "step overrides workflow",
			workflowDir: workflowDirPath,
			stepDir:     stepDirPath,
			basePath:    base,
			expected:    stepDirPath,
		},
		{
			name:        "step relative resolved against base",
			workflowDir: "",
			stepDir:     filepath.FromSlash("step/relative"),
			basePath:    base,
			expected:    filepath.Join(base, filepath.FromSlash("step/relative")),
		},
		{
			name:        "whitespace trimmed from absolute path",
			workflowDir: "  " + absolutePath + "  ",
			stepDir:     "",
			basePath:    base,
			expected:    absolutePath,
		},
	}

	executor := NewExecutor(nil, nil, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflowDef := &schema.WorkflowDefinition{
				WorkingDirectory: tt.workflowDir,
			}
			step := &schema.WorkflowStep{
				WorkingDirectory: tt.stepDir,
			}

			result := executor.calculateWorkingDirectory(workflowDef, step, tt.basePath)

			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExecutor_ensureToolchainDependencies_NoDeps tests early return when no dependencies.
func TestExecutor_ensureToolchainDependencies_NoDeps(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockDeps := NewMockDependencyProvider(ctrl)

	// Expect no .tool-versions and no workflow deps.
	mockDeps.EXPECT().LoadToolVersionsDependencies().Return(map[string]string{}, nil)
	mockDeps.EXPECT().ResolveWorkflowDependencies(gomock.Any()).Return(map[string]string{}, nil)
	mockDeps.EXPECT().MergeDependencies(map[string]string{}, map[string]string{}).Return(map[string]string{}, nil)
	// EnsureTools and UpdatePathForTools should NOT be called (early return).

	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, nil, nil).WithDependencyProvider(mockDeps)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_ensureToolchainDependencies_ToolVersionsOnly tests loading tools from .tool-versions.
func TestExecutor_ensureToolchainDependencies_ToolVersionsOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockDeps := NewMockDependencyProvider(ctrl)

	toolVersionsDeps := map[string]string{"jq": "1.7.1"}

	// Expect .tool-versions to be loaded and installed.
	mockDeps.EXPECT().LoadToolVersionsDependencies().Return(toolVersionsDeps, nil)
	mockDeps.EXPECT().ResolveWorkflowDependencies(gomock.Any()).Return(map[string]string{}, nil)
	mockDeps.EXPECT().MergeDependencies(toolVersionsDeps, map[string]string{}).Return(toolVersionsDeps, nil)
	mockDeps.EXPECT().EnsureTools(toolVersionsDeps).Return(nil)
	mockDeps.EXPECT().UpdatePathForTools(toolVersionsDeps).Return(nil)

	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, nil, nil).WithDependencyProvider(mockDeps)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_ensureToolchainDependencies_WorkflowDepsOnly tests workflow-specific dependencies.
func TestExecutor_ensureToolchainDependencies_WorkflowDepsOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockDeps := NewMockDependencyProvider(ctrl)

	workflowDeps := map[string]string{"terraform": "1.10.0"}

	// Expect workflow deps to be installed.
	mockDeps.EXPECT().LoadToolVersionsDependencies().Return(map[string]string{}, nil)
	mockDeps.EXPECT().ResolveWorkflowDependencies(gomock.Any()).Return(workflowDeps, nil)
	mockDeps.EXPECT().MergeDependencies(map[string]string{}, workflowDeps).Return(workflowDeps, nil)
	mockDeps.EXPECT().EnsureTools(workflowDeps).Return(nil)
	mockDeps.EXPECT().UpdatePathForTools(workflowDeps).Return(nil)

	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, nil, nil).WithDependencyProvider(mockDeps)

	workflowDef := &schema.WorkflowDefinition{
		Dependencies: &schema.Dependencies{
			Tools: workflowDeps,
		},
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_ensureToolchainDependencies_MergedDeps tests workflow deps override .tool-versions.
func TestExecutor_ensureToolchainDependencies_MergedDeps(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockDeps := NewMockDependencyProvider(ctrl)

	toolVersionsDeps := map[string]string{"jq": "1.7.0"}
	workflowDeps := map[string]string{"jq": "1.7.1"}
	mergedDeps := map[string]string{"jq": "1.7.1"} // Workflow overrides.

	// Expect merge to occur.
	mockDeps.EXPECT().LoadToolVersionsDependencies().Return(toolVersionsDeps, nil)
	mockDeps.EXPECT().ResolveWorkflowDependencies(gomock.Any()).Return(workflowDeps, nil)
	mockDeps.EXPECT().MergeDependencies(toolVersionsDeps, workflowDeps).Return(mergedDeps, nil)
	mockDeps.EXPECT().EnsureTools(mergedDeps).Return(nil)
	mockDeps.EXPECT().UpdatePathForTools(mergedDeps).Return(nil)

	mockRunner.EXPECT().
		RunShell("echo 'hello'", "test-workflow-step-0", ".", []string{}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, nil, nil).WithDependencyProvider(mockDeps)

	workflowDef := &schema.WorkflowDefinition{
		Dependencies: &schema.Dependencies{
			Tools: workflowDeps,
		},
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_ensureToolchainDependencies_LoadToolVersionsError tests .tool-versions load failure.
func TestExecutor_ensureToolchainDependencies_LoadToolVersionsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockDeps := NewMockDependencyProvider(ctrl)

	loadError := errors.New("failed to load .tool-versions")

	mockDeps.EXPECT().LoadToolVersionsDependencies().Return(nil, loadError)

	executor := NewExecutor(mockRunner, nil, nil).WithDependencyProvider(mockDeps)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDependencyResolution)
	assert.False(t, result.Success)
}

// TestExecutor_ensureToolchainDependencies_ResolveWorkflowError tests workflow resolution failure.
func TestExecutor_ensureToolchainDependencies_ResolveWorkflowError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockDeps := NewMockDependencyProvider(ctrl)

	resolveError := errors.New("failed to resolve workflow dependencies")

	mockDeps.EXPECT().LoadToolVersionsDependencies().Return(map[string]string{}, nil)
	mockDeps.EXPECT().ResolveWorkflowDependencies(gomock.Any()).Return(nil, resolveError)

	executor := NewExecutor(mockRunner, nil, nil).WithDependencyProvider(mockDeps)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDependencyResolution)
	assert.False(t, result.Success)
}

// TestExecutor_ensureToolchainDependencies_MergeError tests merge failure.
func TestExecutor_ensureToolchainDependencies_MergeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockDeps := NewMockDependencyProvider(ctrl)

	mergeError := errors.New("failed to merge dependencies")

	mockDeps.EXPECT().LoadToolVersionsDependencies().Return(map[string]string{"jq": "1.7.0"}, nil)
	mockDeps.EXPECT().ResolveWorkflowDependencies(gomock.Any()).Return(map[string]string{"jq": "1.7.1"}, nil)
	mockDeps.EXPECT().MergeDependencies(gomock.Any(), gomock.Any()).Return(nil, mergeError)

	executor := NewExecutor(mockRunner, nil, nil).WithDependencyProvider(mockDeps)

	workflowDef := &schema.WorkflowDefinition{
		Dependencies: &schema.Dependencies{
			Tools: map[string]string{"jq": "1.7.1"},
		},
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDependencyResolution)
	assert.False(t, result.Success)
}

// TestExecutor_ensureToolchainDependencies_InstallError tests tool installation failure.
func TestExecutor_ensureToolchainDependencies_InstallError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockDeps := NewMockDependencyProvider(ctrl)

	deps := map[string]string{"jq": "1.7.1"}
	installError := errors.New("failed to install jq")

	mockDeps.EXPECT().LoadToolVersionsDependencies().Return(deps, nil)
	mockDeps.EXPECT().ResolveWorkflowDependencies(gomock.Any()).Return(map[string]string{}, nil)
	mockDeps.EXPECT().MergeDependencies(gomock.Any(), gomock.Any()).Return(deps, nil)
	mockDeps.EXPECT().EnsureTools(deps).Return(installError)

	executor := NewExecutor(mockRunner, nil, nil).WithDependencyProvider(mockDeps)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolInstall)
	assert.False(t, result.Success)
}

// TestExecutor_ensureToolchainDependencies_UpdatePathError tests PATH update failure.
func TestExecutor_ensureToolchainDependencies_UpdatePathError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockDeps := NewMockDependencyProvider(ctrl)

	deps := map[string]string{"jq": "1.7.1"}
	pathError := errors.New("failed to update PATH")

	mockDeps.EXPECT().LoadToolVersionsDependencies().Return(deps, nil)
	mockDeps.EXPECT().ResolveWorkflowDependencies(gomock.Any()).Return(map[string]string{}, nil)
	mockDeps.EXPECT().MergeDependencies(gomock.Any(), gomock.Any()).Return(deps, nil)
	mockDeps.EXPECT().EnsureTools(deps).Return(nil)
	mockDeps.EXPECT().UpdatePathForTools(deps).Return(pathError)

	executor := NewExecutor(mockRunner, nil, nil).WithDependencyProvider(mockDeps)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo 'hello'", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDependencyResolution)
	assert.False(t, result.Success)
}

// TestMergeWorkflowEnv tests the mergeWorkflowEnv helper function.
func TestMergeWorkflowEnv(t *testing.T) {
	tests := []struct {
		name        string
		workflowEnv map[string]string
		stepEnv     map[string]string
		expected    map[string]string
	}{
		{
			name:        "both nil",
			workflowEnv: nil,
			stepEnv:     nil,
			expected:    nil,
		},
		{
			name:        "workflow env only",
			workflowEnv: map[string]string{"FOO": "bar"},
			stepEnv:     nil,
			expected:    map[string]string{"FOO": "bar"},
		},
		{
			name:        "step env only",
			workflowEnv: nil,
			stepEnv:     map[string]string{"BAZ": "qux"},
			expected:    map[string]string{"BAZ": "qux"},
		},
		{
			name:        "different keys merged",
			workflowEnv: map[string]string{"FOO": "bar"},
			stepEnv:     map[string]string{"BAZ": "qux"},
			expected:    map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:        "step overrides workflow",
			workflowEnv: map[string]string{"FOO": "workflow-value"},
			stepEnv:     map[string]string{"FOO": "step-value"},
			expected:    map[string]string{"FOO": "step-value"},
		},
		{
			name:        "mixed override and merge",
			workflowEnv: map[string]string{"FOO": "workflow", "BAR": "workflow"},
			stepEnv:     map[string]string{"FOO": "step", "BAZ": "step"},
			expected:    map[string]string{"FOO": "step", "BAR": "workflow", "BAZ": "step"},
		},
		{
			name:        "both empty",
			workflowEnv: map[string]string{},
			stepEnv:     map[string]string{},
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeWorkflowEnv(tt.workflowEnv, tt.stepEnv)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExecutor_Execute_WorkflowLevelEnv tests workflow-level env vars.
func TestExecutor_Execute_WorkflowLevelEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect shell command with workflow env vars.
	mockRunner.EXPECT().
		RunShell("echo test", "test-workflow-step-0", ".", []string{"MY_VAR=workflow-value"}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Env: map[string]string{"MY_VAR": "workflow-value"},
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo test", Type: "shell"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_StepLevelEnv tests step-level env vars.
func TestExecutor_Execute_StepLevelEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect shell command with step env vars.
	mockRunner.EXPECT().
		RunShell("echo test", "test-workflow-step-0", ".", []string{"STEP_VAR=step-value"}, false).
		Return(nil)

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo test",
				Type:    "shell",
				Env:     map[string]string{"STEP_VAR": "step-value"},
			},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_EnvPrecedence tests that step env overrides workflow env.
func TestExecutor_Execute_EnvPrecedence(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect step env value to override workflow env value.
	// Note: Map iteration order is not guaranteed, so we use gomock.Any()
	// and verify the content in a custom matcher.
	mockRunner.EXPECT().
		RunShell("echo test", "test-workflow-step-0", ".", gomock.Any(), false).
		DoAndReturn(func(command, name, dir string, env []string, dryRun bool) error {
			// Verify step value overrides workflow value.
			found := false
			for _, e := range env {
				if e == "MY_VAR=step-value" {
					found = true
				}
				// Should not have workflow value.
				assert.NotEqual(t, "MY_VAR=workflow-value", e)
			}
			assert.True(t, found, "Expected MY_VAR=step-value in env")
			return nil
		})

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Env: map[string]string{"MY_VAR": "workflow-value"},
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo test",
				Type:    "shell",
				Env:     map[string]string{"MY_VAR": "step-value"},
			},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_EnvMerge tests that workflow and step env are merged.
func TestExecutor_Execute_EnvMerge(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// Expect both workflow and step env vars to be present.
	mockRunner.EXPECT().
		RunShell("echo test", "test-workflow-step-0", ".", gomock.Any(), false).
		DoAndReturn(func(command, name, dir string, env []string, dryRun bool) error {
			// Verify both vars are present.
			hasWorkflowVar := false
			hasStepVar := false
			for _, e := range env {
				if e == "WORKFLOW_VAR=from-workflow" {
					hasWorkflowVar = true
				}
				if e == "STEP_VAR=from-step" {
					hasStepVar = true
				}
			}
			assert.True(t, hasWorkflowVar, "Expected WORKFLOW_VAR=from-workflow in env")
			assert.True(t, hasStepVar, "Expected STEP_VAR=from-step in env")
			return nil
		})

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Env: map[string]string{"WORKFLOW_VAR": "from-workflow"},
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo test",
				Type:    "shell",
				Env:     map[string]string{"STEP_VAR": "from-step"},
			},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_EnvWithIdentity tests env vars combined with identity auth.
func TestExecutor_Execute_EnvWithIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockAuth := NewMockAuthProvider(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	// The workflow/step env vars are passed to PrepareEnvironment as baseEnv.
	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "test-identity").Return(nil, errors.New("no cache"))
	mockAuth.EXPECT().Authenticate(gomock.Any(), "test-identity").Return(nil)
	mockAuth.EXPECT().
		PrepareEnvironment(gomock.Any(), "test-identity", gomock.Any()).
		DoAndReturn(func(ctx context.Context, identity string, baseEnv []string) ([]string, error) {
			// Verify workflow/step env is in baseEnv.
			hasWorkflowVar := false
			for _, e := range baseEnv {
				if e == "MY_VAR=workflow-value" {
					hasWorkflowVar = true
				}
			}
			assert.True(t, hasWorkflowVar, "Expected workflow env var in baseEnv")
			// Return auth env vars combined with baseEnv.
			return append(baseEnv, "AWS_PROFILE=test"), nil
		})

	// Expect command with both env vars.
	mockRunner.EXPECT().
		RunShell("echo test", "test-workflow-step-0", ".", gomock.Any(), false).
		DoAndReturn(func(command, name, dir string, env []string, dryRun bool) error {
			hasWorkflowVar := false
			hasAuthVar := false
			for _, e := range env {
				if e == "MY_VAR=workflow-value" {
					hasWorkflowVar = true
				}
				if e == "AWS_PROFILE=test" {
					hasAuthVar = true
				}
			}
			assert.True(t, hasWorkflowVar, "Expected MY_VAR=workflow-value in env")
			assert.True(t, hasAuthVar, "Expected AWS_PROFILE=test in env")
			return nil
		})

	executor := NewExecutor(mockRunner, mockAuth, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Env: map[string]string{"MY_VAR": "workflow-value"},
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "echo test", Type: "shell", Identity: "test-identity"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// TestExecutor_Execute_AtmosCommandWithEnv tests atmos command type with env vars.
func TestExecutor_Execute_AtmosCommandWithEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockCommandRunner(ctrl)
	mockUI := NewMockUIProvider(ctrl)

	mockUI.EXPECT().PrintMessage(gomock.Any(), gomock.Any()).AnyTimes()
	mockRunner.EXPECT().
		RunAtmos(gomock.Any()).
		DoAndReturn(func(params *AtmosExecParams) error {
			// Verify env vars are passed.
			hasEnvVar := false
			for _, e := range params.Env {
				if e == "TF_VAR_enabled=true" {
					hasEnvVar = true
				}
			}
			assert.True(t, hasEnvVar, "Expected TF_VAR_enabled=true in env")
			return nil
		})

	executor := NewExecutor(mockRunner, nil, mockUI)

	workflowDef := &schema.WorkflowDefinition{
		Env: map[string]string{"TF_VAR_enabled": "true"},
		Steps: []schema.WorkflowStep{
			{Name: "step1", Command: "terraform plan vpc", Type: "atmos"},
		},
	}

	result, err := executor.Execute(newTestParams(workflowDef, ExecuteOptions{}))

	require.NoError(t, err)
	assert.True(t, result.Success)
}
