package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
)

// redisSecretOpts and artifactorySecretOpts hold the minimum options for each backend to
// construct successfully (lazily, without a live connection) so the secret-capability guard
// is the thing under test.
var (
	redisSecretOpts       = map[string]any{"url": "redis://localhost:6379"}
	artifactorySecretOpts = map[string]any{"url": "https://example.jfrog.io", "repo_name": "repo", "access_token": "anonymous"}
)

// TestNewStoreRegistry_SecretOnIncapableBackendErrors verifies that marking a backend that
// cannot encrypt at rest (Redis, Artifactory) as `secret: true` is a hard error at load,
// regardless of whether the backend is selected via the legacy `type` or the new `kind`.
func TestNewStoreRegistry_SecretOnIncapableBackendErrors(t *testing.T) {
	tests := []struct {
		name   string
		config store.StoreConfig
	}{
		{"redis via type", store.StoreConfig{Type: "redis", Secret: true, Options: redisSecretOpts}},
		{"redis via kind", store.StoreConfig{Kind: "redis", Secret: true, Options: redisSecretOpts}},
		{"artifactory via type", store.StoreConfig{Type: "artifactory", Secret: true, Options: artifactorySecretOpts}},
		{"artifactory via kind", store.StoreConfig{Kind: "artifactory", Secret: true, Options: artifactorySecretOpts}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := store.StoresConfig{"store": tt.config}
			registry, err := store.NewStoreRegistry(&cfg)
			require.Error(t, err)
			assert.ErrorIs(t, err, store.ErrSecretBackendNotEncrypted)
			assert.Nil(t, registry)
		})
	}
}

// TestNewStoreRegistry_IncapableBackendWithoutSecretBuilds verifies the guard fires only on
// the `secret` flag: the same backends build fine when not marked secret.
func TestNewStoreRegistry_IncapableBackendWithoutSecretBuilds(t *testing.T) {
	tests := []struct {
		name   string
		config store.StoreConfig
	}{
		{"redis", store.StoreConfig{Type: "redis", Options: redisSecretOpts}},
		{"artifactory", store.StoreConfig{Type: "artifactory", Options: artifactorySecretOpts}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := store.StoresConfig{"store": tt.config}
			registry, err := store.NewStoreRegistry(&cfg)
			require.NoError(t, err)
			s, ok := registry["store"]
			assert.True(t, ok)
			assert.NotNil(t, s)
		})
	}
}

// TestNewStoreRegistry_SecretSetsSecureWrite verifies that a `secret: true` SSM store is built
// with its secret flag set, so writes use the SecureString variant. It lives in the providers
// package (not pkg/store) because it asserts on the concrete *SSMStore secret field, which is
// only observable from within this package.
func TestNewStoreRegistry_SecretSetsSecureWrite(t *testing.T) {
	cfg := store.StoresConfig{
		"app-secrets": store.StoreConfig{
			Type:    "aws-ssm-parameter-store",
			Secret:  true,
			Options: map[string]any{"region": "us-east-1"},
		},
	}
	registry, err := store.NewStoreRegistry(&cfg)
	assert.NoError(t, err)

	s, ok := registry["app-secrets"]
	assert.True(t, ok)

	ssm, ok := s.(*SSMStore)
	assert.True(t, ok)
	// A `secret: true` SSM store must write the SecureString variant.
	assert.True(t, ssm.secret)
}
