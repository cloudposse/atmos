package signature

import (
	"context"
	"crypto"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
	sigstore "github.com/sigstore/sigstore/pkg/signature"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TrustedKey is a public key an operator explicitly configured as a trusted signer.
type TrustedKey struct {
	// Hint is the key identifier a bundle carries in its verification material.
	Hint string
	// PublicKey is the verification key.
	PublicKey crypto.PublicKey
}

// KeyResult names the configured key that authenticated a bundle. It carries no certificate claims.
type KeyResult struct{ Hint string }

// KeyVerifier verifies Sigstore bundles signed with an explicitly configured public key.
//
// It is a separate type from Verifier on purpose: its result is a key hint, never an OIDC
// identity or issuer, so a public-key signature cannot be mistaken for, or satisfy, an identity
// requirement. It requires the bundle to be signed with a key (certificate bundles are rejected),
// inclusion in a trusted transparency log, and an authenticated signing time.
type KeyVerifier struct {
	// Keys are the only keys that can authenticate a bundle. An empty set fails closed.
	Keys []TrustedKey
	// LoadRoot supplies transparency-log trust. It defaults to the Sigstore public-good root.
	LoadRoot func(context.Context, bool) (root.TrustedMaterial, error)
}

// Verify checks raw against the independently supplied subject digest.
func (v KeyVerifier) Verify(ctx context.Context, raw []byte, digest string, offline bool) (*KeyResult, error) {
	defer perf.Track(nil, "signature.KeyVerifier.Verify")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(v.Keys) == 0 {
		return nil, fail(errUtils.ErrSignatureConfigInvalid, nil)
	}
	hash, signed, err := parseBundle(raw, digest)
	if err != nil {
		return nil, err
	}
	verifier, err := v.trustedVerifier(ctx, offline)
	if err != nil {
		return nil, err
	}
	result, err := verifier.Verify(signed, verify.NewPolicy(verify.WithArtifactDigest("sha256", hash), verify.WithKey()))
	if err != nil {
		return nil, fail(errUtils.ErrSignatureInvalid, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result.Signature == nil || result.Signature.PublicKeyID == nil {
		return nil, fail(errUtils.ErrSignatureInvalid, nil)
	}
	return &KeyResult{Hint: string(*result.Signature.PublicKeyID)}, nil
}

// trustedVerifier combines the configured keys with transparency-log trust.
func (v KeyVerifier) trustedVerifier(ctx context.Context, offline bool) (*verify.Verifier, error) {
	keys, err := v.keyMaterial()
	if err != nil {
		return nil, err
	}
	loader := v.LoadRoot
	if loader == nil {
		loader = LoadPublicRoot
	}
	logs, err := loader(ctx, offline)
	if err != nil {
		return nil, fail(errUtils.ErrSignatureInvalid, err)
	}
	verifier, err := verify.NewVerifier(root.TrustedMaterialCollection{keys, logs}, verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))
	if err != nil {
		return nil, fail(errUtils.ErrSignatureInvalid, err)
	}
	return verifier, nil
}

// keyMaterial exposes only the configured keys. Certificate authorities from other trust
// material cannot validate a key-signed bundle.
func (v KeyVerifier) keyMaterial() (root.TrustedMaterial, error) {
	verifiers := make(map[string]*root.ExpiringKey, len(v.Keys))
	for _, key := range v.Keys {
		if key.Hint == "" || key.PublicKey == nil {
			return nil, fail(errUtils.ErrSignatureConfigInvalid, nil)
		}
		loaded, err := sigstore.LoadVerifier(key.PublicKey, crypto.SHA256)
		if err != nil {
			return nil, fail(errUtils.ErrSignatureConfigInvalid, err)
		}
		verifiers[key.Hint] = root.NewExpiringKey(loaded, time.Time{}, time.Time{})
	}
	return root.NewTrustedPublicKeyMaterialFromMapping(verifiers), nil
}
