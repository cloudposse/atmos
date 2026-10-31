package signature

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signature/signaturetest"
)

func logsOnly(authority *signaturetest.Authority) func(context.Context, bool) (root.TrustedMaterial, error) {
	return func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }
}

func TestKeyVerifierAcceptsOnlyConfiguredKeys(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte("key-signed release")
	signed := authority.SignWithKey(t, artifact)
	configured := TrustedKey{Hint: signed.Hint, PublicKey: signed.PublicKey}
	verifier := KeyVerifier{Keys: []TrustedKey{configured}, LoadRoot: logsOnly(authority)}

	result, err := verifier.Verify(t.Context(), signed.Bundle, digestOf(artifact), true)
	require.NoError(t, err)
	assert.Equal(t, &KeyResult{Hint: signaturetest.KeyHint}, result)

	stranger, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		verifier KeyVerifier
		digest   string
		raw      []byte
		kind     error
	}{
		{"different key under the same hint", KeyVerifier{Keys: []TrustedKey{{Hint: signed.Hint, PublicKey: &stranger.PublicKey}}, LoadRoot: logsOnly(authority)}, digestOf(artifact), signed.Bundle, errUtils.ErrSignatureInvalid},
		{"hint not configured", KeyVerifier{Keys: []TrustedKey{{Hint: "other", PublicKey: signed.PublicKey}}, LoadRoot: logsOnly(authority)}, digestOf(artifact), signed.Bundle, errUtils.ErrSignatureInvalid},
		{"wrong subject digest", verifier, "sha256:" + strings.Repeat("b", 64), signed.Bundle, errUtils.ErrSignatureInvalid},
		{"malformed bundle", verifier, digestOf(artifact), []byte("{}"), errUtils.ErrSignatureInvalid},
		{"no configured keys", KeyVerifier{LoadRoot: logsOnly(authority)}, digestOf(artifact), signed.Bundle, errUtils.ErrSignatureConfigInvalid},
		{"key without hint", KeyVerifier{Keys: []TrustedKey{{PublicKey: signed.PublicKey}}, LoadRoot: logsOnly(authority)}, digestOf(artifact), signed.Bundle, errUtils.ErrSignatureConfigInvalid},
		{"key without material", KeyVerifier{Keys: []TrustedKey{{Hint: signed.Hint}}, LoadRoot: logsOnly(authority)}, digestOf(artifact), signed.Bundle, errUtils.ErrSignatureConfigInvalid},
		{"unsupported key type", KeyVerifier{Keys: []TrustedKey{{Hint: signed.Hint, PublicKey: "not a key"}}, LoadRoot: logsOnly(authority)}, digestOf(artifact), signed.Bundle, errUtils.ErrSignatureConfigInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.verifier.Verify(t.Context(), tc.raw, tc.digest, true)
			require.ErrorIs(t, err, tc.kind)
			require.Nil(t, result)
		})
	}

	failing := KeyVerifier{Keys: []TrustedKey{configured}, LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) {
		return nil, errUtils.ErrSignatureOffline
	}}
	_, err = failing.Verify(t.Context(), signed.Bundle, digestOf(artifact), true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = verifier.Verify(ctx, signed.Bundle, digestOf(artifact), true)
	require.ErrorIs(t, err, context.Canceled)
}

// TestKeyAndIdentityPathsNeverSubstituteForEachOther is the trust-boundary guarantee: neither
// verifier accepts the other's evidence, and the key path never reports an OIDC identity.
func TestKeyAndIdentityPathsNeverSubstituteForEachOther(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte("subject")
	identity := Identity{Identity: "builder@example.com", Issuer: "https://issuer.example.com"}
	certificateBundle := authority.Sign(t, identity.Identity, identity.Issuer, artifact)
	keySigned := authority.SignWithKey(t, artifact)
	trust := logsOnly(authority)

	// A public-key signature cannot satisfy an identity requirement, even when the key is trusted.
	_, err := Verifier{LoadRoot: trust}.Verify(t.Context(), keySigned.Bundle, digestOf(artifact), []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)

	// A certificate-signed bundle cannot be accepted by the key path, whatever keys are configured.
	keys := KeyVerifier{Keys: []TrustedKey{{Hint: keySigned.Hint, PublicKey: keySigned.PublicKey}}, LoadRoot: trust}
	_, err = keys.Verify(t.Context(), certificateBundle, digestOf(artifact), true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)
}
