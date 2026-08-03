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

func TestService_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	deletable := NewMockDeletableStore(ctrl)
	plain := NewMockStore(ctrl)

	svc := NewService(
		StoresConfig{
			"app-secrets": {Kind: KindAWSSSM, Secret: true},
			"app-cache":   {Kind: KindRedis},
		},
		StoreRegistry{
			"app-secrets": deletable,
			"app-cache":   plain,
		},
	)

	descriptors := svc.List()
	require.Len(t, descriptors, 2)

	// Sorted by name: "app-cache" before "app-secrets".
	assert.Equal(t, "app-cache", descriptors[0].Name)
	assert.Equal(t, KindRedis, descriptors[0].Kind)
	assert.False(t, descriptors[0].Secret)
	assert.False(t, descriptors[0].Deletable)

	assert.Equal(t, "app-secrets", descriptors[1].Name)
	assert.Equal(t, KindAWSSSM, descriptors[1].Kind)
	assert.True(t, descriptors[1].Secret)
	assert.True(t, descriptors[1].Deletable)
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
