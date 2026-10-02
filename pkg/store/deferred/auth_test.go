package deferred

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/providers"
)

func TestResolveStoreAuthClearsPreviousIdentity(t *testing.T) {
	for _, scenario := range []string{"no manager", "no stack info", "no input info", "disabled", "component disabled"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, err := providers.NewSSMStore(providers.SSMStoreOptions{Region: "us-east-1"}, "")
			require.NoError(t, err)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": s}}
			ac.AuthManager = authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory, Disabled: scenario == "disabled"})
			info := &schema.ConfigAndStacksInfo{Stack: "dev", AuthDisabled: scenario == "component disabled"}
			prior := types.NewMockAuthManager(ctrl)
			info.AuthManager = prior
			s.(store.IdentityAwareStore).SetAuthContext(store.NewMockAuthContextResolver(ctrl), "previous-account")
			switch scenario {
			case "no input info":
				info = nil
				factory.EXPECT().Create(gomock.Any(), gomock.Any(), "").Return(nil, nil)
			case "no manager":
				factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, nil)
			case "no stack info":
				manager := types.NewMockAuthManager(ctrl)
				manager.EXPECT().GetStackInfo().Return(nil).AnyTimes()
				manager.EXPECT().GetChain().Return([]string{}).AnyTimes()
				factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(manager, nil)
			}
			require.NoError(t, resolveStoreAuth(ac, info, "remote"))
			assert.Empty(t, s.(*providers.SSMStore).IdentityName(), "a previous component's identity must not survive")
		})
	}
}

func TestResolveStoreAuthRebindsSameIdentityWithDifferentConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := store.NewMockIdentityAwareStore(ctrl)
	factory := authdeferred.NewMockAuthFactory(ctrl)
	ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": s}}
	ac.AuthManager = authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory})
	for _, profile := range []string{"account-one", "account-two"} {
		info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentSection: map[string]any{
			"auth": map[string]any{"identities": map[string]any{
				"local": map[string]any{"kind": "aws/user", "default": true, "credentials": map[string]any{"profile": profile}},
			}},
		}}
		manager := types.NewMockAuthManager(ctrl)
		manager.EXPECT().GetChain().Return([]string{"local"}).AnyTimes()
		manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{
			AWS: &schema.AWSAuthContext{Profile: profile},
		}}).AnyTimes()
		gomock.InOrder(
			s.EXPECT().ResetAuthContext(),
			factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(manager, nil),
			s.EXPECT().SetAuthContext(gomock.Any(), "local").Do(func(resolver store.AuthContextResolver, identity string) {
				resolved, err := resolver.ResolveAWSAuthContext(t.Context(), identity)
				require.NoError(t, err)
				assert.Equal(t, profile, resolved.Profile)
			}),
		)
		require.NoError(t, resolveStoreAuth(ac, info, "remote"))
	}
}

func TestResolveStoreAuthPreservesConfiguredIdentityWithoutContext(t *testing.T) {
	for _, withManager := range []bool{false, true} {
		t.Run("configured identity remains required", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, err := providers.NewSSMStore(providers.SSMStoreOptions{Region: "us-east-1"}, "configured")
			require.NoError(t, err)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{
				Stores:       store.StoreRegistry{"remote": s},
				StoresConfig: map[string]store.StoreConfig{"remote": {Identity: "configured"}},
				Auth:         schema.AuthConfig{Identities: map[string]schema.Identity{"configured": {Kind: "aws/user"}}},
			}
			ac.AuthManager = authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory})
			if withManager {
				manager := types.NewMockAuthManager(ctrl)
				manager.EXPECT().GetStackInfo().Return(nil).AnyTimes()
				factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(manager, nil)
			} else {
				factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, nil)
			}
			require.NoError(t, resolveStoreAuth(ac, &schema.ConfigAndStacksInfo{Stack: "dev"}, "remote"))
			assert.Equal(t, "configured", s.(*providers.SSMStore).IdentityName())
			_, err = s.GetKey("key")
			require.ErrorIs(t, err, store.ErrIdentityNotConfigured, "must not fall back to ambient credentials")
		})
	}
}

func TestResolveStoreAuthInheritsExplicitCallerIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	backend := store.NewMockIdentityAwareStore(ctrl)
	factory := authdeferred.NewMockAuthFactory(ctrl)
	ac := &schema.AtmosConfiguration{
		AuthManager: authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory}),
		Stores:      store.StoreRegistry{"remote": backend},
		Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
			"default": {Kind: "aws/user", Default: true}, "requested": {Kind: "aws/user"},
		}},
	}
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "requested"}}}).AnyTimes()
	manager.EXPECT().GetChain().Return([]string{"requested"}).AnyTimes()
	backend.EXPECT().ResetAuthContext()
	factory.EXPECT().Create(gomock.Any(), gomock.Cond(func(config *schema.AuthConfig) bool {
		return config.Identities["requested"].Default && !config.Identities["default"].Default
	}), "dev").Return(manager, nil)
	backend.EXPECT().SetAuthContext(gomock.Any(), "requested")
	require.NoError(t, resolveStoreAuth(ac, &schema.ConfigAndStacksInfo{Stack: "dev", Identity: "requested"}, "remote"))
	require.True(t, ac.Auth.Identities["default"].Default)
	require.False(t, ac.Auth.Identities["requested"].Default)
}

// TestResolveStoreAuthUsesOriginalRequest preserves explicit caller and store
// selection after the component manager has replaced Identity with its chain.
func TestResolveStoreAuthUsesOriginalRequest(t *testing.T) {
	for _, tc := range []struct {
		name, caller, configured, want string
		request                        *string
	}{
		{name: "original explicit request", caller: "component", request: new("requested"), want: "requested"},
		{name: "original empty request", caller: "component", request: new(""), want: "default"},
		{name: "legacy caller", caller: "requested", want: "requested"},
		{name: "store overrides caller", caller: "component", request: new("requested"), configured: "store", want: "store"},
		{name: "select is not an identity", caller: "component", request: new(cfg.IdentityFlagSelectValue), want: "default"},
		{name: "disabled is not an identity", caller: "component", request: new(cfg.IdentityFlagDisabledValue), want: "default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			backend := store.NewMockIdentityAwareStore(ctrl)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{
				AuthManager:  authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory}),
				Stores:       store.StoreRegistry{"remote": backend},
				StoresConfig: map[string]store.StoreConfig{"remote": {Identity: tc.configured}},
				Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
					"default": {Kind: "aws/user", Default: true}, "component": {Kind: "aws/user"},
					"requested": {Kind: "aws/user"}, "store": {Kind: "aws/user"},
				}},
			}
			info := &schema.ConfigAndStacksInfo{Stack: "dev", Identity: tc.caller, RequestedIdentity: tc.request}
			manager := types.NewMockAuthManager(ctrl)
			manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: tc.want}}}).AnyTimes()
			manager.EXPECT().GetChain().Return([]string{tc.want}).AnyTimes()
			backend.EXPECT().ResetAuthContext()
			factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Do(func(_ *schema.AtmosConfiguration, config *schema.AuthConfig, _ string) {
				for name, identity := range config.Identities {
					assert.Equal(t, name == tc.want, identity.Default, "default selection for %s", name)
				}
			}).Return(manager, nil)
			backend.EXPECT().SetAuthContext(gomock.Any(), tc.want).Do(func(resolver store.AuthContextResolver, identity string) {
				resolved, err := resolver.ResolveAWSAuthContext(t.Context(), identity)
				require.NoError(t, err)
				assert.Equal(t, tc.want, resolved.Profile)
			})
			require.NoError(t, resolveStoreAuth(ac, info, "remote"))
			assert.Equal(t, tc.caller, info.Identity)
			assert.Equal(t, tc.request, info.RequestedIdentity)
			assert.True(t, ac.Auth.Identities["default"].Default)
			assert.False(t, ac.Auth.Identities["requested"].Default)
		})
	}
}
