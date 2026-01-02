package auth

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

// Main identity resolution tests are in identity_resolution_test.go.

func TestGetSeparatedArgsForExec(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*cobra.Command)
		expected []string
	}{
		{
			name: "no separator",
			setup: func(cmd *cobra.Command) {
				// No args set.
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			tt.setup(cmd)

			result := getSeparatedArgsForExec(cmd)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestExecuteCommandWithEnv_Validation(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		envVars       map[string]string
		expectedError error
	}{
		{
			name:          "empty args returns error",
			args:          []string{},
			envVars:       nil,
			expectedError: errUtils.ErrNoCommandSpecified,
		},
		{
			name:          "nil args returns error",
			args:          nil,
			envVars:       nil,
			expectedError: errUtils.ErrNoCommandSpecified,
		},
		{
			name:          "command not found",
			args:          []string{"nonexistent-command-that-does-not-exist-12345"},
			envVars:       nil,
			expectedError: errUtils.ErrCommandNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executeCommandWithEnv(tt.args, tt.envVars)

			assert.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedError)
		})
	}
}

func TestAuthExecCommand_Structure(t *testing.T) {
	assert.Equal(t, "exec -- <command> [args...]", authExecCmd.Use)
	assert.NotEmpty(t, authExecCmd.Short)
	assert.NotEmpty(t, authExecCmd.Long)
	assert.NotEmpty(t, authExecCmd.Example)
	assert.NotNil(t, authExecCmd.RunE)
}

func TestExecParser_Initialization(t *testing.T) {
	// execParser should be initialized in init().
	assert.NotNil(t, execParser)
}

func TestAuthExecCommand_FParseErrWhitelist(t *testing.T) {
	// Verify FParseErrWhitelist is configured.
	assert.False(t, authExecCmd.FParseErrWhitelist.UnknownFlags)
}

func TestResolveIdentityNameForExec_ViperFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthManager := authTypes.NewMockAuthManager(ctrl)

	// Create test command with no flag set.
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String(IdentityFlagName, "", "identity")

	// Set identity in viper as fallback.
	v := viper.New()
	v.Set(IdentityFlagName, "viper-identity")

	result, err := resolveIdentityNameForExec(cmd, v, mockAuthManager)

	assert.NoError(t, err)
	assert.Equal(t, "viper-identity", result)
}

func TestGetSeparatedArgsForExec_EmptyCommand(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	result := getSeparatedArgsForExec(cmd)

	assert.Nil(t, result)
}

func TestExecuteCommandWithEnv_WithValidCommand(t *testing.T) {
	// Test with a valid command that exits quickly.
	err := executeCommandWithEnv([]string{"true"}, map[string]string{
		"TEST_VAR": "test_value",
	})

	// "true" command should succeed.
	assert.NoError(t, err)
}

func TestExecuteCommandWithEnv_WithFailingCommand(t *testing.T) {
	// Test with a command that always fails.
	err := executeCommandWithEnv([]string{"false"}, nil)

	// "false" command should return non-zero exit code.
	assert.Error(t, err)
}
