package workflow

import (
	"context"
	"errors"
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
	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "test-identity").Return(nil, errors.New("no cache"))
	mockAuth.EXPECT().Authenticate(gomock.Any(), "test-identity").Return(nil)
	mockAuth.EXPECT().PrepareEnvironment(gomock.Any(), "test-identity", nil).Return([]string{"AWS_PROFILE=test"}, nil)

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
	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "cli-identity").Return(nil, errors.New("no cache")).Times(2)
	mockAuth.EXPECT().Authenticate(gomock.Any(), "cli-identity").Return(nil).Times(2)
	mockAuth.EXPECT().PrepareEnvironment(gomock.Any(), "cli-identity", nil).Return([]string{"IDENTITY=cli"}, nil).Times(2)

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
	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "cached-identity").Return("cached-creds", nil)
	mockAuth.EXPECT().PrepareEnvironment(gomock.Any(), "cached-identity", nil).Return([]string{"CACHED=true"}, nil)

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
	mockAuth.EXPECT().GetCachedCredentials(gomock.Any(), "test-identity").Return(nil, errors.New("no cache"))
	mockAuth.EXPECT().Authenticate(gomock.Any(), "test-identity").Return(nil)
	mockAuth.EXPECT().PrepareEnvironment(gomock.Any(), "test-identity", nil).Return(nil, errors.New("prepare failed"))

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
			basePath:             "/base",
			expectedShellWorkDir: ".",
			expectedAtmosWorkDir: ".",
		},
		{
			name:                 "workflow level absolute path",
			workflowWorkDir:      "/tmp",
			stepWorkDir:          "",
			basePath:             "/base",
			expectedShellWorkDir: "/tmp",
			expectedAtmosWorkDir: "/tmp",
		},
		{
			name:                 "workflow level relative path",
			workflowWorkDir:      "subdir",
			stepWorkDir:          "",
			basePath:             "/base",
			expectedShellWorkDir: "/base/subdir",
			expectedAtmosWorkDir: "/base/subdir",
		},
		{
			name:                 "step overrides workflow",
			workflowWorkDir:      "/workflow-dir",
			stepWorkDir:          "/step-dir",
			basePath:             "/base",
			expectedShellWorkDir: "/step-dir",
			expectedAtmosWorkDir: "/step-dir",
		},
		{
			name:                 "step relative overrides workflow",
			workflowWorkDir:      "/workflow-dir",
			stepWorkDir:          "step-subdir",
			basePath:             "/base",
			expectedShellWorkDir: "/base/step-subdir",
			expectedAtmosWorkDir: "/base/step-subdir",
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
			basePath:    "/base",
			expected:    "",
		},
		{
			name:        "workflow absolute path",
			workflowDir: "/absolute/path",
			stepDir:     "",
			basePath:    "/base",
			expected:    "/absolute/path",
		},
		{
			name:        "workflow relative path resolved against base",
			workflowDir: "relative/path",
			stepDir:     "",
			basePath:    "/base",
			expected:    "/base/relative/path",
		},
		{
			name:        "step overrides workflow",
			workflowDir: "/workflow/dir",
			stepDir:     "/step/dir",
			basePath:    "/base",
			expected:    "/step/dir",
		},
		{
			name:        "step relative resolved against base",
			workflowDir: "",
			stepDir:     "step/relative",
			basePath:    "/base",
			expected:    "/base/step/relative",
		},
		{
			name:        "whitespace trimmed",
			workflowDir: "  /with/spaces  ",
			stepDir:     "",
			basePath:    "/base",
			expected:    "/with/spaces",
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
