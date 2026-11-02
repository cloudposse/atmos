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

func TestBindStoreOperationSelectsOnlyTheRequestedIdentity(t *testing.T) {
	for _, test := range []struct {
		name, pinned, requested, want string
	}{
		{name: "pinned", pinned: "pinned", requested: "caller", want: "pinned"},
		{name: "explicit caller", requested: "caller", want: "caller"},
		{name: "authenticated default", want: "default"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			manager := authtypes.NewMockAuthManager(ctrl)
			manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{})
			if test.pinned == "" && test.requested == "" {
				manager.EXPECT().GetChain().Return([]string{"base", "default"})
			}
			backend := pstore.NewMockIdentityAwareStore(ctrl)
			backend.EXPECT().ResetAuthContext()
			backend.EXPECT().SetAuthContext(gomock.Not(nil), test.want)
			config := &schema.AtmosConfiguration{
				Stores:       pstore.StoreRegistry{"selected": backend, "unused": pstore.NewMockIdentityAwareStore(ctrl)},
				StoresConfig: pstore.StoresConfig{"selected": {Identity: test.pinned}},
			}
			bindStoreOperation(config, manager, storeScope{Identity: test.requested}, "selected")
		})
	}
	t.Run("unselected authentication", func(t *testing.T) {
		backend := pstore.NewMockIdentityAwareStore(gomock.NewController(t))
		backend.EXPECT().ResetAuthContext()
		config := &schema.AtmosConfiguration{Stores: pstore.StoreRegistry{"selected": backend}}
		bindStoreOperation(config, nil, storeScope{}, "selected")
	})
	t.Run("explicitly disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		backend := pstore.NewMockIdentityAwareStore(ctrl)
		backend.EXPECT().ResetAuthContext()
		config := &schema.AtmosConfiguration{
			Stores:       pstore.StoreRegistry{"selected": backend},
			StoresConfig: pstore.StoresConfig{"selected": {Identity: "pinned"}},
		}
		bindStoreOperation(config, authtypes.NewMockAuthManager(ctrl), storeScope{Identity: cfg.IdentityFlagDisabledValue}, "selected")
	})
	t.Run("backend without identity support", func(t *testing.T) {
		backend := pstore.NewMockStore(gomock.NewController(t))
		config := &schema.AtmosConfiguration{Stores: pstore.StoreRegistry{"selected": backend}}
		bindStoreOperation(config, nil, storeScope{}, "selected")
	})
}
