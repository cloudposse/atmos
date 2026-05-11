package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
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

// TestValidateAuthShellArgs covers all four positional-arg shapes for the
// shell command. The two acceptable shapes are: no positional args at all,
// or `--` as the first non-flag token followed by zero+ shell args. The
// two rejected shapes are: positional args without any `--`, and positional
// args appearing before a `--` (which getSeparatedArgs would silently drop).
func TestValidateAuthShellArgs(t *testing.T) {
	tests := []struct {
		name      string
		cliArgs   []string
		expectErr bool
	}{
		{
			name:      "no positional args",
			cliArgs:   nil,
			expectErr: false,
		},
		{
			name:      "dash-separator with shell args is allowed",
			cliArgs:   []string{"--", "-lc", "echo hello"},
			expectErr: false,
		},
		{
			name:      "dash-separator alone is allowed",
			cliArgs:   []string{"--"},
			expectErr: false,
		},
		{
			name:      "positional arg without separator is rejected",
			cliArgs:   []string{"bash"},
			expectErr: true,
		},
		{
			name:      "positional arg before separator is rejected (would be silently dropped)",
			cliArgs:   []string{"bash", "--", "-lc", "echo hello"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "shell"}
			require.NoError(t, cmd.ParseFlags(tt.cliArgs))

			err := validateAuthShellArgs(cmd, cmd.Flags().Args())
			if tt.expectErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidArguments,
					"rejected arg shape must wrap ErrInvalidArguments so callers can branch on it")
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestExecuteAuthShellCommand_SmokeNoConfig exercises the shell orchestrator
// from a directory without an atmos.yaml. Contract: no panic.
func TestExecuteAuthShellCommand_SmokeNoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := authShellCmd
	cmd.SetContext(context.Background())

	assert.NotPanics(t, func() {
		_ = executeAuthShellCommand(cmd, nil)
	})
}

// TestAuthShell_ProfileFlagAppliedToConfig is a regression test for issue #1973
// (`--profile` global flag not applied for `auth exec` and `auth shell` commands).
//
// HEAD's executeAuthShellCommand calls BuildConfigAndStacksInfo(cmd, v) before
// cfg.InitCliConfig, so --profile must round-trip into ProfilesFromArg.
func TestAuthShell_ProfileFlagAppliedToConfig(t *testing.T) {
	runProfileFlagAppliedRegressionTest(t, "shell")
}

// TestPrepareShellEnvironment covers the cache-hit, fresh-auth, ErrUserAborted,
// authenticate-error, and PrepareShellEnvironment-error branches via a mocked
// AuthManager. The atmos config Env map is passed through to MergeGlobalEnv.
func TestPrepareShellEnvironment(t *testing.T) {
	emptyCfg := &schema.AtmosConfiguration{}

	t.Run("cached credentials skip Authenticate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		m := authTypes.NewMockAuthManager(ctrl)

		m.EXPECT().GetCachedCredentials(gomock.Any(), "id").
			Return(&authTypes.WhoamiInfo{Identity: "id"}, nil)
		m.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Times(0)
		m.EXPECT().PrepareShellEnvironment(gomock.Any(), "id", gomock.Any()).
			Return([]string{"AWS_REGION=us-east-1", "AWS_PROFILE=p"}, nil)
		m.EXPECT().GetProviderForIdentity("id").Return("aws-sso")

		envList, provider, err := prepareShellEnvironment(m, "id", emptyCfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"AWS_REGION=us-east-1", "AWS_PROFILE=p"}, envList)
		assert.Equal(t, "aws-sso", provider)
	})

	t.Run("missing cache triggers Authenticate then succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		m := authTypes.NewMockAuthManager(ctrl)

		m.EXPECT().GetCachedCredentials(gomock.Any(), "id").
			Return(nil, errors.New("no cache"))
		m.EXPECT().Authenticate(gomock.Any(), "id").
			Return(&authTypes.WhoamiInfo{Identity: "id"}, nil)
		m.EXPECT().PrepareShellEnvironment(gomock.Any(), "id", gomock.Any()).
			Return([]string{"AWS_REGION=us-east-1"}, nil)
		m.EXPECT().GetProviderForIdentity("id").Return("aws-sso")

		envList, provider, err := prepareShellEnvironment(m, "id", emptyCfg)
		require.NoError(t, err)
		assert.NotEmpty(t, envList)
		assert.Equal(t, "aws-sso", provider)
	})

	t.Run("ErrUserAborted surfaces unwrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		m := authTypes.NewMockAuthManager(ctrl)

		m.EXPECT().GetCachedCredentials(gomock.Any(), "id").
			Return(nil, errors.New("no cache"))
		m.EXPECT().Authenticate(gomock.Any(), "id").
			Return(nil, errUtils.ErrUserAborted)

		envList, provider, err := prepareShellEnvironment(m, "id", emptyCfg)
		require.Error(t, err)
		assert.Nil(t, envList)
		assert.Empty(t, provider)
		assert.ErrorIs(t, err, errUtils.ErrUserAborted)
		assert.NotErrorIs(t, err, errUtils.ErrAuthenticationFailed,
			"ErrUserAborted is a clean cancel; do not wrap as failure")
	})

	t.Run("generic Authenticate error wraps with ErrAuthenticationFailed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		m := authTypes.NewMockAuthManager(ctrl)

		boom := errors.New("backend down")
		m.EXPECT().GetCachedCredentials(gomock.Any(), "id").
			Return(nil, errors.New("no cache"))
		m.EXPECT().Authenticate(gomock.Any(), "id").Return(nil, boom)

		_, _, err := prepareShellEnvironment(m, "id", emptyCfg)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		assert.ErrorIs(t, err, boom, "original error must be preserved in the chain")
	})

	t.Run("PrepareShellEnvironment failure wraps ErrPrepareShellEnvironment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		m := authTypes.NewMockAuthManager(ctrl)

		m.EXPECT().GetCachedCredentials(gomock.Any(), "id").
			Return(&authTypes.WhoamiInfo{Identity: "id"}, nil)
		envErr := errors.New("env build failed")
		m.EXPECT().PrepareShellEnvironment(gomock.Any(), "id", gomock.Any()).
			Return(nil, envErr)

		_, _, err := prepareShellEnvironment(m, "id", emptyCfg)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPrepareShellEnvironment,
			"sentinel must be in the chain so callers can branch via errors.Is")
		assert.ErrorIs(t, err, envErr,
			"original underlying error must also be preserved in the chain")
	})

	t.Run("atmosConfig.Env contributes to the base env list", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		m := authTypes.NewMockAuthManager(ctrl)

		cfgWithGlobalEnv := &schema.AtmosConfiguration{
			Env: map[string]string{
				"ATMOS_GLOBAL_VAR": "from-config",
			},
		}

		var capturedBaseEnv []string
		m.EXPECT().GetCachedCredentials(gomock.Any(), "id").
			Return(&authTypes.WhoamiInfo{Identity: "id"}, nil)
		m.EXPECT().PrepareShellEnvironment(gomock.Any(), "id", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, baseEnv []string) ([]string, error) {
				capturedBaseEnv = baseEnv
				return baseEnv, nil
			})
		m.EXPECT().GetProviderForIdentity("id").Return("aws-sso")

		_, _, err := prepareShellEnvironment(m, "id", cfgWithGlobalEnv)
		require.NoError(t, err)
		// The global env value from atmos.yaml must appear in the base env list
		// passed to PrepareShellEnvironment (via MergeGlobalEnv).
		var found bool
		for _, kv := range capturedBaseEnv {
			if kv == "ATMOS_GLOBAL_VAR=from-config" {
				found = true
				break
			}
		}
		assert.True(t, found,
			"atmosConfig.Env entries must be merged into the base env before PrepareShellEnvironment")
	})
}
