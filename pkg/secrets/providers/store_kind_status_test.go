package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// newStoreProviderForTest builds a store-backed provider over the given mock, with an optional
// explicit `kind` on the store config.
func newStoreProviderForTest(t *testing.T, mockStore store.Store, kind string) Provider {
	t.Helper()

	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{
			"app": store.StoreConfig{Type: "aws-ssm-parameter-store", Kind: kind, Secret: true},
		},
		Stores: store.StoreRegistry{"app": mockStore},
	}
	p, err := newStoreProvider(cfg, "app")
	require.NoError(t, err)
	return p
}

// TestStoreProvider_Kind reports the explicit kind when set, otherwise the store type.
func TestStoreProvider_Kind(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	withKind := newStoreProviderForTest(t, store.NewMockStore(ctrl), "secrets-manager")
	assert.Equal(t, "secrets-manager", withKind.Kind())

	withoutKind := newStoreProviderForTest(t, store.NewMockStore(ctrl), "")
	assert.Equal(t, "aws-ssm-parameter-store", withoutKind.Kind())
}

// TestStoreProvider_Status_GetFallback covers the non-StatusStore branch of Status: a plain
// MockStore does not implement StatusStore, so Status falls back to Get — success => initialized,
// error => not initialized.
func TestStoreProvider_Status_GetFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	coord := Coordinate{Stack: "prod", Component: "api", Key: "API_KEY"}

	t.Run("present", func(t *testing.T) {
		mockStore := store.NewMockStore(ctrl)
		mockStore.EXPECT().Get("prod", "api", "API_KEY").Return("v1", nil)
		p := newStoreProviderForTest(t, mockStore, "")

		ok, err := p.Status(coord)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("absent", func(t *testing.T) {
		mockStore := store.NewMockStore(ctrl)
		mockStore.EXPECT().Get("prod", "api", "API_KEY").Return(nil, assertProviderErr{})
		p := newStoreProviderForTest(t, mockStore, "")

		ok, err := p.Status(coord)
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

// assertProviderErr is a trivial error used to simulate a backend miss.
type assertProviderErr struct{}

func (assertProviderErr) Error() string { return "not found" }
