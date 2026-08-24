package store

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// These smoke tests exercise the generated GoMock doubles the way consumers (e.g. pkg/secrets) use
// them: program expectations via EXPECT(), invoke the interface methods, and let gomock verify the
// call wiring. They guard the generated mocks against silent breakage on regeneration.

func TestMockStore_DrivesAllMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := NewMockStore(ctrl)

	m.EXPECT().Set("prod", "api", "k", "v").Return(nil)
	m.EXPECT().Get("prod", "api", "k").Return("v", nil)
	m.EXPECT().GetKey("k").Return("v", nil)

	require.NoError(t, m.Set("prod", "api", "k", "v"))

	got, err := m.Get("prod", "api", "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)

	got, err = m.GetKey("k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)

	// Confirm the mock satisfies the Store interface.
	var _ Store = m
}

func TestMockDeletableStore_DrivesAllMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := NewMockDeletableStore(ctrl)

	m.EXPECT().Set("prod", "api", "k", "v").Return(nil)
	m.EXPECT().Get("prod", "api", "k").Return("v", nil)
	m.EXPECT().GetKey("k").Return("v", nil)
	m.EXPECT().Delete("prod", "api", "k").Return(nil)

	require.NoError(t, m.Set("prod", "api", "k", "v"))
	_, err := m.Get("prod", "api", "k")
	require.NoError(t, err)
	_, err = m.GetKey("k")
	require.NoError(t, err)
	require.NoError(t, m.Delete("prod", "api", "k"))

	var _ DeletableStore = m
}

func TestMockStatusStore_DrivesAllMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := NewMockStatusStore(ctrl)

	m.EXPECT().Set("prod", "api", "k", "v").Return(nil)
	m.EXPECT().Get("prod", "api", "k").Return("v", nil)
	m.EXPECT().GetKey("k").Return("v", nil)
	m.EXPECT().Has("prod", "api", "k").Return(true, nil)

	require.NoError(t, m.Set("prod", "api", "k", "v"))
	_, err := m.Get("prod", "api", "k")
	require.NoError(t, err)
	_, err = m.GetKey("k")
	require.NoError(t, err)
	has, err := m.Has("prod", "api", "k")
	require.NoError(t, err)
	assert.True(t, has)

	var _ StatusStore = m
}

func TestMockSecretAwareStore_DrivesAllMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := NewMockSecretAwareStore(ctrl)

	m.EXPECT().Set("prod", "api", "k", "v").Return(nil)
	m.EXPECT().Get("prod", "api", "k").Return("v", nil)
	m.EXPECT().GetKey("k").Return("v", nil)
	m.EXPECT().SetSecret(true)

	require.NoError(t, m.Set("prod", "api", "k", "v"))
	_, err := m.Get("prod", "api", "k")
	require.NoError(t, err)
	_, err = m.GetKey("k")
	require.NoError(t, err)
	m.SetSecret(true)

	var _ SecretAwareStore = m
}

func TestMockAuthContextResolver_DrivesAllMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := NewMockAuthContextResolver(ctrl)
	ctx := context.Background()
	boom := errors.New("denied")

	m.EXPECT().ResolveAWSAuthContext(ctx, "aws/admin").Return(&AWSAuthConfig{}, nil)
	m.EXPECT().ResolveAzureAuthContext(ctx, "az/admin").Return(&AzureAuthConfig{}, nil)
	m.EXPECT().ResolveGCPAuthContext(ctx, "gcp/admin").Return(nil, boom)

	aws, err := m.ResolveAWSAuthContext(ctx, "aws/admin")
	require.NoError(t, err)
	require.NotNil(t, aws)

	az, err := m.ResolveAzureAuthContext(ctx, "az/admin")
	require.NoError(t, err)
	require.NotNil(t, az)

	_, err = m.ResolveGCPAuthContext(ctx, "gcp/admin")
	require.ErrorIs(t, err, boom)

	var _ AuthContextResolver = m
}

func TestMockIdentityAwareStore_DrivesAllMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := NewMockIdentityAwareStore(ctrl)
	resolver := NewMockAuthContextResolver(ctrl)

	m.EXPECT().Set("prod", "api", "k", "v").Return(nil)
	m.EXPECT().Get("prod", "api", "k").Return("v", nil)
	m.EXPECT().GetKey("k").Return("v", nil)
	m.EXPECT().SetAuthContext(resolver, "aws/admin")

	require.NoError(t, m.Set("prod", "api", "k", "v"))
	_, err := m.Get("prod", "api", "k")
	require.NoError(t, err)
	_, err = m.GetKey("k")
	require.NoError(t, err)
	m.SetAuthContext(resolver, "aws/admin")

	var _ IdentityAwareStore = m
}
