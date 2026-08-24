package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	pstore "github.com/cloudposse/atmos/pkg/store"
)

// TestStoreDefaultIdentity covers the precedence used to pick the default identity for
// identity-aware stores without their own configured identity: an explicit, real `--identity`
// wins; the select/disabled sentinels and the empty value fall back to the tail of the
// authenticated chain; an empty chain yields an empty string.
func TestStoreDefaultIdentity(t *testing.T) {
	t.Run("explicit identity wins without consulting the chain", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		am := authtypes.NewMockAuthManager(ctrl)
		// No GetChain expectation: an explicit identity must short-circuit before the chain lookup.
		assert.Equal(t, "explicit", storeDefaultIdentity(storeScope{Identity: "explicit"}, am))
	})

	sentinels := map[string]string{
		"empty":    "",
		"select":   cfg.IdentityFlagSelectValue,
		"disabled": cfg.IdentityFlagDisabledValue,
	}
	for name, id := range sentinels {
		t.Run(name+" identity falls back to the authenticated chain tail", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			am := authtypes.NewMockAuthManager(ctrl)
			am.EXPECT().GetChain().Return([]string{"base", "role-a", "role-b"})
			assert.Equal(t, "role-b", storeDefaultIdentity(storeScope{Identity: id}, am))
		})
	}

	t.Run("empty identity with empty chain yields empty string", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		am := authtypes.NewMockAuthManager(ctrl)
		am.EXPECT().GetChain().Return(nil)
		assert.Empty(t, storeDefaultIdentity(storeScope{}, am))
	})
}

// TestInjectStoreAuthResolver_NilArguments verifies the documented no-op behavior: a nil
// atmosConfig or nil authManager neither panics nor mutates the store registry.
func TestInjectStoreAuthResolver_NilArguments(t *testing.T) {
	t.Run("nil atmosConfig is a no-op", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		am := authtypes.NewMockAuthManager(ctrl)
		injectStoreAuthResolver(nil, am, storeScope{})
	})

	t.Run("nil authManager leaves the store registry untouched", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		injectStoreAuthResolver(atmosConfig, nil, storeScope{})
		assert.Nil(t, atmosConfig.Stores)
	})
}

// TestInjectStoreAuthResolver_ResolverWiring exercises the real wiring branch: a non-nil
// atmosConfig and authManager cause every identity-aware store in the registry to receive the
// resolver via SetAuthContext.
func TestInjectStoreAuthResolver_ResolverWiring(t *testing.T) {
	ctrl := gomock.NewController(t)
	authManager := authtypes.NewMockAuthManager(ctrl)
	mockStore := pstore.NewMockIdentityAwareStore(ctrl)

	authManager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{})
	// MockIdentityAwareStore does not implement the optional `IdentityName() string` interface
	// defaultIdentityForStore probes for, so the registry always falls back to an empty identity
	// name for it regardless of the resolved default identity.
	mockStore.EXPECT().
		SetAuthContext(gomock.Not(nil), "").
		Do(func(resolver pstore.AuthContextResolver, identityName string) {
			assert.NotNil(t, resolver)
			assert.Empty(t, identityName)
		})

	atmosConfig := &schema.AtmosConfiguration{
		Stores: pstore.StoreRegistry{
			"explicit-identity-store": mockStore,
		},
	}

	injectStoreAuthResolver(atmosConfig, authManager, storeScope{Identity: "explicit-identity"})
}
