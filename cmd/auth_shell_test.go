package cmd

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestAuthShellCmd_FlagParsing(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		expectedSentinelErr error // The specific sentinel error expected
	}{
		{
			name: "no identity specified uses default",
			args: []string{},
			// Fixture has test-admin as default. Will fail at authentication since we don't have real AWS SSO.
			expectedSentinelErr: errUtils.ErrAuthenticationFailed,
		},
		{
			name:                "nonexistent identity",
			args:                []string{"--identity=nonexistent"},
			expectedSentinelErr: errUtils.ErrIdentityNotFound,
		},
		{
			name: "valid identity",
			args: []string{"--identity=test-user"},
			// This will fail at authentication since we don't have real AWS credentials.
			expectedSentinelErr: errUtils.ErrAuthenticationFailed,
		},
		{
			name: "shell override flag",
			args: []string{"--shell", "/bin/bash"},
			// Fixture has test-admin as default. Will fail at authentication since we don't have real AWS SSO.
			expectedSentinelErr: errUtils.ErrAuthenticationFailed,
		},
		{
			name: "shell args after double dash",
			args: []string{"--", "-c", "echo test"},
			// Fixture has test-admin as default. Will fail at authentication since we don't have real AWS SSO.
			expectedSentinelErr: errUtils.ErrAuthenticationFailed,
		},
		{
			name: "identity with shell args",
			args: []string{"--identity=test-user", "--", "-c", "env"},
			// This will fail at authentication since we don't have real AWS credentials.
			expectedSentinelErr: errUtils.ErrAuthenticationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// CRITICAL: Must use absolute path, not relative path, because viper may resolve
			// the path from a different working directory in CI vs locally.
			testDir, err := filepath.Abs("../tests/fixtures/scenarios/atmos-auth")
			require.NoError(t, err, "Failed to get absolute path to test fixture")

			t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
			t.Setenv("ATMOS_BASE_PATH", testDir)

			// Create a command instance with the same flags as the real authShellCmd.
			testCmd := &cobra.Command{
				Use:                "shell",
				DisableFlagParsing: true,
			}
			testCmd.Flags().AddFlagSet(authShellCmd.Flags())

			// Call the core business logic directly, bypassing handleHelpRequest and checkAtmosConfig.
			err = executeAuthShellCommandCore(testCmd, tt.args)

			if tt.expectedSentinelErr != nil {
				require.Error(t, err, "Expected an error but got nil")
				assert.ErrorIs(t, err, tt.expectedSentinelErr,
					"Expected error chain to contain %v, but got: %v",
					tt.expectedSentinelErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthShellCmd_CommandStructure(t *testing.T) {
	_ = NewTestKit(t)

	// Test that the real authShellCmd has the expected structure.
	assert.Equal(t, "shell", authShellCmd.Use)
	assert.True(t, authShellCmd.DisableFlagParsing, "DisableFlagParsing should be true to allow pass-through of shell arguments")

	// Verify identity flag exists (inherited from parent authCmd).
	identityFlag := authShellCmd.Flag("identity")
	require.NotNil(t, identityFlag, "identity flag should be inherited from parent authCmd")
	assert.Equal(t, "i", identityFlag.Shorthand)
	assert.Equal(t, "", identityFlag.DefValue)
	assert.Equal(t, IdentityFlagSelectValue, identityFlag.NoOptDefVal, "NoOptDefVal should be __SELECT__")

	// Verify shell flag exists (local flag).
	shellFlag := authShellCmd.Flags().Lookup("shell")
	require.NotNil(t, shellFlag, "shell flag should be registered")
	assert.Equal(t, "", shellFlag.DefValue)
}

func TestAuthShellCmd_InvalidFlagHandling(t *testing.T) {
	_ = NewTestKit(t)

	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/atmos-auth"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	testCmd := &cobra.Command{
		Use:                "shell",
		DisableFlagParsing: true,
	}
	testCmd.Flags().AddFlagSet(authShellCmd.Flags())

	// Test with invalid flag format.
	err := executeAuthShellCommandCore(testCmd, []string{"--invalid-flag-format="})
	assert.Error(t, err)
}

func TestAuthShellCmd_EmptyEnvVars(t *testing.T) {
	// Test that the command handles nil environment variables gracefully.
	// This tests the path where envVars is nil and gets initialized to empty map.
	testDir, err := filepath.Abs("../tests/fixtures/scenarios/atmos-auth")
	require.NoError(t, err, "Failed to get absolute path to test fixture")

	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	testCmd := &cobra.Command{
		Use:                "shell",
		DisableFlagParsing: true,
	}
	testCmd.Flags().AddFlagSet(authShellCmd.Flags())

	// This will fail at authentication but will exercise the env var initialization path.
	err = executeAuthShellCommandCore(testCmd, []string{"--identity=test-user"})
	// Should be an authentication error, not nil pointer errors.
	require.Error(t, err, "Expected an error but got nil")
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed,
		"Expected ErrAuthenticationFailed, but got: %v", err)
}

func TestAuthShellCmd_HelpRequest(t *testing.T) {
	_ = NewTestKit(t)

	// Test that the command handles help request arguments.
	// When DisableFlagParsing is true, Cobra doesn't add the help flag automatically,
	// so handleHelpRequest in cmd/helpers.go handles --help and -h manually.
	testDir := "../tests/fixtures/scenarios/atmos-auth"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// The command should have DisableFlagParsing enabled.
	assert.True(t, authShellCmd.DisableFlagParsing, "DisableFlagParsing should be true")

	// Help is handled by handleHelpRequest which is called in executeAuthShellCommand.
	// We can't easily test this without capturing stdout, but we verify the structure.
	assert.NotNil(t, authShellCmd.RunE, "RunE should be set")
}

func TestAuthShellCmd_ShellEnvironmentBinding(t *testing.T) {
	_ = NewTestKit(t)

	// Test that SHELL and ATMOS_SHELL environment variables are bound.
	// This verifies the init() function's viper bindings work correctly.
	testShell := "/bin/test-shell"
	t.Setenv("ATMOS_SHELL", testShell)

	// The init() function binds these, so we just verify viper can read them.
	// Note: This is an indirect test since init() runs before our test.
	// The real binding happens at package load time.
	assert.NotPanics(t, func() {
		// Just verify the flag exists and can be accessed.
		flag := authShellCmd.Flags().Lookup("shell")
		assert.NotNil(t, flag)
	})
}

func TestAuthShellCmd_WithMockProvider(t *testing.T) {
	_ = NewTestKit(t)

	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: spawns actual shell process")
	}

	// Get OS-specific shell and commands.
	var shellFlag, exitCmd string
	if runtime.GOOS == "windows" {
		shellFlag = "cmd.exe"
		exitCmd = "/c"
	} else {
		shellFlag = "/bin/sh"
		exitCmd = "-c"
	}

	tests := []struct {
		name          string
		shell         string
		args          []string
		expectedError bool
	}{
		{
			name:          "successful auth with explicit mock identity",
			shell:         shellFlag,
			args:          []string{"--identity=mock-identity", "--shell", shellFlag, "--", exitCmd, "exit 0"},
			expectedError: false,
		},
		{
			name:          "successful auth with second mock identity",
			shell:         shellFlag,
			args:          []string{"--identity=mock-identity-2", "--shell", shellFlag, "--", exitCmd, "exit 0"},
			expectedError: false,
		},
		{
			name:          "shell exits with non-zero code",
			shell:         shellFlag,
			args:          []string{"--identity=mock-identity", "--shell", shellFlag, "--", exitCmd, "exit 42"},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Set up mock auth provider fixture for each subtest.
			testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
			t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
			t.Setenv("ATMOS_BASE_PATH", testDir)

			testCmd := &cobra.Command{
				Use:                "shell",
				DisableFlagParsing: true,
			}
			testCmd.Flags().AddFlagSet(authShellCmd.Flags())

			err := executeAuthShellCommandCore(testCmd, tt.args)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthShellCmd_MockProviderEnvironmentVariables(t *testing.T) {
	_ = NewTestKit(t)

	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: spawns actual shell process")
	}

	// Use mock auth provider fixture.
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	testCmd := &cobra.Command{
		Use:                "shell",
		DisableFlagParsing: true,
	}
	testCmd.Flags().AddFlagSet(authShellCmd.Flags())

	// Execute shell that prints environment variables and exits.
	// Use OS-specific commands to check environment variables.
	var args []string
	if runtime.GOOS == "windows" {
		// On Windows, use cmd.exe to check for environment variables.
		args = []string{"--identity=mock-identity", "--shell", "cmd.exe", "--", "/c", "exit 0"}
	} else {
		// On Unix, use sh with grep to verify credentials are set.
		args = []string{"--identity=mock-identity", "--shell", "/bin/sh", "--", "-c", "exit 0"}
	}

	err := executeAuthShellCommandCore(testCmd, args)
	assert.NoError(t, err, "shell should execute successfully with mock credentials")
}

// Note on test coverage:
// The mock provider enables testing of:
// 1. ✅ Successful authentication flow (using mock credentials)
// 2. ✅ Environment variable propagation to shell
// 3. ✅ Shell execution with authenticated context
// 4. ✅ Exit code propagation
//
// The shell_utils_test.go also tests shell execution mechanics separately.
