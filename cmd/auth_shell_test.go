package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthShellCmd_FlagParsing(t *testing.T) {
	// Set up test fixture with auth configuration.
	testDir := "../tests/fixtures/scenarios/atmos-auth"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	tests := []struct {
		name          string
		args          []string
		expectedError string
	}{
		{
			name: "no identity specified uses default",
			args: []string{},
			// This will fail with auth errors since we don't have real AWS SSO configured.
			expectedError: "authentication failed",
		},
		{
			name:          "nonexistent identity",
			args:          []string{"--identity", "nonexistent"},
			expectedError: "identity not found",
		},
		{
			name: "valid identity",
			args: []string{"--identity", "test-user"},
			// This will fail with auth errors since we don't have real AWS credentials.
			expectedError: "authentication failed",
		},
		{
			name: "shell override flag",
			args: []string{"--shell", "/bin/bash"},
			// This will fail with auth errors since we don't have real AWS credentials.
			expectedError: "authentication failed",
		},
		{
			name: "shell args after double dash",
			args: []string{"--", "-c", "echo test"},
			// This will fail with auth errors since we don't have real AWS credentials.
			expectedError: "authentication failed",
		},
		{
			name: "identity with shell args",
			args: []string{"--identity", "test-user", "--", "-c", "env"},
			// This will fail with auth errors since we don't have real AWS credentials.
			expectedError: "authentication failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a command instance with the same flags as the real authShellCmd.
			testCmd := &cobra.Command{
				Use:                "shell",
				DisableFlagParsing: true,
			}
			testCmd.Flags().AddFlagSet(authShellCmd.Flags())

			// Call the core business logic directly, bypassing handleHelpRequest and checkAtmosConfig.
			err := executeAuthShellCommandCore(testCmd, tt.args)

			if tt.expectedError != "" {
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthShellCmd_CommandStructure(t *testing.T) {
	// Test that the real authShellCmd has the expected structure.
	assert.Equal(t, "shell", authShellCmd.Use)
	assert.True(t, authShellCmd.DisableFlagParsing, "DisableFlagParsing should be true to allow pass-through of shell arguments")

	// Verify identity flag exists.
	identityFlag := authShellCmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag, "identity flag should be registered")
	assert.Equal(t, "i", identityFlag.Shorthand)
	assert.Equal(t, "", identityFlag.DefValue)

	// Verify shell flag exists.
	shellFlag := authShellCmd.Flags().Lookup("shell")
	require.NotNil(t, shellFlag, "shell flag should be registered")
	assert.Equal(t, "", shellFlag.DefValue)
}
