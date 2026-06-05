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
	_, err := NewStore(cfg, "missing")
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
	_, err := NewStore(cfg, "plain")
	require.ErrorIs(t, err, ErrStoreNotSecret)
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
	p, err := NewStore(cfg, "app")
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
	p, err := NewStore(cfg, "app")
	require.NoError(t, err)

	err = p.Delete(Coordinate{Stack: "prod", Component: "api", Key: "API_KEY"})
	require.ErrorIs(t, err, ErrDeleteNotSupported)
}

func TestNewSops_FromSection(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	section := map[string]any{
		"dev-sops": map[string]any{
			"kind": "sops/age",
			"spec": map[string]any{"file": "secrets/dev.enc.yaml"},
		},
	}
	p, err := NewSops(cfg, "dev-sops", section)
	require.NoError(t, err)
	assert.Equal(t, "sops/age", p.Kind())
}

func TestNewSops_NotFound(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	_, err := NewSops(cfg, "missing", nil)
	require.ErrorIs(t, err, ErrProviderNotFound)
}
