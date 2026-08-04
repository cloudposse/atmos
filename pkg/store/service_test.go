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

func TestService_Get_StoreNotConfigured(t *testing.T) {
	svc := NewService(StoresConfig{}, StoreRegistry{})

	_, err := svc.Get("missing-store", "", "", "somekey")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStoreNotConfigured)
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

func TestService_GetKey_StoreNotConfigured(t *testing.T) {
	svc := NewService(StoresConfig{}, StoreRegistry{})

	_, err := svc.GetKey("missing-store", "somekey")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStoreNotConfigured)
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

// TestService_ListKeyValues_ValueListingNotSupported proves ListKeyValues checks
// ValueListableStore up front and fails fast with ErrListNotSupported -- without calling Keys or
// Get -- when value listing isn't currently supported (e.g. a GitHub Actions store run outside a
// runner).
func TestService_ListKeyValues_ValueListingNotSupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockValueListableStore(ctrl)
	mockStore.EXPECT().ValueListingSupported().Return(false)
	// No EXPECT() calls for Keys/Get: they must never be invoked.

	svc := NewService(StoresConfig{"app": {Kind: KindGitHubActions}}, StoreRegistry{"app": mockStore})

	_, err := svc.ListKeyValues("app", "prod", "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrListNotSupported)
}

// TestService_ListKeyValues_ValueListingSupported proves a ValueListableStore that reports
// support proceeds through the normal Keys/Get enumeration unchanged.
func TestService_ListKeyValues_ValueListingSupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := NewMockValueListableStore(ctrl)
	mockStore.EXPECT().ValueListingSupported().Return(true)
	mockStore.EXPECT().Keys("prod", "vpc").Return([]string{"image_tag"}, nil)
	mockStore.EXPECT().Get("prod", "vpc", "image_tag").Return("v1.2.3", nil)

	svc := NewService(StoresConfig{"app": {Kind: KindGitHubActions}}, StoreRegistry{"app": mockStore})

	kvs, err := svc.ListKeyValues("app", "prod", "vpc")
	require.NoError(t, err)
	assert.Equal(t, []KeyValue{{Key: "image_tag", Value: "v1.2.3"}}, kvs)
}

func TestService_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	deletable := NewMockDeletableStore(ctrl)
	plain := NewMockStore(ctrl)
	statusStore := NewMockStatusStore(ctrl)
	localStore := NewMockLocalStore(ctrl)
	localStore.EXPECT().IsLocal().Return(true)
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

	names := make([]string, len(descriptors))
	for i, d := range descriptors {
		names[i] = d.Name
	}
	// List() sorts by name -- guards against sorting regressions the map-based lookups below can't catch.
	assert.Equal(t, []string{"app-cache", "app-listable", "app-local", "app-missing", "app-secrets", "app-status"}, names)

	byName := make(map[string]Descriptor, len(descriptors))
	for _, d := range descriptors {
		byName[d.Name] = d
	}

	tests := []struct {
		name          string
		wantKind      string
		wantSecret    bool
		wantDeletable bool
		wantHasStatus bool
		wantLocal     bool
		wantListable  bool
	}{
		{name: "app-cache", wantKind: KindRedis},
		{name: "app-secrets", wantKind: KindAWSSSM, wantSecret: true, wantDeletable: true},
		{name: "app-status", wantKind: KindHashicorpVault, wantHasStatus: true},
		{name: "app-local", wantKind: KindKeychain, wantLocal: true},
		{name: "app-listable", wantKind: KindHashicorpVault, wantListable: true},
		// Configured, but missing from the live registry (construction failed): still gets a
		// descriptor -- from config alone -- with every capability flag left false.
		{name: "app-missing", wantKind: KindAWSSSM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := byName[tt.name]
			assert.Equal(t, tt.wantKind, d.Kind)
			assert.Equal(t, tt.wantSecret, d.Secret)
			assert.Equal(t, tt.wantDeletable, d.Deletable)
			assert.Equal(t, tt.wantHasStatus, d.HasStatus)
			assert.Equal(t, tt.wantLocal, d.Local)
			assert.Equal(t, tt.wantListable, d.Listable)
		})
	}
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
