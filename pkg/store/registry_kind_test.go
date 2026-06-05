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
