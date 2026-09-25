package deferred

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_scoped_test.go -package=deferred

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

type rawIdentityStore interface {
	store.IdentityAwareStore
	store.RawStore
}

var _ rawIdentityStore = (*MockrawIdentityStore)(nil)

func TestScopedStoresPreserveRawCapabilityAndBindings(t *testing.T) {
	ctrl := gomock.NewController(t)
	backend := NewMockrawIdentityStore(ctrl)
	local := store.NewMockStore(ctrl)
	structured := store.NewMockIdentityAwareStore(ctrl)
	ac := &schema.AtmosConfiguration{
		AuthManager: authdeferred.NewManager(authdeferred.AuthOptions{Disabled: true}),
		Stores:      store.StoreRegistry{"remote": backend, "local": local, "structured": structured},
	}
	registry := ScopedStores(ac, &schema.ConfigAndStacksInfo{})
	require.Same(t, local, registry["local"])
	require.Same(t, backend, ac.Stores["remote"])
	require.NotImplements(t, (*store.RawStore)(nil), registry["structured"])
	raw, ok := registry["remote"].(store.RawStore)
	require.True(t, ok)
	backend.EXPECT().ResetAuthContext().Times(5)
	backend.EXPECT().Get("dev", "app", "key").Return("structured", nil)
	backend.EXPECT().GetKey("key").Return("direct", nil)
	backend.EXPECT().Set("dev", "app", "key", "new").Return(nil)
	backend.EXPECT().GetRaw("dev", "app", "key").Return("{raw}", nil)
	backend.EXPECT().GetRaw("dev", "app", "missing").Return("", errUtils.ErrFileNotFound)
	value, err := raw.Get("dev", "app", "key")
	require.NoError(t, err)
	require.Equal(t, "structured", value)
	value, err = raw.GetKey("key")
	require.NoError(t, err)
	require.Equal(t, "direct", value)
	require.NoError(t, raw.Set("dev", "app", "key", "new"))
	text, err := raw.GetRaw("dev", "app", "key")
	require.NoError(t, err)
	require.Equal(t, "{raw}", text)
	_, err = raw.GetRaw("dev", "app", "missing")
	require.ErrorIs(t, err, errUtils.ErrFileNotFound)
	delete(registry, "local")
	require.Same(t, local, ac.Stores["local"])
	delete(ac.Stores, "structured")
	require.NotNil(t, registry["structured"])
}

func TestStoreOperationReleasesOnFailureAndPanic(t *testing.T) {
	backend := store.NewMockIdentityAwareStore(gomock.NewController(t))
	ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": backend}}
	_, err := WithStoreAuth(ac, nil, "remote", func() (any, error) { return nil, errUtils.ErrFileNotFound })
	require.ErrorIs(t, err, errUtils.ErrFileNotFound)
	require.Panics(t, func() {
		_, _ = WithStoreAuth(ac, nil, "remote", func() (any, error) { panic("backend panic") })
	})
	storeOperations.Lock()
	_, retained := storeOperations.entries[backend]
	storeOperations.Unlock()
	require.False(t, retained, "completed operations must not retain discarded stores")
	value, err := WithStoreAuth(ac, nil, "remote", func() (any, error) { return "recovered", nil })
	require.NoError(t, err)
	require.Equal(t, "recovered", value)
}

func TestStoreOperationAcceptsNonComparableBackend(t *testing.T) {
	backend := struct {
		store.IdentityAwareStore
		Values []int
	}{IdentityAwareStore: store.NewMockIdentityAwareStore(gomock.NewController(t)), Values: []int{1}}
	ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": backend}}
	for range 2 {
		value, err := WithStoreAuth(ac, nil, "remote", func() (any, error) { return "value", nil })
		require.NoError(t, err)
		require.Equal(t, "value", value)
	}
}
