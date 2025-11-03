package cmd

import (
	"bytes"
	"errors"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestAuthExecCmd_FlagParsing(t *testing.T) {
	// Set up test fixture with auth configuration.
	testDir := "../tests/fixtures/scenarios/atmos-auth"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	tests := []struct {
		name          string
		args          []string
		skipOnWindows bool
		expectedError string
	}{
		{
			name:          "identity flag without command",
			args:          []string{"--identity=test-user"},
			expectedError: "no command specified",
		},
		{
			name:          "double dash without command",
			args:          []string{"--"},
			expectedError: "no command specified",
		},
		{
			name:          "identity flag with double dash but no command",
			args:          []string{"--identity=test-user", "--"},
			expectedError: "no command specified",
		},
		{
			name:          "nonexistent identity",
			args:          []string{"--identity=nonexistent", "--", "echo", "test"},
			expectedError: "identity not found",
		},
		{
			name:          "identity flag with no value before double dash",
			args:          []string{"--identity", "--", "echo", "test"},
			expectedError: "requires a TTY", // Interactive selection requires TTY.
		},
		{
			name: "valid command with default identity",
			args: []string{"echo", "test"},
			// This will fail with auth errors since we don't have real AWS SSO configured.
			expectedError: "authentication failed",
		},
		{
			name:          "valid command with specific identity and double dash",
			args:          []string{"--identity=test-user", "--", "echo", "hello"},
			skipOnWindows: true, // echo behaves differently on Windows
			// This will fail with auth errors since we don't have real AWS credentials.
			expectedError: "authentication failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping test on Windows: echo command behaves differently")
			}

			// Create a command instance with the same flags as the real authExecCmd.
			testCmd := &cobra.Command{
				Use: "exec",
			}
			testCmd.Flags().AddFlagSet(authExecCmd.Flags())

			// Capture output.
			var buf bytes.Buffer
			testCmd.SetOut(&buf)
			testCmd.SetErr(&buf)

			// Call the core business logic directly, bypassing handleHelpRequest and checkAtmosConfig.
			err := executeAuthExecCommandCore(tt.args)

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

func TestAuthExecCmd_CommandStructure(t *testing.T) {
	// Test that the real authExecCmd has the expected structure.
	assert.Equal(t, "exec", authExecCmd.Use)

	// Verify identity flag exists (registered via PassThroughFlagParser).
	identityFlag := authExecCmd.Flag("identity")
	require.NotNil(t, identityFlag, "identity flag should be registered via PassThroughFlagParser")
	assert.Equal(t, "i", identityFlag.Shorthand)
	assert.Equal(t, "", identityFlag.DefValue)
	assert.Equal(t, IdentityFlagSelectValue, identityFlag.NoOptDefVal, "NoOptDefVal should be __SELECT__")
}

func TestExecuteCommandWithEnv(t *testing.T) {
	// Test the command execution helper directly.
	tests := []struct {
		name          string
		args          []string
		envVars       map[string]string
		skipOnWindows bool
		expectedError string
		expectedCode  int // Expected exit code if error is ExitCodeError
	}{
		{
			name:          "empty args",
			args:          []string{},
			envVars:       map[string]string{},
			expectedError: "no command specified",
		},
		{
			name: "simple echo command",
			args: []string{"echo", "hello"},
			envVars: map[string]string{
				"TEST_VAR": "test-value",
			},
			skipOnWindows: true,
		},
		{
			name:          "nonexistent command",
			args:          []string{"nonexistent-command-xyz"},
			envVars:       map[string]string{},
			expectedError: "command not found",
		},
		{
			name: "command with non-zero exit code",
			args: []string{"sh", "-c", "exit 2"},
			envVars: map[string]string{
				"TEST_VAR": "test-value",
			},
			skipOnWindows: true,
			expectedCode:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping test on Windows: command behaves differently")
			}

			err := executeCommandWithEnv(tt.args, tt.envVars)

			switch {
			case tt.expectedError != "":
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			case tt.expectedCode != 0:
				assert.Error(t, err)
				// Check that it's an ExitCodeError with the correct code
				var exitCodeErr errUtils.ExitCodeError
				if assert.True(t, errors.As(err, &exitCodeErr), "error should be ExitCodeError") {
					assert.Equal(t, tt.expectedCode, exitCodeErr.Code)
				}
			default:
				assert.NoError(t, err)
			}
		})
	}
}
