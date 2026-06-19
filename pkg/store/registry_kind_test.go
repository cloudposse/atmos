package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

// Note: TestNewStoreRegistry_SecretSetsSecureWrite lives in pkg/store/providers
// (registry_secret_test.go) because it asserts on the concrete *providers.SSMStore
// secret flag, which is only observable from within the providers package.

// Note: the redis/artifactory secret-capability behaviour tests
// (TestNewStoreRegistry_SecretOnIncapableBackendErrors and
// TestNewStoreRegistry_IncapableBackendWithoutSecretBuilds) live in pkg/store/providers
// (registry_secret_test.go) because building those backends requires the provider factories,
// which an internal pkg/store test cannot import without an import cycle.

// Compile-time guard: the denylist must reference real kind constants. A rename of either
// kind will fail the build here.
var _ = []bool{secretIncapableKinds[KindRedis], secretIncapableKinds[KindArtifactory]}
