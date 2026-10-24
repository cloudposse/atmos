package sopsauth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/store"
)

// errResolve is a sentinel used to assert that resolver errors are wrapped and surfaced.
var errResolve = errors.New("resolve failed")

func TestAWSKMS(t *testing.T) {
	t.Run("resolver error is wrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveAWSAuthContext(gomock.Any(), "id").Return(nil, errResolve)

		applier, err := NewBuilder(resolver).AWSKMS(context.Background(), "id")
		require.ErrorIs(t, err, errResolve)
		assert.Nil(t, applier)
	})

	t.Run("nil auth context returns explicit error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveAWSAuthContext(gomock.Any(), "id").Return(nil, nil)

		applier, err := NewBuilder(resolver).AWSKMS(context.Background(), "id")
		require.ErrorIs(t, err, errEmptyAWSAuthContext)
		assert.Nil(t, applier)
	})

	t.Run("success returns a KMS applier", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		// Region-only config avoids touching shared-config files/profiles that may not exist in CI.
		resolver.EXPECT().ResolveAWSAuthContext(gomock.Any(), "id").Return(&store.AWSAuthConfig{Region: "us-east-1"}, nil)

		applier, err := NewBuilder(resolver).AWSKMS(context.Background(), "id")
		require.NoError(t, err)
		assert.NotNil(t, applier)
	})
}

func TestAWSAuthConfigOpts(t *testing.T) {
	t.Run("all fields produce one option each", func(t *testing.T) {
		opts := awsAuthConfigOpts(&store.AWSAuthConfig{
			CredentialsFile: "creds",
			ConfigFile:      "config",
			Profile:         "prof",
			Region:          "us-west-2",
		})
		assert.Len(t, opts, 4, "each populated field should contribute exactly one load option")
	})

	t.Run("empty config produces no options", func(t *testing.T) {
		assert.Empty(t, awsAuthConfigOpts(&store.AWSAuthConfig{}))
	})
}

func TestGCPKMS(t *testing.T) {
	t.Run("resolver error is wrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveGCPAuthContext(gomock.Any(), "id").Return(nil, errResolve)

		applier, err := NewBuilder(resolver).GCPKMS(context.Background(), "id")
		require.ErrorIs(t, err, errResolve)
		assert.Nil(t, applier)
	})

	t.Run("nil auth context returns explicit error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveGCPAuthContext(gomock.Any(), "id").Return(nil, nil)

		applier, err := NewBuilder(resolver).GCPKMS(context.Background(), "id")
		require.ErrorIs(t, err, errEmptyGCPAuthContext)
		assert.Nil(t, applier)
	})

	t.Run("access token yields a token-source applier", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveGCPAuthContext(gomock.Any(), "id").Return(&store.GCPAuthConfig{AccessToken: "ya29.token"}, nil)

		applier, err := NewBuilder(resolver).GCPKMS(context.Background(), "id")
		require.NoError(t, err)
		assert.NotNil(t, applier)
	})

	t.Run("credentials file yields a credential-json applier", func(t *testing.T) {
		dir := t.TempDir()
		credPath := filepath.Join(dir, "creds.json")
		require.NoError(t, os.WriteFile(credPath, []byte(`{"type":"service_account"}`), 0o600))

		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveGCPAuthContext(gomock.Any(), "id").Return(&store.GCPAuthConfig{CredentialsFile: credPath}, nil)

		applier, err := NewBuilder(resolver).GCPKMS(context.Background(), "id")
		require.NoError(t, err)
		assert.NotNil(t, applier)
	})

	t.Run("unreadable credentials file is an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		missing := filepath.Join(t.TempDir(), "does-not-exist.json")
		resolver.EXPECT().ResolveGCPAuthContext(gomock.Any(), "id").Return(&store.GCPAuthConfig{CredentialsFile: missing}, nil)

		applier, err := NewBuilder(resolver).GCPKMS(context.Background(), "id")
		require.Error(t, err)
		assert.Nil(t, applier)
	})

	t.Run("no credentials returns the no-credentials error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveGCPAuthContext(gomock.Any(), "id").Return(&store.GCPAuthConfig{}, nil)

		applier, err := NewBuilder(resolver).GCPKMS(context.Background(), "id")
		require.ErrorIs(t, err, errNoGCPCredentials)
		assert.Nil(t, applier)
	})
}

func TestAzureKV(t *testing.T) {
	t.Run("resolver error is wrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveAzureAuthContext(gomock.Any(), "id").Return(nil, errResolve)

		applier, err := NewBuilder(resolver).AzureKV(context.Background(), "id")
		require.ErrorIs(t, err, errResolve)
		assert.Nil(t, applier)
	})

	t.Run("nil auth context returns explicit error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveAzureAuthContext(gomock.Any(), "id").Return(nil, nil)

		applier, err := NewBuilder(resolver).AzureKV(context.Background(), "id")
		require.ErrorIs(t, err, errEmptyAzureAuthContext)
		assert.Nil(t, applier)
	})

	t.Run("success with tenant hint returns a token-credential applier", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveAzureAuthContext(gomock.Any(), "id").Return(&store.AzureAuthConfig{TenantID: "tenant-123"}, nil)

		applier, err := NewBuilder(resolver).AzureKV(context.Background(), "id")
		require.NoError(t, err)
		assert.NotNil(t, applier)
	})

	t.Run("success without tenant hint returns a token-credential applier", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		resolver := store.NewMockAuthContextResolver(ctrl)
		resolver.EXPECT().ResolveAzureAuthContext(gomock.Any(), "id").Return(&store.AzureAuthConfig{}, nil)

		applier, err := NewBuilder(resolver).AzureKV(context.Background(), "id")
		require.NoError(t, err)
		assert.NotNil(t, applier)
	})
}
