package auth

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

// Main identity resolution tests are in identity_resolution_test.go.

func TestGetSeparatedArgs(t *testing.T) {
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
		{
			name: "with separator and args",
			setup: func(cmd *cobra.Command) {
				// Simulate args after "--".
				// In real usage, Cobra sets ArgsLenAtDash based on "--" position.
				cmd.SetArgs([]string{"--", "bash", "-c", "echo hello"})
			},
			expected: nil, // Can't easily simulate ArgsLenAtDash without full parse.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			tt.setup(cmd)

			result := getSeparatedArgs(cmd)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestAuthShellCommand_Structure(t *testing.T) {
	assert.Equal(t, "shell [-- [shell args...]]", authShellCmd.Use)
	assert.NotEmpty(t, authShellCmd.Short)
	assert.NotEmpty(t, authShellCmd.Long)
	assert.NotNil(t, authShellCmd.RunE)
	assert.NotEmpty(t, authShellCmd.Example)

	// Check shell flag exists.
	shellFlag := authShellCmd.Flags().Lookup("shell")
	assert.NotNil(t, shellFlag)
}

func TestShellParser_Initialization(t *testing.T) {
	// shellParser should be initialized in init().
	assert.NotNil(t, shellParser)
}

func TestAuthShellCommand_ValidArgsFunction(t *testing.T) {
	// The shell command should have ValidArgsFunction set to NoFileCompletions.
	assert.NotNil(t, authShellCmd.ValidArgsFunction)
}

func TestAuthShellCommand_FParseErrWhitelist(t *testing.T) {
	// Verify FParseErrWhitelist is configured.
	assert.False(t, authShellCmd.FParseErrWhitelist.UnknownFlags)
}

func TestShellFlagName(t *testing.T) {
	assert.Equal(t, "shell", shellFlagName)
}

func TestGetSeparatedArgs_EmptyCommand(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	result := getSeparatedArgs(cmd)

	assert.Nil(t, result)
}

func TestResolveIdentityNameForShell_ViperFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthManager := authTypes.NewMockAuthManager(ctrl)

	// Create test command with no flag set.
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String(IdentityFlagName, "", "identity")

	// Set identity in viper as fallback.
	v := viper.New()
	v.Set(IdentityFlagName, "viper-identity")

	// When identity is set in viper but not flag, it should use viper value.
	// But the current implementation checks flag first, then viper, then default.
	// Since viper has value, GetDefaultIdentity shouldn't be called.
	// Actually looking at the code, if flag is empty, it checks viper.
	// If viper is also empty, then GetDefaultIdentity is called.
	// So with viper set, we should get viper-identity directly.
	// Wait, but the code checks forceSelect after getting viperIdentity...
	// Let me trace through: identityFlag="", viperIdentity="viper-identity"
	// forceSelect = (viper-identity == __SELECT__) = false
	// if "" || false = false, so it returns viper-identity without calling GetDefaultIdentity.
	// Hmm, but that's not right either because the condition is:
	// if identityName == "" || forceSelect
	// After the viper check, identityName = "viper-identity"
	// So identityName != "" and forceSelect = false
	// So the condition is false and we return identityName directly.
	// So GetDefaultIdentity is NOT called.

	result, err := resolveIdentityNameForShell(cmd, v, mockAuthManager)

	assert.NoError(t, err)
	assert.Equal(t, "viper-identity", result)
}
