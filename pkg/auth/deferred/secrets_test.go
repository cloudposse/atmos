package deferred

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func secretInfo(backend string) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{Stack: "dev", ComponentSection: map[string]any{
		"secrets": map[string]any{"vars": map[string]any{"KEY": map[string]any{backend: "vault"}}},
	}}
}

func TestPrepareSecretAuthDefersCloudAuthentication(t *testing.T) {
	ctrl := gomock.NewController(t)
	factory := NewMockAuthFactory(ctrl)
	ac := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
		"selected": {Kind: "aws/user", Default: true}, "other": {Kind: "aws/user"},
	}}, DeferredAuth: NewAuthResolver(AuthOptions{Factory: factory})}
	info := secretInfo("sops")
	require.NoError(t, PrepareSecretAuth(ac, "!secret KEY", info))
	require.Equal(t, "selected", ac.SecretsAuth.DefaultIdentity)
	// Preparing a local/age secret only installs a lazy resolver. No auth happens
	// until the encryption backend actually requests a cloud identity.
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "other"}}}).AnyTimes()
	factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").DoAndReturn(func(_ *schema.AtmosConfiguration, config *schema.AuthConfig, _ string) (auth.AuthManager, error) {
		require.True(t, config.Identities["other"].Default)
		require.False(t, config.Identities["selected"].Default)
		return manager, nil
	}).Times(1)
	for range 2 {
		credentials, err := ac.SecretsAuth.Resolver.ResolveAWSAuthContext(t.Context(), "other")
		require.NoError(t, err)
		require.Equal(t, "other", credentials.Profile)
	}
	require.True(t, ac.Auth.Identities["selected"].Default, "explicit secret identity must not mutate global defaults")
}

func TestPrepareSecretAuthGuards(t *testing.T) {
	for _, scenario := range []string{"invalid reference", "undeclared", "disabled", "component disabled", "store"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ac := &schema.AtmosConfiguration{DeferredAuth: NewAuthResolver(AuthOptions{Factory: NewMockAuthFactory(ctrl), Disabled: scenario == "disabled"})}
			info := secretInfo("sops")
			input := "!secret KEY"
			switch scenario {
			case "invalid reference":
				input = "!secret"
			case "undeclared":
				input = "!secret MISSING"
			case "component disabled":
				info.AuthDisabled = true
			case "store":
				info = secretInfo("store")
				ac.Stores = store.StoreRegistry{"vault": store.NewMockStore(ctrl)}
			}
			err := PrepareSecretAuth(ac, input, info)
			if scenario == "invalid reference" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Nil(t, ac.SecretsAuth)
		})
	}
}

func TestDeferredSecretContextErrorsAndImplicitIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	factory := NewMockAuthFactory(ctrl)
	ac := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Identities: map[string]schema.Identity{"only": {Kind: "aws/user"}}}, DeferredAuth: NewAuthResolver(AuthOptions{Factory: factory})}
	info := secretInfo("sops")
	require.NoError(t, PrepareSecretAuth(ac, "!secret KEY", info))
	require.Equal(t, "only", ac.SecretsAuth.DefaultIdentity)
	_, err := ac.SecretsAuth.Resolver.ResolveAWSAuthContext(t.Context(), "missing")
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
	factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, errUtils.ErrAuthenticationUnavailable).Times(1)
	for range 2 {
		_, err = ac.SecretsAuth.Resolver.ResolveAWSAuthContext(t.Context(), "")
		require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
	}
}

func TestDeferredAuthRejectsMalformedComponentConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	ac := &schema.AtmosConfiguration{DeferredAuth: NewAuthResolver(AuthOptions{Factory: NewMockAuthFactory(ctrl)})}
	info := secretInfo("sops")
	info.ComponentSection["auth"] = map[string]any{"identities": "invalid"}
	require.Error(t, ResolveAuth(ac, info))
	require.Error(t, PrepareSecretAuth(ac, "!secret KEY", info))
	_, err := (&deferredSecretContext{config: ac, info: info}).resolve(t.Context(), "selected")
	require.Error(t, err)
	s := store.NewMockIdentityAwareStore(ctrl)
	s.EXPECT().ResetAuthContext()
	ac.Stores = store.StoreRegistry{"vault": s}
	ac.StoresConfig = store.StoresConfig{"vault": {Identity: "selected"}}
	require.Error(t, ResolveStoreAuth(ac, info, "vault"))
}

func TestResolveAuthNoopAndDefaultFactory(t *testing.T) {
	require.NoError(t, ResolveAuth(nil, nil))
	require.NoError(t, ResolveAuth(&schema.AtmosConfiguration{}, nil))
	ac := &schema.AtmosConfiguration{}
	ConfigureAuth(ac, "")
	require.NoError(t, ResolveAuth(ac, nil))
	info := &schema.ConfigAndStacksInfo{}
	require.NoError(t, ResolveAuth(ac, info))
	require.Nil(t, info.AuthManager, "no configured identity needs no authentication")
}

func TestDeferredAuthRejectsUnserializableIdentity(t *testing.T) {
	factory := NewMockAuthFactory(gomock.NewController(t))
	ac := &schema.AtmosConfiguration{
		DeferredAuth: NewAuthResolver(AuthOptions{Factory: factory}),
		Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
			"invalid": {Kind: "aws/user", Credentials: map[string]any{"unsupported": make(chan int)}},
		}},
	}
	err := ResolveAuth(ac, &schema.ConfigAndStacksInfo{Stack: "dev"})
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}
