package store

import (
	"strings"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
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

func TestStoreConfigDecode_MapstructureKind(t *testing.T) {
	var cfg StoreConfig
	err := mapstructure.Decode(map[string]any{
		"kind":     "aws/ssm",
		"identity": "local-aws",
		"secret":   true,
		"options": map[string]any{
			"region": "us-east-1",
		},
	}, &cfg)
	require.NoError(t, err)

	assert.Equal(t, "aws/ssm", cfg.Kind)
	assert.Equal(t, "local-aws", cfg.Identity)
	assert.True(t, cfg.Secret)
	assert.Equal(t, "us-east-1", cfg.Options["region"])
}

func TestStoreConfigDecode_ViperKind(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(`
stores:
  config/ssm:
    kind: aws/ssm
    identity: local-aws
    options:
      region: us-east-1
`)))

	var cfg struct {
		Stores StoresConfig `mapstructure:"stores"`
	}
	require.NoError(t, v.Unmarshal(&cfg))

	require.Contains(t, cfg.Stores, "config/ssm")
	assert.Equal(t, "aws/ssm", cfg.Stores["config/ssm"].Kind)
	assert.Equal(t, "local-aws", cfg.Stores["config/ssm"].Identity)
	assert.Equal(t, "us-east-1", cfg.Stores["config/ssm"].Options["region"])
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
