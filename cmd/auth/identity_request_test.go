package auth

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setAmbientIdentityEnv simulates ATMOS_IDENTITY (exported inside `atmos auth
// shell`) as Viper sees it, and resets the global Viper afterwards (an empty override would mask later lookups).
func setAmbientIdentityEnv(t *testing.T, identity string) {
	t.Helper()
	viper.Set(IdentityFlagName, identity)
	t.Cleanup(func() { viper.Reset() })
}

func newIdentityFlagCmd(t *testing.T, cliArgs ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String(IdentityFlagName, "", "identity")
	cmd.Flags().Lookup(IdentityFlagName).NoOptDefVal = IdentityFlagSelectValue
	require.NoError(t, cmd.ParseFlags(cliArgs))
	return cmd
}

func TestResolveIdentityRequest(t *testing.T) {
	t.Run("explicit flag", func(t *testing.T) {
		setAmbientIdentityEnv(t, "from-env")
		req := resolveIdentityRequest(newIdentityFlagCmd(t, "--identity=prod"), viper.GetViper())
		assert.Equal(t, identityRequest{flag: "prod"}, req)
		assert.Equal(t, "prod", req.name())
	})
	t.Run("flag without a value", func(t *testing.T) {
		req := resolveIdentityRequest(newIdentityFlagCmd(t, "--identity"), viper.GetViper())
		assert.Equal(t, IdentityFlagSelectValue, req.flag)
	})
	t.Run("environment only", func(t *testing.T) {
		setAmbientIdentityEnv(t, "from-env")
		req := resolveIdentityRequest(newIdentityFlagCmd(t), viper.GetViper())
		assert.Equal(t, identityRequest{env: "from-env"}, req)
		assert.Equal(t, "from-env", req.name())
	})
	t.Run("nothing", func(t *testing.T) {
		req := resolveIdentityRequest(newIdentityFlagCmd(t), viper.GetViper())
		assert.Equal(t, identityRequest{}, req)
		assert.Empty(t, req.name())
	})
}

// TestAuthenticateIdentity_TagsWinOverAmbientIdentityEnv verifies that inside
// `atmos auth shell` (which exports ATMOS_IDENTITY), `atmos auth login --tags`
// selects by tag instead of silently logging in to the shell's identity.
func TestAuthenticateIdentity_TagsWinOverAmbientIdentityEnv(t *testing.T) {
	ctrl := gomock.NewController(t)

	setAmbientIdentityEnv(t, "shell-identity")
	viper.Set(authTagsViperKey, "admin")
	t.Cleanup(func() { viper.Reset() })

	mockAuthManager := authTypes.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin":     {Kind: "aws/permission-set", Tags: []string{"admin"}},
		"shell-identity": {Kind: "aws/permission-set"},
	})
	mockAuthManager.EXPECT().Authenticate(gomock.Any(), "prod-admin").Return(&authTypes.WhoamiInfo{Identity: "prod-admin"}, nil)

	whoami, needsFallback, err := authenticateIdentity(context.Background(), newIdentityFlagCmd(t), auth.AuthManager(mockAuthManager))
	require.NoError(t, err)
	assert.False(t, needsFallback)
	require.NotNil(t, whoami)
	assert.Equal(t, "prod-admin", whoami.Identity)
}

// TestAuthenticateIdentity_TagsWithoutMatchFailInsteadOfUsingAmbientIdentity
// verifies that an unmatched --tags reports the mismatch rather than falling
// back to ATMOS_IDENTITY, and that nothing is authenticated.
func TestAuthenticateIdentity_TagsWithoutMatchFailInsteadOfUsingAmbientIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)

	setAmbientIdentityEnv(t, "shell-identity")
	viper.Set(authTagsViperKey, "nonexistent")
	t.Cleanup(func() { viper.Reset() })

	mockAuthManager := authTypes.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"shell-identity": {Kind: "aws/permission-set", Tags: []string{"admin"}},
	}).AnyTimes()

	whoami, _, err := authenticateIdentity(context.Background(), newIdentityFlagCmd(t), auth.AuthManager(mockAuthManager))
	assert.ErrorIs(t, err, errUtils.ErrNoIdentitiesMatchTags)
	assert.Nil(t, whoami)
}

// TestAuthenticateIdentity_ExplicitIdentityWinsOverTags keeps the existing
// precedence: an explicit --identity flag is used even when --tags is set.
func TestAuthenticateIdentity_ExplicitIdentityWinsOverTags(t *testing.T) {
	ctrl := gomock.NewController(t)

	viper.Set(authTagsViperKey, "admin")
	t.Cleanup(func() { viper.Reset() })

	mockAuthManager := authTypes.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().Authenticate(gomock.Any(), "dev").Return(&authTypes.WhoamiInfo{Identity: "dev"}, nil)

	whoami, _, err := authenticateIdentity(context.Background(), newIdentityFlagCmd(t, "--identity=dev"), auth.AuthManager(mockAuthManager))
	require.NoError(t, err)
	require.NotNil(t, whoami)
	assert.Equal(t, "dev", whoami.Identity)
}

// TestResolveLogoutTarget_AmbientIdentityEnv verifies ATMOS_IDENTITY is not an
// explicit selector: it does not conflict with --tags (or --provider/--all) and
// is used only when nothing else selects a target.
func TestResolveLogoutTarget_AmbientIdentityEnv(t *testing.T) {
	got, err := resolveLogoutTarget(&logoutSelection{identityEnv: "shell-identity", tags: []string{"production"}})
	require.NoError(t, err, "ATMOS_IDENTITY must not make --tags mutually exclusive")
	assert.Equal(t, logoutTarget{kind: logoutTargetTags}, got)

	got, err = resolveLogoutTarget(&logoutSelection{identityEnv: "shell-identity"})
	require.NoError(t, err)
	assert.Equal(t, logoutTarget{kind: logoutTargetIdentity, identity: "shell-identity"}, got)

	got, err = resolveLogoutTarget(&logoutSelection{identityEnv: "shell-identity", identityFlag: "prod"})
	require.NoError(t, err)
	assert.Equal(t, logoutTarget{kind: logoutTargetIdentity, identity: "prod"}, got, "an explicit --identity wins over ATMOS_IDENTITY")
}
