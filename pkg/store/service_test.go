package store

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestService_Set(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().Set("prod", "vpc", "image_tag", "v1.2.3").Return(nil)

	svc := NewService(StoresConfig{"app": {Kind: KindAWSSSM}}, StoreRegistry{"app": mockStore})

	err := svc.Set("app", "prod", "vpc", "image_tag", "v1.2.3")
	require.NoError(t, err)
}

func TestService_Set_StoreNotConfigured(t *testing.T) {
	svc := NewService(StoresConfig{}, StoreRegistry{})

	err := svc.Set("missing", "prod", "vpc", "key", "value")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStoreNotConfigured)
}

func TestService_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().Get("prod", "vpc", "image_tag").Return("v1.2.3", nil)

	svc := NewService(StoresConfig{"app": {Kind: KindAWSSSM}}, StoreRegistry{"app": mockStore})

	value, err := svc.Get("app", "prod", "vpc", "image_tag")
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", value)
}

func TestService_GetKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockStore(ctrl)
	mockStore.EXPECT().GetKey("image_tag").Return("v1.2.3", nil)

	svc := NewService(StoresConfig{"app": {Kind: KindAWSSSM}}, StoreRegistry{"app": mockStore})

	value, err := svc.GetKey("app", "image_tag")
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", value)
}

func TestService_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockDeletableStore(ctrl)
	mockStore.EXPECT().Delete("prod", "vpc", "image_tag").Return(nil)

	svc := NewService(StoresConfig{"app": {Kind: KindAWSSSM}}, StoreRegistry{"app": mockStore})

	err := svc.Delete("app", "prod", "vpc", "image_tag")
	require.NoError(t, err)
}

func TestService_Delete_NotSupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockStore(ctrl) // Store only -- does not implement DeletableStore.

	svc := NewService(StoresConfig{"app": {Kind: KindRedis}}, StoreRegistry{"app": mockStore})

	err := svc.Delete("app", "prod", "vpc", "image_tag")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeleteNotSupported)
}

func TestService_Delete_StoreNotConfigured(t *testing.T) {
	svc := NewService(StoresConfig{}, StoreRegistry{})

	err := svc.Delete("missing", "prod", "vpc", "key")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStoreNotConfigured)
}

// fakeLocalStore is a small hand-written fake (rather than a generated mock) implementing both
// Store and LocalStore -- there is no generated MockLocalStore, and none of the generated mocks
// in mock_store.go happen to implement IsLocal.
type fakeLocalStore struct{}

func (fakeLocalStore) Set(stack, component, key string, value any) error { return nil }
func (fakeLocalStore) Get(stack, component, key string) (any, error)     { return nil, nil }
func (fakeLocalStore) GetKey(key string) (any, error)                    { return nil, nil }
func (fakeLocalStore) IsLocal() bool                                     { return true }

func TestService_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	deletable := NewMockDeletableStore(ctrl)
	plain := NewMockStore(ctrl)
	statusStore := NewMockStatusStore(ctrl)
	localStore := fakeLocalStore{}

	svc := NewService(
		StoresConfig{
			"app-secrets": {Kind: KindAWSSSM, Secret: true},
			"app-cache":   {Kind: KindRedis},
			"app-status":  {Kind: KindHashicorpVault},
			"app-local":   {Kind: KindKeychain},
			"app-missing": {Kind: KindAWSSSM}, // Configured, but absent from the live registry.
		},
		StoreRegistry{
			"app-secrets": deletable,
			"app-cache":   plain,
			"app-status":  statusStore,
			"app-local":   localStore,
			// "app-missing" intentionally omitted, simulating a store whose construction failed.
		},
	)

	descriptors := svc.List()
	require.Len(t, descriptors, 5)

	byName := make(map[string]Descriptor, len(descriptors))
	for _, d := range descriptors {
		byName[d.Name] = d
	}

	appCache := byName["app-cache"]
	assert.Equal(t, KindRedis, appCache.Kind)
	assert.False(t, appCache.Secret)
	assert.False(t, appCache.Deletable)
	assert.False(t, appCache.HasStatus)
	assert.False(t, appCache.Local)

	appSecrets := byName["app-secrets"]
	assert.Equal(t, KindAWSSSM, appSecrets.Kind)
	assert.True(t, appSecrets.Secret)
	assert.True(t, appSecrets.Deletable)
	assert.False(t, appSecrets.HasStatus)
	assert.False(t, appSecrets.Local)

	appStatus := byName["app-status"]
	assert.Equal(t, KindHashicorpVault, appStatus.Kind)
	assert.True(t, appStatus.HasStatus)
	assert.False(t, appStatus.Deletable)
	assert.False(t, appStatus.Local)

	appLocal := byName["app-local"]
	assert.Equal(t, KindKeychain, appLocal.Kind)
	assert.True(t, appLocal.Local)
	assert.False(t, appLocal.HasStatus)
	assert.False(t, appLocal.Deletable)

	// A store present in StoresConfig but missing from the live registry (construction failed)
	// still gets a descriptor -- from config alone -- with every capability flag left false.
	appMissing := byName["app-missing"]
	assert.Equal(t, KindAWSSSM, appMissing.Kind)
	assert.False(t, appMissing.Secret)
	assert.False(t, appMissing.Deletable)
	assert.False(t, appMissing.HasStatus)
	assert.False(t, appMissing.Local)
}

func TestService_List_Empty(t *testing.T) {
	svc := NewService(StoresConfig{}, StoreRegistry{})
	assert.Empty(t, svc.List())
}

func TestService_lookup_wrapsSentinel(t *testing.T) {
	svc := NewService(StoresConfig{}, StoreRegistry{})

	_, err := svc.lookup("missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrStoreNotConfigured))
}
