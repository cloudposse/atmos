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
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
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
				Use:                "exec",
				DisableFlagParsing: true,
			}
			testCmd.Flags().AddFlagSet(authExecCmd.Flags())

			// Capture output.
			var buf bytes.Buffer
			testCmd.SetOut(&buf)
			testCmd.SetErr(&buf)

			// Call the core business logic directly, bypassing handleHelpRequest and checkAtmosConfig.
			err := executeAuthExecCommandCore(testCmd, tt.args)

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
	assert.True(t, authExecCmd.DisableFlagParsing, "DisableFlagParsing should be true to allow pass-through of command flags")

	// Verify identity flag exists (inherited from parent authCmd).
	identityFlag := authExecCmd.Flag("identity")
	require.NotNil(t, identityFlag, "identity flag should be inherited from parent authCmd")
	assert.Equal(t, "i", identityFlag.Shorthand)
	assert.Equal(t, "", identityFlag.DefValue)
	assert.Equal(t, IdentityFlagSelectValue, identityFlag.NoOptDefVal, "NoOptDefVal should be __SELECT__")
}

func TestExtractIdentityFlag(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		expectedIdentity    string
		expectedCommandArgs []string
	}{
		{
			name:                "no flags, just command",
			args:                []string{"echo", "hello"},
			expectedIdentity:    "",
			expectedCommandArgs: []string{"echo", "hello"},
		},
		{
			name:                "identity with value, double dash, command",
			args:                []string{"--identity=test-user", "--", "echo", "hello"},
			expectedIdentity:    "test-user",
			expectedCommandArgs: []string{"echo", "hello"},
		},
		{
			name:                "identity equals syntax",
			args:                []string{"--identity=test-user", "--", "echo", "hello"},
			expectedIdentity:    "test-user",
			expectedCommandArgs: []string{"echo", "hello"},
		},
		{
			name:                "identity flag without value before double dash",
			args:                []string{"--identity", "--", "echo", "hello"},
			expectedIdentity:    IdentityFlagSelectValue,
			expectedCommandArgs: []string{"echo", "hello"},
		},
		{
			name:                "identity flag without value, no double dash",
			args:                []string{"--identity", "echo", "hello"},
			expectedIdentity:    "echo",
			expectedCommandArgs: []string{"hello"},
		},
		{
			name:                "short flag -i with value",
			args:                []string{"-i", "test-user", "--", "aws", "s3", "ls"},
			expectedIdentity:    "test-user",
			expectedCommandArgs: []string{"aws", "s3", "ls"},
		},
		{
			name:                "short flag -i without value before double dash",
			args:                []string{"-i", "--", "aws", "s3", "ls"},
			expectedIdentity:    IdentityFlagSelectValue,
			expectedCommandArgs: []string{"aws", "s3", "ls"},
		},
		{
			name:                "double dash with no identity flag",
			args:                []string{"--", "echo", "hello"},
			expectedIdentity:    "",
			expectedCommandArgs: []string{"echo", "hello"},
		},
		{
			name:                "identity equals empty string",
			args:                []string{"--identity=", "--", "echo", "hello"},
			expectedIdentity:    IdentityFlagSelectValue,
			expectedCommandArgs: []string{"echo", "hello"},
		},
		{
			name:                "no double dash, identity with value",
			args:                []string{"--identity=test-user", "terraform", "plan"},
			expectedIdentity:    "test-user",
			expectedCommandArgs: []string{"terraform", "plan"},
		},
		{
			name:                "identity at end with no value",
			args:                []string{"echo", "hello", "--identity"},
			expectedIdentity:    IdentityFlagSelectValue,
			expectedCommandArgs: []string{"echo", "hello"},
		},
		{
			name:                "empty args",
			args:                []string{},
			expectedIdentity:    "",
			expectedCommandArgs: nil,
		},
		{
			name:                "only identity flag",
			args:                []string{"--identity"},
			expectedIdentity:    IdentityFlagSelectValue,
			expectedCommandArgs: nil,
		},
		{
			name:                "only double dash",
			args:                []string{"--"},
			expectedIdentity:    "",
			expectedCommandArgs: nil, // No args after "--"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, commandArgs := extractIdentityFlag(tt.args)
			assert.Equal(t, tt.expectedIdentity, identity, "identity value mismatch")
			assert.Equal(t, tt.expectedCommandArgs, commandArgs, "command args mismatch")
		})
	}
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

func TestPrintAuthExecTip(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
	}{
		{
			name:         "shows tip with identity name",
			identityName: "test-identity",
		},
		{
			name:         "shows tip with different identity name",
			identityName: "dev-admin",
		},
		{
			name:         "handles empty identity name",
			identityName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize I/O context for UI layer.
			ioCtx, err := iolib.NewContext()
			require.NoError(t, err)
			data.InitWriter(ioCtx)
			ui.InitFormatter(ioCtx)

			// Call the function - it should not panic.
			// The actual output formatting is tested by the UI layer tests.
			// We verify the function executes without error with the identity name.
			assert.NotPanics(t, func() {
				printAuthExecTip(tt.identityName)
			})
		})
	}
}
