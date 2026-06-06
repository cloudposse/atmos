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
		// An explicit secret:false on a non-secret-by-default kind is left untouched.
		"ssm": StoreConfig{Type: "aws-ssm-parameter-store"},
	}
	ApplySecretDefaults(cfg)
	assert.True(t, cfg["op"].Secret, "1Password store should default to secret: true")
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
