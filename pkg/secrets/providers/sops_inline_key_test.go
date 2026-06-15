package providers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/getsops/sops/v3/keyservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// newInlineAgeKeyProvider builds a SOPS provider sourcing its private key from inline
// `spec.age_key` material (with recipients pinned), so encrypt/decrypt route through the
// identity-injecting ageKeyServiceClient — exercising its Encrypt delegation and Decrypt paths.
func newInlineAgeKeyProvider(t *testing.T, ageKey, recipients string) (Provider, string) {
	t.Helper()

	dir := t.TempDir()
	file := filepath.Join(dir, "dev.enc.yaml")
	// Ensure the ambient env key is absent so the inline key is the only source.
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(dir, "absent.txt"))

	section := map[string]any{
		"dev-sops": map[string]any{
			"kind": "sops/age",
			"spec": map[string]any{
				"file":           file,
				"age_recipients": recipients,
				"age_key":        ageKey,
			},
		},
	}
	p, err := newSopsProvider(&schema.AtmosConfiguration{}, "dev-sops", section)
	require.NoError(t, err)
	return p, file
}

// TestSopsProvider_InlineAgeKey_RoundTrip proves a Set encrypts (via ageKeyServiceClient.Encrypt
// delegation to the fallback local client) and a Get decrypts using the injected inline identity.
func TestSopsProvider_InlineAgeKey_RoundTrip(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	p, file := newInlineAgeKeyProvider(t, identity.String(), identity.Recipient().String())
	coord := Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"}

	require.NoError(t, p.Set(coord, "dd-inline-secret"))
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "ENC[", "value must be encrypted at rest")
	assert.NotContains(t, string(raw), "dd-inline-secret", "plaintext must not leak to disk")

	got, err := p.Get(coord)
	require.NoError(t, err)
	assert.Equal(t, "dd-inline-secret", got)
}

// TestSopsProvider_InlineAgeKey_Invalid proves that invalid inline `spec.age_key` material is
// reported as ErrSopsAgeKey (via ageKeyErr) when the key client is built for decryption.
func TestSopsProvider_InlineAgeKey_Invalid(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	// Encrypt a real file with a valid inline key first.
	valid, file := newInlineAgeKeyProvider(t, identity.String(), identity.Recipient().String())
	coord := Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"}
	require.NoError(t, valid.Set(coord, "dd-inline-secret"))
	require.FileExists(t, file)

	// Build a second provider over the same file with garbage inline key material.
	section := map[string]any{
		"dev-sops": map[string]any{
			"kind": "sops/age",
			"spec": map[string]any{
				"file":           file,
				"age_recipients": identity.Recipient().String(),
				"age_key":        "not-a-valid-age-key",
			},
		},
	}
	bad, err := newSopsProvider(&schema.AtmosConfiguration{}, "dev-sops", section)
	require.NoError(t, err)

	_, err = bad.Get(coord)
	require.ErrorIs(t, err, ErrSopsAgeKey)
}

// TestAgeKeyServiceClient_EncryptDelegates proves the identity-injecting key service client
// delegates Encrypt to its fallback local client (it only special-cases Decrypt for age keys). It
// encrypts an age master key's data key through the client and confirms a non-empty ciphertext.
func TestAgeKeyServiceClient_EncryptDelegates(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	client, err := ageClientFromKeyMaterial(identity.String(), ageKeyErr)
	require.NoError(t, err)

	req := &keyservice.EncryptRequest{
		Key: &keyservice.Key{
			KeyType: &keyservice.Key_AgeKey{
				AgeKey: &keyservice.AgeKey{Recipient: identity.Recipient().String()},
			},
		},
		Plaintext: []byte("0123456789abcdef0123456789abcdef"),
	}

	resp, err := client.Encrypt(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Ciphertext, "Encrypt must delegate to the fallback local client")
}
