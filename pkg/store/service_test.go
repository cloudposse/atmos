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

func TestService_Keys(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockListableStore(ctrl)
	mockStore.EXPECT().Keys("prod", "vpc").Return([]string{"image_tag", "region"}, nil)

	svc := NewService(StoresConfig{"app": {Kind: KindAWSSSM}}, StoreRegistry{"app": mockStore})

	keys, err := svc.Keys("app", "prod", "vpc")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"image_tag", "region"}, keys)
}

func TestService_Keys_NotSupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockStore(ctrl) // Store only -- does not implement ListableStore.

	svc := NewService(StoresConfig{"app": {Kind: KindOnePassword}}, StoreRegistry{"app": mockStore})

	_, err := svc.Keys("app", "prod", "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrListNotSupported)
}

func TestService_Keys_StoreNotConfigured(t *testing.T) {
	svc := NewService(StoresConfig{}, StoreRegistry{})

	_, err := svc.Keys("missing", "prod", "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStoreNotConfigured)
}

func TestService_ListKeyValues(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockListableStore(ctrl)
	mockStore.EXPECT().Keys("prod", "vpc").Return([]string{"region", "image_tag"}, nil)
	mockStore.EXPECT().Get("prod", "vpc", "image_tag").Return("v1.2.3", nil)
	mockStore.EXPECT().Get("prod", "vpc", "region").Return("us-east-1", nil)

	svc := NewService(StoresConfig{"app": {Kind: KindAWSSSM}}, StoreRegistry{"app": mockStore})

	kvs, err := svc.ListKeyValues("app", "prod", "vpc")
	require.NoError(t, err)
	require.Len(t, kvs, 2)
	assert.Equal(t, KeyValue{Key: "image_tag", Value: "v1.2.3"}, kvs[0])
	assert.Equal(t, KeyValue{Key: "region", Value: "us-east-1"}, kvs[1])
}

func TestService_ListKeyValues_NotSupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockStore(ctrl) // Store only -- does not implement ListableStore.

	svc := NewService(StoresConfig{"app": {Kind: KindOnePassword}}, StoreRegistry{"app": mockStore})

	_, err := svc.ListKeyValues("app", "prod", "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrListNotSupported)
}

func TestService_ListKeyValues_GetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockListableStore(ctrl)
	mockStore.EXPECT().Keys("prod", "vpc").Return([]string{"image_tag"}, nil)
	mockStore.EXPECT().Get("prod", "vpc", "image_tag").Return(nil, assert.AnError)

	svc := NewService(StoresConfig{"app": {Kind: KindAWSSSM}}, StoreRegistry{"app": mockStore})

	_, err := svc.ListKeyValues("app", "prod", "vpc")
	require.ErrorIs(t, err, assert.AnError)
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
	listableStore := NewMockListableStore(ctrl)

	svc := NewService(
		StoresConfig{
			"app-secrets":  {Kind: KindAWSSSM, Secret: true},
			"app-cache":    {Kind: KindRedis},
			"app-status":   {Kind: KindHashicorpVault},
			"app-local":    {Kind: KindKeychain},
			"app-missing":  {Kind: KindAWSSSM}, // Configured, but absent from the live registry.
			"app-listable": {Kind: KindHashicorpVault},
		},
		StoreRegistry{
			"app-secrets":  deletable,
			"app-cache":    plain,
			"app-status":   statusStore,
			"app-local":    localStore,
			"app-listable": listableStore,
			// "app-missing" intentionally omitted, simulating a store whose construction failed.
		},
	)

	descriptors := svc.List()
	require.Len(t, descriptors, 6)

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
	assert.False(t, appCache.Listable)

	appSecrets := byName["app-secrets"]
	assert.Equal(t, KindAWSSSM, appSecrets.Kind)
	assert.True(t, appSecrets.Secret)
	assert.True(t, appSecrets.Deletable)
	assert.False(t, appSecrets.HasStatus)
	assert.False(t, appSecrets.Local)
	assert.False(t, appSecrets.Listable)

	appStatus := byName["app-status"]
	assert.Equal(t, KindHashicorpVault, appStatus.Kind)
	assert.True(t, appStatus.HasStatus)
	assert.False(t, appStatus.Deletable)
	assert.False(t, appStatus.Local)
	assert.False(t, appStatus.Listable)

	appLocal := byName["app-local"]
	assert.Equal(t, KindKeychain, appLocal.Kind)
	assert.True(t, appLocal.Local)
	assert.False(t, appLocal.HasStatus)
	assert.False(t, appLocal.Deletable)
	assert.False(t, appLocal.Listable)

	appListable := byName["app-listable"]
	assert.Equal(t, KindHashicorpVault, appListable.Kind)
	assert.True(t, appListable.Listable)
	assert.False(t, appListable.Deletable)
	assert.False(t, appListable.HasStatus)
	assert.False(t, appListable.Local)

	// A store present in StoresConfig but missing from the live registry (construction failed)
	// still gets a descriptor -- from config alone -- with every capability flag left false.
	appMissing := byName["app-missing"]
	assert.Equal(t, KindAWSSSM, appMissing.Kind)
	assert.False(t, appMissing.Secret)
	assert.False(t, appMissing.Deletable)
	assert.False(t, appMissing.HasStatus)
	assert.False(t, appMissing.Local)
	assert.False(t, appMissing.Listable)
}

func TestService_IsSecret(t *testing.T) {
	svc := NewService(StoresConfig{
		"app-secrets": {Kind: KindAWSSSM, Secret: true},
		"app-cache":   {Kind: KindRedis},
	}, StoreRegistry{})

	assert.True(t, svc.IsSecret("app-secrets"))
	assert.False(t, svc.IsSecret("app-cache"))
	assert.False(t, svc.IsSecret("missing"))
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
