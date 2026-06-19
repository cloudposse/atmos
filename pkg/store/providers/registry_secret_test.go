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

func TestNewStoreRegistry_AWSKinds(t *testing.T) {
	cfg := store.StoresConfig{
		"ssm": store.StoreConfig{
			Kind:    "aws/ssm",
			Options: map[string]any{"region": "us-east-1"},
		},
		"asm": store.StoreConfig{
			Kind:    "aws/asm",
			Options: map[string]any{"region": "us-east-1"},
		},
	}
	registry, err := store.NewStoreRegistry(&cfg)
	require.NoError(t, err)

	_, ok := registry["ssm"].(*SSMStore)
	assert.True(t, ok)

	_, ok = registry["asm"].(*SecretsManagerStore)
	assert.True(t, ok)
}

func TestNewStoreRegistry_AzureAndGCPKinds(t *testing.T) {
	cfg := store.StoresConfig{
		"azure": store.StoreConfig{
			Identity: "azure/test",
			Kind:     "azure/keyvault",
			Options:  map[string]any{"vault_url": "https://test.vault.azure.net"},
		},
		"gcp": store.StoreConfig{
			Kind:    "gcp/secretmanager",
			Options: map[string]any{"project_id": "test-project"},
		},
		"google": store.StoreConfig{
			Kind:    "google/secretmanager",
			Options: map[string]any{"project_id": "test-project"},
		},
	}
	registry, err := store.NewStoreRegistry(&cfg)
	require.NoError(t, err)

	_, ok := registry["azure"].(*AzureKeyVaultStore)
	assert.True(t, ok)

	_, ok = registry["gcp"].(*GSMStore)
	assert.True(t, ok)

	_, ok = registry["google"].(*GSMStore)
	assert.True(t, ok)
}

func TestNewStoreRegistry_SupportedStoreMatrix(t *testing.T) {
	strPtr := func(v string) *string { return &v }

	tests := []struct {
		name    string
		config  store.StoreConfig
		asserts func(t *testing.T, s store.Store)
	}{
		{
			name:   "aws ssm kind",
			config: store.StoreConfig{Kind: store.KindAWSSSM, Options: map[string]any{"region": "us-east-1"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*SSMStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "aws ssm legacy type",
			config: store.StoreConfig{Type: "aws-ssm-parameter-store", Options: map[string]any{"region": "us-east-1"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*SSMStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "aws secrets manager kind",
			config: store.StoreConfig{Kind: store.KindAWSASM, Options: map[string]any{"region": "us-east-1"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*SecretsManagerStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "aws secrets manager legacy type",
			config: store.StoreConfig{Type: "aws-secrets-manager", Options: map[string]any{"region": "us-east-1"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*SecretsManagerStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "azure key vault kind",
			config: store.StoreConfig{Kind: store.KindAzureKeyVault, Identity: "azure/test", Options: map[string]any{"vault_url": "https://test.vault.azure.net"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*AzureKeyVaultStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "azure key vault legacy type",
			config: store.StoreConfig{Type: "azure-key-vault", Identity: "azure/test", Options: map[string]any{"vault_url": "https://test.vault.azure.net"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*AzureKeyVaultStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "gcp secret manager kind",
			config: store.StoreConfig{Kind: store.KindGCPSecret, Options: map[string]any{"project_id": "test-project"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*GSMStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "gcp secret manager legacy alias",
			config: store.StoreConfig{Type: "gsm", Options: map[string]any{"project_id": "test-project"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*GSMStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "hashicorp vault kind",
			config: store.StoreConfig{Kind: store.KindHashicorpVault, Options: map[string]any{"mount": "secret", "address": "https://vault.example", "token": "t"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*VaultStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "hashicorp vault legacy type",
			config: store.StoreConfig{Type: "hashicorp-vault", Options: map[string]any{"mount": "secret", "address": "https://vault.example", "token": "t"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*VaultStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "onepassword kind",
			config: store.StoreConfig{Kind: store.KindOnePassword, Options: map[string]any{"vault": "Shared"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*OnePasswordStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "onepassword legacy type",
			config: store.StoreConfig{Type: "1password", Options: map[string]any{"vault": "Shared"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*OnePasswordStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "keychain kind",
			config: store.StoreConfig{Kind: store.KindKeychain, Options: map[string]any{"backend": "memory"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*KeychainStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "keychain legacy keyring type",
			config: store.StoreConfig{Type: "keyring", Options: map[string]any{"backend": "memory"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*KeychainStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "github actions kind",
			config: store.StoreConfig{Kind: store.KindGitHubActions, Options: map[string]any{"owner": "cloudposse", "repo": "atmos"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*GitHubActionsStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "github actions legacy type",
			config: store.StoreConfig{Type: "github-actions", Options: map[string]any{"owner": "cloudposse", "repo": "atmos"}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*GitHubActionsStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "redis kind",
			config: store.StoreConfig{Kind: store.KindRedis, Options: map[string]any{"url": strPtr("redis://localhost:6379")}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*RedisStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "redis legacy type",
			config: store.StoreConfig{Type: "redis", Options: map[string]any{"url": strPtr("redis://localhost:6379")}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*RedisStore)
				assert.True(t, ok)
			},
		},
		{
			name: "artifactory kind",
			config: store.StoreConfig{Kind: store.KindArtifactory, Options: map[string]any{
				"access_token": strPtr("anonymous"),
				"url":          "https://example.jfrog.io/artifactory",
				"repo_name":    "test-repo",
			}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*ArtifactoryStore)
				assert.True(t, ok)
			},
		},
		{
			name: "artifactory legacy type",
			config: store.StoreConfig{Type: "artifactory", Options: map[string]any{
				"access_token": strPtr("anonymous"),
				"url":          "https://example.jfrog.io/artifactory",
				"repo_name":    "test-repo",
			}},
			asserts: func(t *testing.T, s store.Store) {
				_, ok := s.(*ArtifactoryStore)
				assert.True(t, ok)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := store.StoresConfig{"store": tt.config}
			store.ApplySecretDefaults(cfg)

			registry, err := store.NewStoreRegistry(&cfg)
			require.NoError(t, err)
			require.Contains(t, registry, "store")
			tt.asserts(t, registry["store"])
		})
	}
}

func TestNewStoreRegistry_CustomEndpointOptions(t *testing.T) {
	config := &store.StoresConfig{
		"local-ssm": store.StoreConfig{
			Kind:     store.KindAWSSSM,
			Identity: "aws/local",
			Options: map[string]interface{}{
				"region":   "us-east-1",
				"endpoint": "http://localhost:4566",
			},
		},
		"local-asm": store.StoreConfig{
			Kind:     store.KindAWSASM,
			Identity: "aws/local",
			Options: map[string]interface{}{
				"region":       "us-east-1",
				"endpoint_url": "http://localhost:4566",
			},
		},
		"local-azure": store.StoreConfig{
			Kind:     store.KindAzureKeyVault,
			Identity: "azure/local",
			Options: map[string]interface{}{
				"endpoint": "http://localhost:4567",
				"disable_challenge_resource_verification": true,
				"without_authentication":                  true,
			},
		},
		"local-gcp": store.StoreConfig{
			Kind:     store.KindGCPSecret,
			Identity: "gcp/local",
			Options: map[string]interface{}{
				"project_id":             "local-project",
				"endpoint":               "http://localhost:4568",
				"endpoint_insecure":      true,
				"without_authentication": true,
			},
		},
	}

	registry, err := store.NewStoreRegistry(config)
	assert.NoError(t, err)

	ssmStore := registry["local-ssm"].(*SSMStore)
	assert.Equal(t, "http://localhost:4566", ssmStore.endpoint)

	asmStore := registry["local-asm"].(*SecretsManagerStore)
	assert.Equal(t, "http://localhost:4566", asmStore.endpoint)

	azureStore := registry["local-azure"].(*AzureKeyVaultStore)
	assert.Equal(t, "http://localhost:4567", azureStore.vaultURL)
	assert.True(t, azureStore.clientOptions.DisableChallengeResourceVerification)
	assert.True(t, azureStore.clientOptions.InsecureAllowCredentialWithHTTP)
	assert.True(t, azureStore.withoutAuth)

	gsmStore := registry["local-gcp"].(*GSMStore)
	assert.Equal(t, "http://localhost:4568", gsmStore.endpoint)
	assert.True(t, gsmStore.endpointInsecure)
	assert.True(t, gsmStore.withoutAuthentication)
}
