package auth

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

// Main identity resolution tests are in identity_resolution_test.go.

func TestGetSeparatedArgsForExec(t *testing.T) {
	tests := []struct {
		name string
		// args is the raw CLI arg list passed to cmd.ParseFlags; the helper
		// reads cmd.Flags().Args() + cmd.Flags().ArgsLenAtDash() to find the
		// segment after `--`.
		args     []string
		expected []string
	}{
		{
			name:     "no separator and no positional args",
			args:     nil,
			expected: nil,
		},
		{
			name:     "positional args without separator are dropped",
			args:     []string{"some-stray-arg"},
			expected: nil,
		},
		{
			name:     "single command after separator",
			args:     []string{"--", "aws"},
			expected: []string{"aws"},
		},
		{
			name:     "command with arguments after separator",
			args:     []string{"--", "aws", "sts", "get-caller-identity"},
			expected: []string{"aws", "sts", "get-caller-identity"},
		},
		{
			name:     "separator with no following args yields nil",
			args:     []string{"--"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			require.NoError(t, cmd.ParseFlags(tt.args))

			result := getSeparatedArgsForExec(cmd)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecuteCommandWithEnv_Validation(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		envList       []string
		expectedError error
	}{
		{
			name:          "empty args returns error",
			args:          []string{},
			envList:       nil,
			expectedError: errUtils.ErrNoCommandSpecified,
		},
		{
			name:          "nil args returns error",
			args:          nil,
			envList:       nil,
			expectedError: errUtils.ErrNoCommandSpecified,
		},
		{
			name:          "command not found",
			args:          []string{"nonexistent-command-that-does-not-exist-12345"},
			envList:       nil,
			expectedError: errUtils.ErrCommandNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executeCommandWithEnv(tt.args, tt.envList)

			assert.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedError)

			// Regression guard for the "atmos requires a subcommand" bug: a missing
			// external executable must report ErrCommandNotFound and must NEVER be
			// classified as an unknown Atmos subcommand, or the root error handler
			// masks it with root usage output.
			if errors.Is(tt.expectedError, errUtils.ErrCommandNotFound) {
				assert.NotErrorIs(t, err, errUtils.ErrUnknownSubcommand)
			}
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

func TestExecuteCommandWithEnv_WithValidCommand(t *testing.T) {
	// Cross-platform: spawn the test binary itself with the exit-OK env flag
	// (handled by TestMain). This avoids dependence on PATH-resolved binaries
	// like `go` / `true` which aren't available on every CI runner.
	exe, err := os.Executable()
	require.NoError(t, err)

	err = executeCommandWithEnv([]string{exe}, []string{"_ATMOS_AUTH_TEST_EXIT_OK=1"})
	assert.NoError(t, err)
}

func TestExecuteCommandWithEnv_NonZeroExit(t *testing.T) {
	// Cross-platform exit-1 subprocess: test binary + flag, expect ExitCodeError.
	exe, err := os.Executable()
	require.NoError(t, err)

	err = executeCommandWithEnv([]string{exe}, []string{"_ATMOS_AUTH_TEST_EXIT_ONE=1"})
	require.Error(t, err)
	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr,
		"non-zero subprocess exit must surface as errUtils.ExitCodeError so the root can propagate the code")
	assert.Equal(t, 1, exitErr.Code)
}

// TestExecuteAuthExecCommand_SmokeNoConfig exercises the exec orchestrator
// from a directory without an atmos.yaml. Contract: no panic.
func TestExecuteAuthExecCommand_SmokeNoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := authExecCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())

	assert.NotPanics(t, func() {
		_ = executeAuthExecCommand(cmd, nil)
	})
}

// TestExecuteAuthExecCommand_NoCommand covers the validation branch where
// the user invokes `atmos auth exec` without a `--` separator + command.
// Must return ErrNoCommandSpecified before authenticating.
func TestExecuteAuthExecCommand_NoCommand(t *testing.T) {
	setupMockAuthFixture(t)

	cmd := authExecCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags(nil))

	err := executeAuthExecCommand(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoCommandSpecified,
		"no command args must surface ErrNoCommandSpecified")
}

// TestPrepareAuthenticatedEnv_SmokeNoConfig exercises the exec helper from a
// directory without an atmos.yaml.
func TestPrepareAuthenticatedEnv_SmokeNoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := authExecCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())
	v := viper.New()

	assert.NotPanics(t, func() {
		_, _ = prepareAuthenticatedEnv(cmd, v)
	})
}

// TestPrepareAuthenticatedEnv_WithMockAuth exercises the full happy path
// against the mock auth fixture. Drives config load, auth manager creation,
// identity resolution, cache check, authentication, and PrepareShellEnvironment.
func TestPrepareAuthenticatedEnv_WithMockAuth(t *testing.T) {
	setupMockAuthFixture(t)

	cmd := authExecCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags(nil))
	v := viper.GetViper()

	execCtx, err := prepareAuthenticatedEnv(cmd, v)
	require.NoError(t, err,
		"prepareAuthenticatedEnv against the mock provider must run to completion")
	require.NotNil(t, execCtx)
	assert.NotEmpty(t, execCtx.envList,
		"the returned env list must include the OS env merged with auth vars")

	// The mock identity sets AWS_PROFILE=mock-identity and AWS_REGION=us-east-1.
	var awsProfile, awsRegion string
	for _, kv := range execCtx.envList {
		switch {
		case len(kv) > len("AWS_PROFILE=") && kv[:len("AWS_PROFILE=")] == "AWS_PROFILE=":
			awsProfile = kv[len("AWS_PROFILE="):]
		case len(kv) > len("AWS_REGION=") && kv[:len("AWS_REGION=")] == "AWS_REGION=":
			awsRegion = kv[len("AWS_REGION="):]
		}
	}
	assert.Equal(t, "mock-identity", awsProfile,
		"mock provider must inject AWS_PROFILE")
	assert.Equal(t, "us-east-1", awsRegion,
		"mock provider must inject AWS_REGION")
}

// TestAuthExec_ProfileFlagAppliedToConfig is a regression test for issue #1973
// (`--profile` global flag not applied for `auth exec` and `auth shell` commands).
//
// Before the cmd/auth/* refactor, executeAuthExecCommandCore loaded the atmos
// config from an empty schema.ConfigAndStacksInfo{} which silently dropped the
// --profile flag. The new flow runs through BuildConfigAndStacksInfo(cmd, v)
// which honours --profile, --base-path, --config and --config-path.
//
// This test asserts that the helper used by `auth exec` actually surfaces the
// --profile flag value into the ConfigAndStacksInfo used to InitCliConfig.
func TestAuthExec_ProfileFlagAppliedToConfig(t *testing.T) {
	runProfileFlagAppliedRegressionTest(t, "exec")
}
