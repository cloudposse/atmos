package deferred

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/auth/types"
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
			require.NoError(t, ResolveStoreAuth(ac, info, "remote"))
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
		require.NoError(t, ResolveStoreAuth(ac, info, "remote"))
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
			require.NoError(t, ResolveStoreAuth(ac, &schema.ConfigAndStacksInfo{Stack: "dev"}, "remote"))
			assert.Equal(t, "configured", s.(*providers.SSMStore).IdentityName())
			_, err = s.GetKey("key")
			require.ErrorIs(t, err, store.ErrIdentityNotConfigured, "must not fall back to ambient credentials")
		})
	}
}
