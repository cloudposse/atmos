package signature

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signature/signaturetest"
)

func digestOf(artifact []byte) string {
	sum := sha256.Sum256(artifact)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestVerifyRealSigstoreEvidence(t *testing.T) {
	authority := signaturetest.New(t)
	verifier := Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }}
	artifact := []byte("immutable subject")
	reference := digestOf(artifact)
	identity := Identity{Identity: "https://github.com/acme/images/.github/workflows/release.yml@refs/heads/main", Issuer: "https://token.actions.githubusercontent.com"}
	raw := authority.Sign(t, identity.Identity, identity.Issuer, artifact)

	result, err := verifier.Verify(t.Context(), raw, reference, []Identity{identity}, true)
	require.NoError(t, err)
	assert.Equal(t, &Result{Identity: identity.Identity, Issuer: identity.Issuer}, result)

	wildcard := Identity{Identity: "https://github.com/acme/*", Issuer: identity.Issuer}
	_, err = verifier.Verify(t.Context(), raw, reference, []Identity{wildcard}, true)
	require.NoError(t, err)

	for _, tc := range []struct {
		name, digest string
		identity     Identity
		raw          []byte
	}{
		{"wrong digest", "sha256:" + strings.Repeat("a", 64), identity, raw},
		{"wrong issuer", reference, Identity{Identity: identity.Identity, Issuer: "https://wrong.example"}, raw},
		{"wrong identity", reference, Identity{Identity: "https://github.com/other/*", Issuer: identity.Issuer}, raw},
		{"identity is anchored", reference, Identity{Identity: "github.com/acme/*", Issuer: identity.Issuer}, raw},
		{"malformed bundle", reference, identity, []byte("{}")},
		{"empty bundle", reference, identity, nil},
		{"oversized bundle", reference, identity, make([]byte, MaxBundleSize+1)},
		{"non-sha256 digest", "sha512:" + strings.Repeat("a", 128), identity, raw},
		{"short digest", "sha256:abcd", identity, raw},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := verifier.Verify(t.Context(), tc.raw, tc.digest, []Identity{tc.identity}, true)
			require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)
		})
	}

	var changed map[string]any
	require.NoError(t, json.Unmarshal(raw, &changed))
	changed["verificationMaterial"].(map[string]any)["tlogEntries"] = []any{}
	tampered, err := json.Marshal(changed)
	require.NoError(t, err)
	_, err = verifier.Verify(t.Context(), tampered, reference, []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid, "a bundle without transparency inclusion is rejected")

	other := signaturetest.New(t)
	verifier.LoadRoot = func(context.Context, bool) (root.TrustedMaterial, error) { return other, nil }
	_, err = verifier.Verify(t.Context(), raw, reference, []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid, "a root that did not issue the certificate cannot verify it")
}

func TestVerifyRejectsUnusablePolicyAndTrust(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte("subject")
	identity := Identity{Identity: "builder@example.com", Issuer: "https://issuer.example.com"}
	raw := authority.Sign(t, identity.Identity, identity.Issuer, artifact)
	trusted := func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }

	_, err := Verifier{LoadRoot: trusted}.Verify(t.Context(), raw, digestOf(artifact), nil, true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid, "verification without a required identity is refused")

	for _, bad := range []Identity{{Issuer: identity.Issuer}, {Identity: identity.Identity}} {
		_, err = Verifier{LoadRoot: trusted}.Verify(t.Context(), raw, digestOf(artifact), []Identity{bad}, true)
		require.ErrorIs(t, err, errUtils.ErrSignatureConfigInvalid)
		require.NotErrorIs(t, err, errUtils.ErrSignatureInvalid)
	}

	failing := func(context.Context, bool) (root.TrustedMaterial, error) { return nil, errUtils.ErrSignatureOffline }
	_, err = Verifier{LoadRoot: failing}.Verify(t.Context(), raw, digestOf(artifact), []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)
	require.ErrorIs(t, err, errUtils.ErrSignatureOffline, "the trust failure stays inspectable")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = Verifier{LoadRoot: trusted}.Verify(ctx, raw, digestOf(artifact), []Identity{identity}, true)
	require.ErrorIs(t, err, context.Canceled)
}

func TestVerifyRejectsPublicKeyOnlySignatureForIdentityRequirements(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte("release signed without a certificate")
	raw, trusted := authority.SignWithPublicKey(t, artifact)
	verifier := Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) { return trusted, nil }}
	result, err := verifier.Verify(t.Context(), raw, digestOf(artifact), []Identity{{
		Identity: "builder@example.com", Issuer: "https://issuer.example.com",
	}}, true)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)
	require.ErrorContains(t, err, "SCTs required but bundle is signed with a public key")
	require.Nil(t, result)
}

func TestErrorExposesSentinelAndCause(t *testing.T) {
	cause := context.DeadlineExceeded
	err := fail(errUtils.ErrSignatureInvalid, cause)
	require.ErrorIs(t, err, errUtils.ErrSignatureInvalid)
	require.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), errUtils.ErrSignatureInvalid.Error())
	assert.Contains(t, err.Error(), cause.Error())
	assert.Equal(t, errUtils.ErrSignatureInvalid.Error(), fail(errUtils.ErrSignatureInvalid, nil).Error())
}
