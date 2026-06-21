package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestNewStore_NotConfigured(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	_, err := newStoreProvider(cfg, "missing")
	require.ErrorIs(t, err, ErrStoreNotFound)
}

func TestNewStore_NotSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{
			"plain": store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: false},
		},
		Stores: store.StoreRegistry{"plain": store.NewMockStore(ctrl)},
	}
	_, err := newStoreProvider(cfg, "plain")
	require.ErrorIs(t, err, ErrStoreNotSecret)
}

func TestNewStore_ClaimedSecretStoreKinds(t *testing.T) {
	tests := []struct {
		name            string
		config          store.StoreConfig
		secretByDefault bool
	}{
		{name: "aws ssm", config: store.StoreConfig{Kind: store.KindAWSSSM}},
		{name: "aws secrets manager", config: store.StoreConfig{Kind: store.KindAWSASM}},
		{name: "hashicorp vault", config: store.StoreConfig{Kind: store.KindHashicorpVault}},
		{name: "azure key vault", config: store.StoreConfig{Kind: store.KindAzureKeyVault}},
		{name: "gcp secret manager", config: store.StoreConfig{Kind: store.KindGCPSecret}},
		{name: "onepassword", config: store.StoreConfig{Kind: store.KindOnePassword}, secretByDefault: true},
		{name: "onepassword legacy alias", config: store.StoreConfig{Type: "1password"}, secretByDefault: true},
		{name: "keychain", config: store.StoreConfig{Kind: store.KindKeychain}, secretByDefault: true},
		{name: "keychain legacy alias", config: store.StoreConfig{Type: "keyring"}, secretByDefault: true},
		{name: "github actions", config: store.StoreConfig{Kind: store.KindGitHubActions}, secretByDefault: true},
		{name: "github actions legacy alias", config: store.StoreConfig{Type: "github-actions"}, secretByDefault: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cfgMap := store.StoresConfig{"app": tt.config}
			store.ApplySecretDefaults(cfgMap)
			cfg := &schema.AtmosConfiguration{
				StoresConfig: cfgMap,
				Stores:       store.StoreRegistry{"app": store.NewMockStore(ctrl)},
			}

			p, err := newStoreProvider(cfg, "app")
			if tt.secretByDefault {
				require.NoError(t, err)
				assert.NotNil(t, p)
				return
			}
			require.ErrorIs(t, err, ErrStoreNotSecret)

			secretConfig := tt.config
			secretConfig.Secret = true
			cfg.StoresConfig["app"] = secretConfig
			p, err = newStoreProvider(cfg, "app")
			require.NoError(t, err)
			assert.NotNil(t, p)
		})
	}
}

func TestStoreProvider_SetGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().Set("prod", "api", "API_KEY", "v1").Return(nil)
	mockStore.EXPECT().Get("prod", "api", "API_KEY").Return("v1", nil)

	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{"app": store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: true}},
		Stores:       store.StoreRegistry{"app": mockStore},
	}
	p, err := newStoreProvider(cfg, "app")
	require.NoError(t, err)

	coord := Coordinate{Stack: "prod", Component: "api", Key: "API_KEY"}
	require.NoError(t, p.Set(coord, "v1"))

	got, err := p.Get(coord)
	require.NoError(t, err)
	assert.Equal(t, "v1", got)
}

func TestStoreProvider_DeleteUnsupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Plain MockStore does not implement DeletableStore.
	mockStore := store.NewMockStore(ctrl)
	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{"app": store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: true}},
		Stores:       store.StoreRegistry{"app": mockStore},
	}
	p, err := newStoreProvider(cfg, "app")
	require.NoError(t, err)

	err = p.Delete(Coordinate{Stack: "prod", Component: "api", Key: "API_KEY"})
	require.ErrorIs(t, err, ErrDeleteNotSupported)
}

// localFakeStore implements store.Store plus store.LocalStore (IsLocal=true), modeling a local
// backend like the OS keychain.
type localFakeStore struct{ local bool }

func (s *localFakeStore) Set(_, _, _ string, _ any) error { return nil }
func (s *localFakeStore) Get(_, _, _ string) (any, error) { return nil, nil }
func (s *localFakeStore) GetKey(_ string) (any, error)    { return nil, nil }
func (s *localFakeStore) IsLocal() bool                   { return s.local }

// TestStoreProvider_LocalStatusCheck proves the provider forwards the underlying store's locality:
// a store without LocalStore (remote) reports false; a local store reports its IsLocal() value.
func TestStoreProvider_LocalStatusCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// A plain MockStore does not implement store.LocalStore → treated as remote.
	remote := &storeProvider{store: store.NewMockStore(ctrl)}
	assert.False(t, remote.LocalStatusCheck(), "a store without LocalStore must be non-local")

	// A store implementing LocalStore forwards its IsLocal() result.
	local := &storeProvider{store: &localFakeStore{local: true}}
	assert.True(t, local.LocalStatusCheck(), "a local store must report local")

	notLocal := &storeProvider{store: &localFakeStore{local: false}}
	assert.False(t, notLocal.LocalStatusCheck(), "a LocalStore returning false is not local")
}

func TestNewSops_FromSection(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	section := map[string]any{
		"dev-sops": map[string]any{
			"kind": "sops/age",
			"spec": map[string]any{"file": "secrets/dev.enc.yaml"},
		},
	}
	p, err := newSopsProvider(cfg, "dev-sops", section)
	require.NoError(t, err)
	assert.Equal(t, "sops/age", p.Kind())
}

func TestNewSops_NotFound(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	_, err := newSopsProvider(cfg, "missing", nil)
	require.ErrorIs(t, err, ErrProviderNotFound)
}
