package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapLegacyType(t *testing.T) {
	cases := map[string]string{
		"aws-ssm-parameter-store": KindAWSSSM,
		"aws-secrets-manager":     KindAWSASM,
		"hashicorp-vault":         KindHashicorpVault,
		"azure-key-vault":         KindAzureKeyVault,
		"google/secretmanager":    KindGCPSecret,
		"google-secret-manager":   KindGCPSecret,
		"gsm":                     KindGCPSecret,
		"redis":                   KindRedis,
		"artifactory":             KindArtifactory,
		// Unknown values pass through unchanged so a kind can be supplied via `type`.
		"aws/ssm": "aws/ssm",
	}
	for in, want := range cases {
		assert.Equal(t, want, mapLegacyType(in), "mapLegacyType(%q)", in)
	}
}

func TestResolveKind_KindTakesPrecedence(t *testing.T) {
	// Explicit kind wins over legacy type.
	got := resolveKind(StoreConfig{Type: "aws-ssm-parameter-store", Kind: "aws/asm"})
	assert.Equal(t, KindAWSASM, got)

	// Falls back to legacy type when kind is empty.
	got = resolveKind(StoreConfig{Type: "aws-ssm-parameter-store"})
	assert.Equal(t, KindAWSSSM, got)
}

func TestMapLegacyType_OnePasswordAliases(t *testing.T) {
	assert.Equal(t, KindOnePassword, mapLegacyType("onepassword"))
	assert.Equal(t, KindOnePassword, mapLegacyType("1password"))
}

func TestApplySecretDefaults_OnePasswordImpliesSecret(t *testing.T) {
	cfg := StoresConfig{
		// 1Password without explicit `secret:` must become a secret store.
		"op": StoreConfig{Type: "onepassword"},
		// Local/CI secret backends are also secret-by-default.
		"keychain": StoreConfig{Type: "keyring"},
		"github":   StoreConfig{Kind: "github/actions"},
		// An explicit secret:false on a non-secret-by-default kind is left untouched.
		"ssm": StoreConfig{Type: "aws-ssm-parameter-store"},
	}
	ApplySecretDefaults(cfg)
	assert.True(t, cfg["op"].Secret, "1Password store should default to secret: true")
	assert.True(t, cfg["keychain"].Secret, "Keychain store should default to secret: true")
	assert.True(t, cfg["github"].Secret, "GitHub Actions store should default to secret: true")
	assert.False(t, cfg["ssm"].Secret, "SSM store should not be forced secret")
}

func TestApplySecretDefaults_RespectsExplicitSecret(t *testing.T) {
	cfg := StoresConfig{
		"op": StoreConfig{Kind: "onepassword", Secret: true},
	}
	ApplySecretDefaults(cfg)
	assert.True(t, cfg["op"].Secret)
}

func TestNewStoreRegistry_SecretSetsSecureWrite(t *testing.T) {
	cfg := StoresConfig{
		"app-secrets": StoreConfig{
			Type:    "aws-ssm-parameter-store",
			Secret:  true,
			Options: map[string]any{"region": "us-east-1"},
		},
	}
	registry, err := NewStoreRegistry(&cfg)
	assert.NoError(t, err)

	s, ok := registry["app-secrets"]
	assert.True(t, ok)

	ssm, ok := s.(*SSMStore)
	assert.True(t, ok)
	// A `secret: true` SSM store must write the SecureString variant.
	assert.True(t, ssm.secret)
}

func TestNewStoreRegistry_AWSKinds(t *testing.T) {
	cfg := StoresConfig{
		"ssm": StoreConfig{
			Kind:    "aws/ssm",
			Options: map[string]any{"region": "us-east-1"},
		},
		"asm": StoreConfig{
			Kind:    "aws/asm",
			Options: map[string]any{"region": "us-east-1"},
		},
	}
	registry, err := NewStoreRegistry(&cfg)
	require.NoError(t, err)

	_, ok := registry["ssm"].(*SSMStore)
	assert.True(t, ok)

	_, ok = registry["asm"].(*SecretsManagerStore)
	assert.True(t, ok)
}

func TestNewStoreRegistry_AzureAndGCPKinds(t *testing.T) {
	cfg := StoresConfig{
		"azure": StoreConfig{
			Identity: "azure/test",
			Kind:     "azure/keyvault",
			Options:  map[string]any{"vault_url": "https://test.vault.azure.net"},
		},
		"gcp": StoreConfig{
			Kind:    "gcp/secretmanager",
			Options: map[string]any{"project_id": "test-project"},
		},
		"google": StoreConfig{
			Kind:    "google/secretmanager",
			Options: map[string]any{"project_id": "test-project"},
		},
	}
	registry, err := NewStoreRegistry(&cfg)
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
		config  StoreConfig
		asserts func(t *testing.T, s Store)
	}{
		{
			name:   "aws ssm kind",
			config: StoreConfig{Kind: KindAWSSSM, Options: map[string]any{"region": "us-east-1"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*SSMStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "aws ssm legacy type",
			config: StoreConfig{Type: "aws-ssm-parameter-store", Options: map[string]any{"region": "us-east-1"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*SSMStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "aws secrets manager kind",
			config: StoreConfig{Kind: KindAWSASM, Options: map[string]any{"region": "us-east-1"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*SecretsManagerStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "aws secrets manager legacy type",
			config: StoreConfig{Type: "aws-secrets-manager", Options: map[string]any{"region": "us-east-1"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*SecretsManagerStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "azure key vault kind",
			config: StoreConfig{Kind: KindAzureKeyVault, Identity: "azure/test", Options: map[string]any{"vault_url": "https://test.vault.azure.net"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*AzureKeyVaultStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "azure key vault legacy type",
			config: StoreConfig{Type: "azure-key-vault", Identity: "azure/test", Options: map[string]any{"vault_url": "https://test.vault.azure.net"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*AzureKeyVaultStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "gcp secret manager kind",
			config: StoreConfig{Kind: KindGCPSecret, Options: map[string]any{"project_id": "test-project"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*GSMStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "gcp secret manager legacy alias",
			config: StoreConfig{Type: "gsm", Options: map[string]any{"project_id": "test-project"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*GSMStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "hashicorp vault kind",
			config: StoreConfig{Kind: KindHashicorpVault, Options: map[string]any{"mount": "secret", "address": "https://vault.example", "token": "t"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*VaultStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "hashicorp vault legacy type",
			config: StoreConfig{Type: "hashicorp-vault", Options: map[string]any{"mount": "secret", "address": "https://vault.example", "token": "t"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*VaultStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "onepassword kind",
			config: StoreConfig{Kind: KindOnePassword, Options: map[string]any{"vault": "Shared"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*OnePasswordStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "onepassword legacy type",
			config: StoreConfig{Type: "1password", Options: map[string]any{"vault": "Shared"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*OnePasswordStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "keychain kind",
			config: StoreConfig{Kind: KindKeychain, Options: map[string]any{"backend": "memory"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*KeychainStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "keychain legacy keyring type",
			config: StoreConfig{Type: "keyring", Options: map[string]any{"backend": "memory"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*KeychainStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "github actions kind",
			config: StoreConfig{Kind: KindGitHubActions, Options: map[string]any{"owner": "cloudposse", "repo": "atmos"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*GitHubActionsStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "github actions legacy type",
			config: StoreConfig{Type: "github-actions", Options: map[string]any{"owner": "cloudposse", "repo": "atmos"}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*GitHubActionsStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "redis kind",
			config: StoreConfig{Kind: KindRedis, Options: map[string]any{"url": strPtr("redis://localhost:6379")}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*RedisStore)
				assert.True(t, ok)
			},
		},
		{
			name:   "redis legacy type",
			config: StoreConfig{Type: "redis", Options: map[string]any{"url": strPtr("redis://localhost:6379")}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*RedisStore)
				assert.True(t, ok)
			},
		},
		{
			name: "artifactory kind",
			config: StoreConfig{Kind: KindArtifactory, Options: map[string]any{
				"access_token": strPtr("anonymous"),
				"url":          "https://example.jfrog.io/artifactory",
				"repo_name":    "test-repo",
			}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*ArtifactoryStore)
				assert.True(t, ok)
			},
		},
		{
			name: "artifactory legacy type",
			config: StoreConfig{Type: "artifactory", Options: map[string]any{
				"access_token": strPtr("anonymous"),
				"url":          "https://example.jfrog.io/artifactory",
				"repo_name":    "test-repo",
			}},
			asserts: func(t *testing.T, s Store) {
				_, ok := s.(*ArtifactoryStore)
				assert.True(t, ok)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := StoresConfig{"store": tt.config}
			ApplySecretDefaults(cfg)

			registry, err := NewStoreRegistry(&cfg)
			require.NoError(t, err)
			require.Contains(t, registry, "store")
			tt.asserts(t, registry["store"])
		})
	}
}

// redisSecretOpts and artifactorySecretOpts hold the minimum options for each backend to
// construct successfully (lazily, without a live connection) so the secret-capability guard,
// which runs after the store is built, is the thing under test.
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
		config StoreConfig
	}{
		{"redis via type", StoreConfig{Type: "redis", Secret: true, Options: redisSecretOpts}},
		{"redis via kind", StoreConfig{Kind: "redis", Secret: true, Options: redisSecretOpts}},
		{"artifactory via type", StoreConfig{Type: "artifactory", Secret: true, Options: artifactorySecretOpts}},
		{"artifactory via kind", StoreConfig{Kind: "artifactory", Secret: true, Options: artifactorySecretOpts}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := StoresConfig{"store": tt.config}
			registry, err := NewStoreRegistry(&cfg)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrSecretBackendNotEncrypted)
			assert.Nil(t, registry)
		})
	}
}

// TestNewStoreRegistry_IncapableBackendWithoutSecretBuilds verifies the guard fires only on
// the `secret` flag: the same backends build fine when not marked secret.
func TestNewStoreRegistry_IncapableBackendWithoutSecretBuilds(t *testing.T) {
	tests := []struct {
		name   string
		config StoreConfig
	}{
		{"redis", StoreConfig{Type: "redis", Options: redisSecretOpts}},
		{"artifactory", StoreConfig{Type: "artifactory", Options: artifactorySecretOpts}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := StoresConfig{"store": tt.config}
			registry, err := NewStoreRegistry(&cfg)
			require.NoError(t, err)
			s, ok := registry["store"]
			assert.True(t, ok)
			assert.NotNil(t, s)
		})
	}
}

// Compile-time guard: the denylist must reference real kind constants. A rename of either
// kind will fail the build here.
var _ = []bool{secretIncapableKinds[KindRedis], secretIncapableKinds[KindArtifactory]}
