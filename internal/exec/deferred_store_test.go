package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
)

func TestDeferredStoreFailedAuthStopsRead(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := store.NewMockIdentityAwareStore(ctrl)
	ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": s}}
	authdeferred.ConfigureAuth(ac, "")
	factory := authdeferred.NewMockAuthFactory(ctrl)
	setDeferredAuthFactory(ac, factory)
	factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, unavailableAuth(errUtils.ErrExpiredCredentials)).Times(1)
	s.EXPECT().ResetAuthContext().Times(2)
	for range 2 {
		_, err := storedeferred.ReadStore(ac, "!store.get remote key | default fallback", "dev", &schema.ConfigAndStacksInfo{Stack: "dev"})
		require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
	}
}

func TestDeferredStoreSuccessfulAuthIsNotRepeated(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := store.NewMockIdentityAwareStore(ctrl)
	ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": s}}
	authdeferred.ConfigureAuth(ac, "")
	factory := authdeferred.NewMockAuthFactory(ctrl)
	setDeferredAuthFactory(ac, factory)
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().GetChain().Return([]string{"local"}).AnyTimes()
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "local"}}}).AnyTimes()
	factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(manager, nil).Times(1)
	s.EXPECT().ResetAuthContext().Times(2)
	s.EXPECT().SetAuthContext(gomock.Any(), "local").Do(func(resolver store.AuthContextResolver, name string) {
		resolved, err := resolver.ResolveAWSAuthContext(t.Context(), name)
		require.NoError(t, err)
		assert.Equal(t, "local", resolved.Profile)
	}).Times(2)
	s.EXPECT().GetKey("key").Return("resolved", nil).Times(2)
	for range 2 {
		value, err := storedeferred.ReadStore(ac, "!store.get remote key", "dev", &schema.ConfigAndStacksInfo{Stack: "dev"})
		require.NoError(t, err)
		assert.Equal(t, "resolved", value)
	}
}

func TestDeferredStoreDisabledAndLocal(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := store.NewMockIdentityAwareStore(ctrl)
	ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": s}}
	authdeferred.ConfigureAuth(ac, "false")
	setDeferredAuthFactory(ac, authdeferred.NewMockAuthFactory(ctrl))
	s.EXPECT().ResetAuthContext()
	require.NoError(t, storedeferred.ResolveStoreAuth(ac, &schema.ConfigAndStacksInfo{}, "remote"))
	authdeferred.ConfigureAuth(ac, "")
	setDeferredAuthFactory(ac, authdeferred.NewMockAuthFactory(ctrl))
	local := store.NewMockStore(ctrl)
	ac.Stores["local"] = local
	local.EXPECT().GetKey("key").Return("local value", nil)
	value, err := storedeferred.ReadStore(ac, "!store.get local key", "dev", nil)
	require.NoError(t, err)
	assert.Equal(t, "local value", value)
}
