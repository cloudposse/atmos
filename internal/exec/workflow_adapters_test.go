package exec

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/workflow"
)

// TestWorkflowCommandRunner_InterfaceCompliance verifies that WorkflowCommandRunner
// implements the workflow.CommandRunner interface.
func TestWorkflowCommandRunner_InterfaceCompliance(t *testing.T) {
	var _ workflow.CommandRunner = (*WorkflowCommandRunner)(nil)
}

// TestWorkflowAuthProvider_InterfaceCompliance verifies that WorkflowAuthProvider
// implements the workflow.AuthProvider interface.
func TestWorkflowAuthProvider_InterfaceCompliance(t *testing.T) {
	var _ workflow.AuthProvider = (*WorkflowAuthProvider)(nil)
}

// TestWorkflowUIProvider_InterfaceCompliance verifies that WorkflowUIProvider
// implements the workflow.UIProvider interface.
func TestWorkflowUIProvider_InterfaceCompliance(t *testing.T) {
	var _ workflow.UIProvider = (*WorkflowUIProvider)(nil)
}

// TestNewWorkflowCommandRunner tests the constructor.
func TestNewWorkflowCommandRunner(t *testing.T) {
	tests := []struct {
		name        string
		retryConfig *schema.RetryConfig
	}{
		{
			name:        "nil retry config",
			retryConfig: nil,
		},
		{
			name: "with retry config",
			retryConfig: &schema.RetryConfig{
				MaxAttempts: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewWorkflowCommandRunner(tt.retryConfig)
			assert.NotNil(t, runner)
		})
	}
}

// TestNewWorkflowAuthProvider tests the constructor.
func TestNewWorkflowAuthProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	provider := NewWorkflowAuthProvider(mockManager)

	assert.NotNil(t, provider)
	assert.Equal(t, mockManager, provider.manager)
}

// TestNewWorkflowUIProvider tests the constructor.
func TestNewWorkflowUIProvider(t *testing.T) {
	provider := NewWorkflowUIProvider()
	assert.NotNil(t, provider)
}

// TestWorkflowAuthProvider_NeedsAuth tests the NeedsAuth method.
func TestWorkflowAuthProvider_NeedsAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	provider := NewWorkflowAuthProvider(mockManager)

	tests := []struct {
		name                string
		steps               []schema.WorkflowStep
		commandLineIdentity string
		expected            bool
	}{
		{
			name:                "command line identity specified",
			steps:               []schema.WorkflowStep{},
			commandLineIdentity: "my-identity",
			expected:            true,
		},
		{
			name: "step has identity",
			steps: []schema.WorkflowStep{
				{Name: "step1", Command: "echo hello"},
				{Name: "step2", Command: "echo world", Identity: "step-identity"},
			},
			commandLineIdentity: "",
			expected:            true,
		},
		{
			name: "no identity anywhere",
			steps: []schema.WorkflowStep{
				{Name: "step1", Command: "echo hello"},
				{Name: "step2", Command: "echo world"},
			},
			commandLineIdentity: "",
			expected:            false,
		},
		{
			name:                "empty steps no identity",
			steps:               []schema.WorkflowStep{},
			commandLineIdentity: "",
			expected:            false,
		},
		{
			name: "first step has identity",
			steps: []schema.WorkflowStep{
				{Name: "step1", Command: "terraform plan", Identity: "admin"},
			},
			commandLineIdentity: "",
			expected:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.NeedsAuth(tt.steps, tt.commandLineIdentity)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestWorkflowAuthProvider_Authenticate tests the Authenticate method.
func TestWorkflowAuthProvider_Authenticate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		identity      string
		setupMock     func(*types.MockAuthManager)
		expectedError bool
	}{
		{
			name:     "successful authentication",
			identity: "test-identity",
			setupMock: func(m *types.MockAuthManager) {
				m.EXPECT().
					Authenticate(gomock.Any(), "test-identity").
					Return(&types.WhoamiInfo{Identity: "test-identity"}, nil)
			},
			expectedError: false,
		},
		{
			name:     "authentication failure",
			identity: "bad-identity",
			setupMock: func(m *types.MockAuthManager) {
				m.EXPECT().
					Authenticate(gomock.Any(), "bad-identity").
					Return(nil, errors.New("authentication failed"))
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMock(mockManager)

			provider := NewWorkflowAuthProvider(mockManager)
			err := provider.Authenticate(context.Background(), tt.identity)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestWorkflowAuthProvider_GetCachedCredentials tests the GetCachedCredentials method.
func TestWorkflowAuthProvider_GetCachedCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		identity      string
		setupMock     func(*types.MockAuthManager)
		expectedError bool
		expectedCreds any
	}{
		{
			name:     "cached credentials found",
			identity: "cached-identity",
			setupMock: func(m *types.MockAuthManager) {
				m.EXPECT().
					GetCachedCredentials(gomock.Any(), "cached-identity").
					Return(&types.WhoamiInfo{Identity: "cached-identity"}, nil)
			},
			expectedError: false,
			expectedCreds: &types.WhoamiInfo{Identity: "cached-identity"},
		},
		{
			name:     "no cached credentials",
			identity: "uncached-identity",
			setupMock: func(m *types.MockAuthManager) {
				m.EXPECT().
					GetCachedCredentials(gomock.Any(), "uncached-identity").
					Return(nil, errors.New("no cached credentials"))
			},
			expectedError: true,
			expectedCreds: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMock(mockManager)

			provider := NewWorkflowAuthProvider(mockManager)
			creds, err := provider.GetCachedCredentials(context.Background(), tt.identity)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, creds)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, creds)
			}
		})
	}
}

// TestWorkflowAuthProvider_PrepareEnvironment tests the PrepareEnvironment method.
func TestWorkflowAuthProvider_PrepareEnvironment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		identity      string
		baseEnv       []string
		setupMock     func(*types.MockAuthManager, []string)
		expectedError bool
		checkResult   func(t *testing.T, result []string)
	}{
		{
			name:     "prepare with base env",
			identity: "test-identity",
			baseEnv:  []string{"EXISTING=value"},
			setupMock: func(m *types.MockAuthManager, baseEnv []string) {
				m.EXPECT().
					PrepareShellEnvironment(gomock.Any(), "test-identity", baseEnv).
					Return([]string{"EXISTING=value", "AWS_PROFILE=test"}, nil)
			},
			expectedError: false,
			checkResult: func(t *testing.T, result []string) {
				assert.Contains(t, result, "EXISTING=value")
				assert.Contains(t, result, "AWS_PROFILE=test")
			},
		},
		{
			name:     "prepare with nil base env uses os.Environ",
			identity: "test-identity",
			baseEnv:  nil,
			setupMock: func(m *types.MockAuthManager, _ []string) {
				// When baseEnv is nil, it should use os.Environ().
				m.EXPECT().
					PrepareShellEnvironment(gomock.Any(), "test-identity", gomock.Any()).
					DoAndReturn(func(ctx context.Context, identity string, env []string) ([]string, error) {
						// Verify it received a non-empty environment (from os.Environ).
						if len(env) == 0 {
							return nil, errors.New("expected non-empty environment")
						}
						return append(env, "AWS_PROFILE=test"), nil
					})
			},
			expectedError: false,
			checkResult: func(t *testing.T, result []string) {
				assert.Contains(t, result, "AWS_PROFILE=test")
				// Should have more than just our added var since it uses os.Environ.
				assert.Greater(t, len(result), 1)
			},
		},
		{
			name:     "prepare environment failure",
			identity: "bad-identity",
			baseEnv:  []string{},
			setupMock: func(m *types.MockAuthManager, baseEnv []string) {
				m.EXPECT().
					PrepareShellEnvironment(gomock.Any(), "bad-identity", baseEnv).
					Return(nil, errors.New("failed to prepare environment"))
			},
			expectedError: true,
			checkResult:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMock(mockManager, tt.baseEnv)

			provider := NewWorkflowAuthProvider(mockManager)
			result, err := provider.PrepareEnvironment(context.Background(), tt.identity, tt.baseEnv)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

// TestWorkflowUIProvider_PrintMessage tests the PrintMessage method.
func TestWorkflowUIProvider_PrintMessage(t *testing.T) {
	provider := NewWorkflowUIProvider()

	// Capture stderr to verify output.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	provider.PrintMessage("Hello %s!", "World")

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)

	assert.Equal(t, "Hello World!", buf.String())
}

// TestWorkflowUIProvider_PrintMessage_NoArgs tests PrintMessage with no format args.
func TestWorkflowUIProvider_PrintMessage_NoArgs(t *testing.T) {
	provider := NewWorkflowUIProvider()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	provider.PrintMessage("Simple message")

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)

	assert.Equal(t, "Simple message", buf.String())
}

// TestWorkflowUIProvider_PrintError tests the PrintError method.
func TestWorkflowUIProvider_PrintError(t *testing.T) {
	provider := NewWorkflowUIProvider()

	// PrintError writes to stderr via CheckErrorAndPrint.
	// We just verify it doesn't panic.
	testErr := errors.New("test error")
	assert.NotPanics(t, func() {
		provider.PrintError(testErr, "Test Title", "Test explanation")
	})
}

// TestWorkflowCommandRunner_RunShell_DryRun tests the RunShell method in dry-run mode.
func TestWorkflowCommandRunner_RunShell_DryRun(t *testing.T) {
	runner := NewWorkflowCommandRunner(nil)

	// Dry run should not actually execute the command.
	err := runner.RunShell("echo hello", "test-command", ".", []string{}, true)
	assert.NoError(t, err)
}

// TestWorkflowCommandRunner_RunShell_InvalidCommand tests RunShell with an invalid command.
func TestWorkflowCommandRunner_RunShell_InvalidCommand(t *testing.T) {
	runner := NewWorkflowCommandRunner(nil)

	// This should fail because the command doesn't exist.
	err := runner.RunShell("nonexistent_command_12345", "test-command", ".", []string{}, false)
	// Invalid commands should return an error.
	assert.Error(t, err)
}

// TestWorkflowCommandRunner_RunShell_WithEnv tests RunShell with environment variables.
func TestWorkflowCommandRunner_RunShell_WithEnv(t *testing.T) {
	runner := NewWorkflowCommandRunner(nil)

	env := []string{"TEST_VAR=test_value"}
	// Dry run to avoid actually executing.
	err := runner.RunShell("echo $TEST_VAR", "test-env", ".", env, true)
	assert.NoError(t, err)
}

// TestWorkflowCommandRunner_RunAtmos_DryRun tests the RunAtmos method in dry-run mode.
func TestWorkflowCommandRunner_RunAtmos_DryRun(t *testing.T) {
	runner := NewWorkflowCommandRunner(nil)

	params := &workflow.AtmosExecParams{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Args:        []string{"version"},
		Dir:         ".",
		Env:         []string{},
		DryRun:      true,
	}

	// Dry run should not actually execute the command.
	err := runner.RunAtmos(params)
	assert.NoError(t, err)
}

// TestWorkflowCommandRunner_RunAtmos_NilParams tests RunAtmos with nil params.
func TestWorkflowCommandRunner_RunAtmos_NilParams(t *testing.T) {
	runner := NewWorkflowCommandRunner(nil)

	err := runner.RunAtmos(nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

// TestWorkflowCommandRunner_RunAtmos_NilAtmosConfig tests RunAtmos with nil AtmosConfig.
func TestWorkflowCommandRunner_RunAtmos_NilAtmosConfig(t *testing.T) {
	runner := NewWorkflowCommandRunner(nil)

	params := &workflow.AtmosExecParams{
		Ctx:         context.Background(),
		AtmosConfig: nil,
		Args:        []string{"version"},
		Dir:         ".",
		Env:         []string{},
		DryRun:      true,
	}

	err := runner.RunAtmos(params)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

// TestWorkflowAuthProvider_NeedsAuth_EmptyIdentityString tests edge case with whitespace identity.
func TestWorkflowAuthProvider_NeedsAuth_EmptyIdentityString(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	provider := NewWorkflowAuthProvider(mockManager)

	// Empty string identity should not trigger auth.
	steps := []schema.WorkflowStep{
		{Name: "step1", Command: "echo hello", Identity: ""},
	}
	result := provider.NeedsAuth(steps, "")
	assert.False(t, result)
}

// TestWorkflowAuthProvider_NeedsAuth_WhitespaceIdentity tests that whitespace-only identity is
// treated as having an identity (the provider doesn't trim).
func TestWorkflowAuthProvider_NeedsAuth_WhitespaceIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	provider := NewWorkflowAuthProvider(mockManager)

	// Whitespace-only identity - the NeedsAuth checks for non-empty string.
	steps := []schema.WorkflowStep{
		{Name: "step1", Command: "echo hello", Identity: "  "},
	}
	// This will be true because "  " != "".
	result := provider.NeedsAuth(steps, "")
	assert.True(t, result)
}

// TestWorkflowUIProvider_PrintMessage_MultipleArgs tests PrintMessage with multiple format args.
func TestWorkflowUIProvider_PrintMessage_MultipleArgs(t *testing.T) {
	provider := NewWorkflowUIProvider()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	provider.PrintMessage("Step %d of %d: %s", 1, 3, "running")

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)

	assert.Equal(t, "Step 1 of 3: running", buf.String())
}

// TestWorkflowAuthProvider_PrepareEnvironment_EmptyBaseEnv tests with empty (not nil) base env.
func TestWorkflowAuthProvider_PrepareEnvironment_EmptyBaseEnv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	mockManager.EXPECT().
		PrepareShellEnvironment(gomock.Any(), "test-identity", []string{}).
		Return([]string{"AWS_PROFILE=test"}, nil)

	provider := NewWorkflowAuthProvider(mockManager)
	result, err := provider.PrepareEnvironment(context.Background(), "test-identity", []string{})

	assert.NoError(t, err)
	assert.Equal(t, []string{"AWS_PROFILE=test"}, result)
}
