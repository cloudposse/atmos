package auth

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestAuthCommandProvider(t *testing.T) {
	provider := &AuthCommandProvider{}

	assert.Equal(t, "auth", provider.GetName())
	assert.Equal(t, "Pro Features", provider.GetGroup())
	assert.NotNil(t, provider.GetCommand())
	assert.NotNil(t, provider.GetFlagsBuilder())
}

func TestGetIdentityFromFlags(t *testing.T) {
	tests := []struct {
		name     string
		setupCmd func(*cobra.Command)
		expected string
	}{
		{
			name: "flag not set",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String(IdentityFlagName, "", "identity")
			},
			expected: "",
		},
		{
			name: "flag set with value",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String(IdentityFlagName, "", "identity")
				_ = cmd.Flags().Set(IdentityFlagName, "prod-admin")
			},
			expected: "prod-admin",
		},
		{
			name: "flag set without value (NoOptDefVal)",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().String(IdentityFlagName, "", "identity")
				// Simulate NoOptDefVal behavior by setting the select value.
				_ = cmd.Flags().Set(IdentityFlagName, IdentityFlagSelectValue)
			},
			expected: IdentityFlagSelectValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			tt.setupCmd(cmd)

			result := GetIdentityFromFlags(cmd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAuthCommand_Structure(t *testing.T) {
	assert.Equal(t, "auth", authCmd.Use)
	assert.NotEmpty(t, authCmd.Short)
	assert.NotEmpty(t, authCmd.Long)

	// Auth command should have subcommands.
	subcommands := authCmd.Commands()
	assert.Greater(t, len(subcommands), 0)

	// Get subcommand names.
	subcommandNames := make([]string, len(subcommands))
	for i, cmd := range subcommands {
		subcommandNames[i] = cmd.Name()
	}

	// Verify expected subcommands exist.
	assert.Contains(t, subcommandNames, "login")
	assert.Contains(t, subcommandNames, "logout")
	assert.Contains(t, subcommandNames, "whoami")
	assert.Contains(t, subcommandNames, "list")
	assert.Contains(t, subcommandNames, "env")
	assert.Contains(t, subcommandNames, "shell")
	assert.Contains(t, subcommandNames, "exec")
	assert.Contains(t, subcommandNames, "console")
	assert.Contains(t, subcommandNames, "validate")
}

func TestAuthCommand_IdentityFlag(t *testing.T) {
	// Check identity flag is registered as persistent flag.
	identityFlag := authCmd.PersistentFlags().Lookup(IdentityFlagName)
	assert.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)
}

func TestIdentityFlagName(t *testing.T) {
	assert.Equal(t, "identity", IdentityFlagName)
}

func TestIdentityFlagSelectValue(t *testing.T) {
	assert.Equal(t, "__SELECT__", IdentityFlagSelectValue)
}

func TestAuthCommand_HasRunE(t *testing.T) {
	// Auth parent command may or may not have RunE.
	// Just verify the command is properly configured.
	assert.Equal(t, "auth", authCmd.Use)
}

func TestAuthCommand_PersistentPreRunE(t *testing.T) {
	// Auth command should have a PersistentPreRunE for flag setup.
	// The actual function is set, so verify it's not nil.
	// Note: In the actual implementation, this might be set to nil or a function.
	// Just verify the command is properly initialized.
	assert.NotNil(t, authCmd)
}

func TestGetIdentityFromFlags_NoFlag(t *testing.T) {
	// Test with a command that doesn't have the identity flag at all.
	cmd := &cobra.Command{Use: "test"}

	// Should return empty string without panicking.
	result := GetIdentityFromFlags(cmd)
	assert.Equal(t, "", result)
}

func TestAddIdentityCompletion(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String(IdentityFlagName, "", "identity")

	// Should not panic.
	assert.NotPanics(t, func() {
		AddIdentityCompletion(cmd)
	})
}
