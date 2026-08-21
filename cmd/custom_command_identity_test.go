package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommand_IdentityFlagNoOptDefVal verifies custom commands register --identity via
// the shared flags.WithIdentityFlag() builder, so a bare --identity (no value) is legal and
// carries the interactive-selection sentinel instead of failing with "flag needs an argument".
func TestCustomCommand_IdentityFlagNoOptDefVal(t *testing.T) {
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	atmosConfig.Commands = []schema.Command{
		{
			Name:  "ci-build",
			Steps: schema.Tasks{{Type: "shell", Command: "true"}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	buildCmd := findSubcommand(RootCmd, "ci-build")
	require.NotNil(t, buildCmd, "ci-build command should be registered")

	identityFlag := buildCmd.PersistentFlags().Lookup(customCommandKeyIdentity)
	require.NotNil(t, identityFlag, "identity flag should be registered on custom command")
	assert.Equal(t, cfg.IdentityFlagSelectValue, identityFlag.NoOptDefVal,
		"bare --identity must carry the select sentinel, matching every other Atmos command")
	assert.Equal(t, "i", identityFlag.Shorthand, "identity flag should support the -i shorthand like other commands")
}

// TestPrepareCustomCommandAuth_SelectValue verifies that passing the interactive-selection
// sentinel (produced by a bare --identity) resolves to a concrete identity via
// GetDefaultIdentity(forceSelect=true) before the cache check and Authenticate call, instead of
// being passed through literally (which would fail with ErrIdentityNotFound).
func TestPrepareCustomCommandAuth_SelectValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)

	mgr.EXPECT().GetDefaultIdentity(true).Return("resolved-identity", nil)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), "resolved-identity").
		Return(nil, assert.AnError)
	mgr.EXPECT().Authenticate(gomock.Any(), "resolved-identity").
		Return(&types.WhoamiInfo{Credentials: &types.AWSCredentials{}}, nil)

	orig := newCustomCommandAuthManagerFn
	newCustomCommandAuthManagerFn = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return mgr, nil
	}
	t.Cleanup(func() { newCustomCommandAuthManagerFn = orig })

	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{Identities: map[string]schema.Identity{"resolved-identity": {Kind: "aws/user"}}},
	}

	result := prepareCustomCommandAuth(atmosConfig, cfg.IdentityFlagSelectValue, "ci-build", true)

	require.NotNil(t, result)
	assert.Same(t, mgr, result)
}
