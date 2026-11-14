package signature

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	sigstore "github.com/sigstore/sigstore/pkg/signature"
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

// leakingRoot is transparency-log trust that also resolves a signer key, as a loader might.
type leakingRoot struct {
	*signaturetest.Authority
	key root.TimeConstrainedVerifier
}

func (l leakingRoot) PublicKeyVerifier(string) (root.TimeConstrainedVerifier, error) {
	return l.key, nil
}

func TestKeyVerifierIgnoresSignerKeysFromTheLoader(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte("key-signed release")
	signed := authority.SignWithKey(t, artifact)
	loaded, err := sigstore.LoadVerifier(signed.PublicKey, crypto.SHA256)
	require.NoError(t, err)
	leaking := func(context.Context, bool) (root.TrustedMaterial, error) {
		return leakingRoot{Authority: authority, key: root.NewExpiringKey(loaded, time.Time{}, time.Time{})}, nil
	}
	stranger, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// The loader's key would verify the bundle, but it is not configured, so verification fails.
	verifier := KeyVerifier{Keys: []TrustedKey{{Hint: "other", PublicKey: &stranger.PublicKey}}, LoadRoot: leaking}
	_, err = verifier.Verify(t.Context(), signed.Bundle, digestOf(artifact), true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)

	// The loader's transparency-log trust is still used when the signer key is configured.
	verifier.Keys = []TrustedKey{{Hint: signed.Hint, PublicKey: signed.PublicKey}}
	result, err := verifier.Verify(t.Context(), signed.Bundle, digestOf(artifact), true)
	require.NoError(t, err)
	assert.Equal(t, signaturetest.KeyHint, result.Hint)
}

func TestKeyVerifierTriesEveryKeyThatSharesAHint(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte("key-signed release")
	signed := authority.SignWithKey(t, artifact)
	stranger, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	right := TrustedKey{Hint: signed.Hint, PublicKey: signed.PublicKey}
	wrong := TrustedKey{Hint: signed.Hint, PublicKey: &stranger.PublicKey}

	for name, keys := range map[string][]TrustedKey{"wrong key first": {wrong, right}, "right key first": {right, wrong}} {
		t.Run(name, func(t *testing.T) {
			result, err := KeyVerifier{Keys: keys, LoadRoot: logsOnly(authority)}.Verify(t.Context(), signed.Bundle, digestOf(artifact), true)
			require.NoError(t, err)
			assert.Equal(t, signaturetest.KeyHint, result.Hint)
		})
	}

	// Two non-matching keys under the hint still fail.
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	_, err = KeyVerifier{Keys: []TrustedKey{wrong, {Hint: signed.Hint, PublicKey: &other.PublicKey}}, LoadRoot: logsOnly(authority)}.Verify(t.Context(), signed.Bundle, digestOf(artifact), true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)
}
