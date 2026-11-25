package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

func TestIsIdentityDisabled(t *testing.T) {
	for _, value := range []string{"false", "FALSE", "0", "no", "Off", "__DISABLED__"} {
		assert.True(t, isIdentityDisabled(value), value)
	}
	for _, value := range []string{"", "prod", "falsey", IdentityFlagSelectValue} {
		assert.False(t, isIdentityDisabled(value), value)
	}
	assert.True(t, identityRequest{flag: "false"}.disabled())
	assert.True(t, identityRequest{env: "off"}.disabled())
	assert.False(t, identityRequest{flag: "prod", env: "false"}.disabled(), "an explicit identity wins over ATMOS_IDENTITY")
}

// identityResolver is the identity resolution step of an auth subcommand that
// cannot run without an identity.
type identityResolver func(cmd *cobra.Command, authManager auth.AuthManager) (string, error)

func disabledIdentityResolvers() map[string]identityResolver {
	return map[string]identityResolver{
		"whoami": identityFromFlagOrDefault,
		"env": func(cmd *cobra.Command, m auth.AuthManager) (string, error) {
			return resolveIdentityNameForEnv(cmd, viper.GetViper(), m)
		},
		"shell": func(cmd *cobra.Command, m auth.AuthManager) (string, error) {
			return resolveIdentityNameForShell(cmd, viper.GetViper(), m)
		},
		"console": func(cmd *cobra.Command, m auth.AuthManager) (string, error) {
			return resolveIdentityName(cmd, viper.GetViper(), m)
		},
		"login": func(cmd *cobra.Command, m auth.AuthManager) (string, error) {
			whoami, _, err := authenticateIdentity(context.Background(), cmd, m)
			if whoami != nil {
				return whoami.Identity, err
			}
			return "", err
		},
	}
}

// TestIdentityFalseIsRejectedByIdentityCommands verifies that subcommands which
// exist to act on an identity reject `--identity=false` and `ATMOS_IDENTITY=false`
// with a clear error instead of looking up an identity named "false".
func TestIdentityFalseIsRejectedByIdentityCommands(t *testing.T) {
	sources := map[string]func(t *testing.T) *cobra.Command{
		"--identity=false": func(t *testing.T) *cobra.Command { return newIdentityFlagCmd(t, "--identity=false") },
		"ATMOS_IDENTITY=false": func(t *testing.T) *cobra.Command {
			setAmbientIdentityEnv(t, "false")
			return newIdentityFlagCmd(t)
		},
	}
	for command, resolve := range disabledIdentityResolvers() {
		for source, newCmd := range sources {
			t.Run(command+"/"+source, func(t *testing.T) {
				// No AuthManager calls are expected: nothing may be looked up or authenticated.
				mockAuthManager := authTypes.NewMockAuthManager(gomock.NewController(t))

				identity, err := resolve(newCmd(t), auth.AuthManager(mockAuthManager))
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrAuthDisabledIdentityRequired)
				assert.Empty(t, identity)
			})
		}
	}
}

// TestCheckIdentityNotDisabled_AllowsNamedIdentities is the negative path:
// ordinary identity names and the interactive sentinel pass through.
func TestCheckIdentityNotDisabled_AllowsNamedIdentities(t *testing.T) {
	for _, identity := range []string{"", "prod", IdentityFlagSelectValue} {
		assert.NoError(t, checkIdentityNotDisabled("whoami", identity))
	}
}

func TestResolveLogoutTarget_IdentityFalse(t *testing.T) {
	_, err := resolveLogoutTarget(&logoutSelection{identityFlag: "false"})
	assert.ErrorIs(t, err, errUtils.ErrAuthDisabledIdentityRequired)

	_, err = resolveLogoutTarget(&logoutSelection{identityEnv: "false"})
	assert.ErrorIs(t, err, errUtils.ErrAuthDisabledIdentityRequired)

	// ATMOS_IDENTITY=false is ignored when another selector picks the target.
	got, err := resolveLogoutTarget(&logoutSelection{identityEnv: "false", tags: []string{"production"}})
	require.NoError(t, err)
	assert.Equal(t, logoutTarget{kind: logoutTargetTags}, got)

	// --all-realms needs no identity, so an ambient ATMOS_IDENTITY=false does not block it.
	got, err = resolveLogoutTarget(&logoutSelection{identityEnv: "false", allRealms: true})
	require.NoError(t, err)
	assert.Equal(t, logoutTarget{kind: logoutTargetAllRealms}, got)

	got, err = resolveLogoutTarget(&logoutSelection{identityEnv: "false", all: true})
	require.NoError(t, err)
	assert.Equal(t, logoutTarget{kind: logoutTargetAll}, got)
}

// TestExecuteAuthExecCommand_IdentityFalseRunsWithoutAuth verifies the
// documented meaning of `--identity=false` for `atmos auth exec`: Atmos skips
// authentication and runs the command with the ambient environment, so cloud
// SDKs fall back to their default credential resolution.
func TestExecuteAuthExecCommand_IdentityFalseRunsWithoutAuth(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	for name, flagArgs := range map[string][]string{
		"--identity=false":     {"--identity=false"},
		"ATMOS_IDENTITY=false": nil,
	} {
		t.Run(name, func(t *testing.T) {
			setupMockAuthFixture(t)
			if flagArgs == nil {
				setAmbientIdentityEnv(t, "false")
			}
			t.Setenv("ATMOS_IDENTITY", "ambient-marker")
			reportPath := filepath.Join(t.TempDir(), "child-report.json")
			t.Setenv(authExecChildEnv, "1")
			t.Setenv(authExecChildReportEnv, reportPath)
			t.Setenv(authExecChildExitEnv, "0")

			cmd := authExecCmd
			resetAuthCmdFlags(t, cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags(append(flagArgs, "--", executable, "child-arg")))

			require.NoError(t, executeAuthExecCommand(cmd, cmd.Flags().Args()))

			raw, readErr := os.ReadFile(reportPath)
			require.NoError(t, readErr, "the child process must have run")
			var report authExecChildReport
			require.NoError(t, json.Unmarshal(raw, &report))
			assert.Equal(t, []string{"child-arg"}, report.Args)
			assert.Equal(t, "ambient-marker", report.Identity,
				"with authentication disabled the child must inherit the ambient environment, not an Atmos identity")
		})
	}
}
