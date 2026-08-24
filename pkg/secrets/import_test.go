package secrets

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// importTestConfig builds a config with two secret stores and one declared secret per backend
// shape used by the import tests.
func importTestConfig(appSecrets, legacy store.Store) (*schema.AtmosConfiguration, map[string]any) {
	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{
			"app-secrets": store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: true},
			"legacy":      store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: true},
		},
		Stores: store.StoreRegistry{"app-secrets": appSecrets, "legacy": legacy},
	}
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"SHARED_CLIENT_SECRET": map[string]any{"store": "app-secrets", "scope": "global"},
				"API_KEY":              map[string]any{"store": "app-secrets"},
			},
		},
	}
	return cfg, section
}

// TestImportFromStore_DefaultsAndGlobalTarget proves the defaults (source store = the
// declaration's store, source key = the secret name) and that the write lands at the
// declaration's computed coordinate (here global: empty stack and component). The source read
// uses the raw --from-* segments verbatim.
func TestImportFromStore_DefaultsAndGlobalTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	appSecrets := store.NewMockStore(ctrl)
	// Source read: raw legacy segments, default key = declaration name.
	appSecrets.EXPECT().Get("atmos", "shared", "SHARED_CLIENT_SECRET").Return("legacy-value", nil)
	// Target write: the global coordinate.
	appSecrets.EXPECT().Set("", "", "SHARED_CLIENT_SECRET", "legacy-value").Return(nil)

	cfg, section := importTestConfig(appSecrets, store.NewMockStore(ctrl))
	svc := NewService(cfg, "prod", "api", section)

	err := svc.ImportFromStore("SHARED_CLIENT_SECRET", ImportSource{Stack: "atmos", Component: "shared"}, false)
	require.NoError(t, err)
}

// TestImportFromStore_ExplicitSourceStoreAndKey proves an explicit source store and key override
// the defaults, and the target write still goes through the declaration's own store.
func TestImportFromStore_ExplicitSourceStoreAndKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	appSecrets := store.NewMockStore(ctrl)
	legacy := store.NewMockStore(ctrl)
	legacy.EXPECT().Get("atmos", "shared", "client_secret").Return("v", nil)
	appSecrets.EXPECT().Set("prod", "api", "API_KEY", "v").Return(nil)

	cfg, section := importTestConfig(appSecrets, legacy)
	svc := NewService(cfg, "prod", "api", section)

	src := ImportSource{Store: "legacy", Stack: "atmos", Component: "shared", Key: "client_secret"}
	require.NoError(t, svc.ImportFromStore("API_KEY", src, false))
}

// TestImportFromStore_DryRunReadsButNeverWrites proves dry-run reads the source (verifying it
// exists) and writes nothing.
func TestImportFromStore_DryRunReadsButNeverWrites(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	appSecrets := store.NewMockStore(ctrl)
	appSecrets.EXPECT().Get("atmos", "shared", "API_KEY").Return("v", nil)
	// No Set expectation: a write would fail the mock controller.

	cfg, section := importTestConfig(appSecrets, store.NewMockStore(ctrl))
	svc := NewService(cfg, "prod", "api", section)

	require.NoError(t, svc.ImportFromStore("API_KEY", ImportSource{Stack: "atmos", Component: "shared"}, true))
}

// TestImportFromStore_Errors proves the error paths: undeclared name, unknown source store,
// unreadable source, and a non-store declaration with no explicit source store.
func TestImportFromStore_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("undeclared name", func(t *testing.T) {
		cfg, section := importTestConfig(store.NewMockStore(ctrl), store.NewMockStore(ctrl))
		svc := NewService(cfg, "prod", "api", section)
		err := svc.ImportFromStore("NOT_DECLARED", ImportSource{Stack: "x"}, false)
		require.ErrorIs(t, err, ErrSecretNotDeclared)
	})

	t.Run("unknown source store", func(t *testing.T) {
		cfg, section := importTestConfig(store.NewMockStore(ctrl), store.NewMockStore(ctrl))
		svc := NewService(cfg, "prod", "api", section)
		err := svc.ImportFromStore("API_KEY", ImportSource{Store: "nope", Stack: "x"}, false)
		require.ErrorIs(t, err, ErrStoreNotFound)
	})

	t.Run("unreadable source", func(t *testing.T) {
		appSecrets := store.NewMockStore(ctrl)
		appSecrets.EXPECT().Get("atmos", "shared", "API_KEY").Return(nil, errors.New("not found"))
		cfg, section := importTestConfig(appSecrets, store.NewMockStore(ctrl))
		svc := NewService(cfg, "prod", "api", section)
		err := svc.ImportFromStore("API_KEY", ImportSource{Stack: "atmos", Component: "shared"}, false)
		require.ErrorIs(t, err, ErrImportSourceRead)
		assert.Contains(t, err.Error(), `stack="atmos"`)
	})

	t.Run("sops declaration needs explicit source store", func(t *testing.T) {
		cfg, _ := importTestConfig(store.NewMockStore(ctrl), store.NewMockStore(ctrl))
		section := map[string]any{
			"secrets": map[string]any{
				"vars": map[string]any{
					"GH_KEY": map[string]any{"sops": "default"},
				},
			},
		}
		svc := NewService(cfg, "prod", "api", section)
		err := svc.ImportFromStore("GH_KEY", ImportSource{Stack: "x"}, false)
		require.ErrorIs(t, err, ErrImportSourceStore)
	})
}
