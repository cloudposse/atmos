package signature

import (
	"context"
	"crypto"
	"fmt"
	"time"

	"github.com/sigstore/sigstore-go/pkg/bundle"
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

// Verify checks raw against the independently supplied subject digest. When several configured keys
// share a hint, each of them is tried; the first that verifies the bundle wins.
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
	rounds, err := v.keyRounds()
	if err != nil {
		return nil, err
	}
	logs, err := v.loadLogs(ctx, offline)
	if err != nil {
		return nil, err
	}
	policy := verify.NewPolicy(verify.WithArtifactDigest("sha256", hash), verify.WithKey())
	return verifyRounds(ctx, rounds, logs, signed, policy)
}

// verifyRounds tries each key round in order and returns the hint of the first key that verifies
// the bundle. When no round verifies it, the last verification error is reported.
func verifyRounds(ctx context.Context, rounds []root.TrustedMaterial, logs root.TrustedMaterial, signed *bundle.Bundle, policy verify.PolicyBuilder) (*KeyResult, error) {
	var last error
	for _, keys := range rounds {
		verifier, err := verify.NewVerifier(root.TrustedMaterialCollection{keys, logs}, verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))
		if err != nil {
			return nil, fail(errUtils.ErrSignatureInvalid, err)
		}
		result, err := verifier.Verify(signed, policy)
		if err != nil {
			last = err
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if result.Signature == nil || result.Signature.PublicKeyID == nil {
			return nil, fail(errUtils.ErrSignatureInvalid, nil)
		}
		return &KeyResult{Hint: string(*result.Signature.PublicKeyID)}, nil
	}
	return nil, fail(errUtils.ErrSignatureInvalid, last)
}

// loadLogs loads transparency-log trust and strips any public keys it carries, so only the
// configured keys can authenticate a key-signed bundle.
func (v KeyVerifier) loadLogs(ctx context.Context, offline bool) (root.TrustedMaterial, error) {
	loader := v.LoadRoot
	if loader == nil {
		loader = LoadPublicRoot
	}
	logs, err := loader(ctx, offline)
	if err != nil {
		return nil, fail(errUtils.ErrSignatureInvalid, err)
	}
	return transparencyOnly{TrustedMaterial: logs}, nil
}

// transparencyOnly exposes transparency-log and timestamp trust but never resolves a signer key.
type transparencyOnly struct{ root.TrustedMaterial }

//nolint:lintroller // Constant rejection with no work to track.
func (transparencyOnly) PublicKeyVerifier(hint string) (root.TimeConstrainedVerifier, error) {
	return nil, fmt.Errorf("%w: key %q is not a configured trusted key", errUtils.ErrSignatureInvalid, hint)
}

// keyRounds groups the configured keys so each round holds at most one key per hint. A bundle names
// a single hint, so trying every round tries every configured key for that hint, and a key never
// overwrites another that shares its hint. Certificate authorities from other trust material cannot
// validate a key-signed bundle.
func (v KeyVerifier) keyRounds() ([]root.TrustedMaterial, error) {
	var rounds []map[string]*root.ExpiringKey
	seen := make(map[string]int, len(v.Keys))
	for _, key := range v.Keys {
		if key.Hint == "" || key.PublicKey == nil {
			return nil, fail(errUtils.ErrSignatureConfigInvalid, nil)
		}
		loaded, err := sigstore.LoadVerifier(key.PublicKey, crypto.SHA256)
		if err != nil {
			return nil, fail(errUtils.ErrSignatureConfigInvalid, err)
		}
		round := seen[key.Hint]
		seen[key.Hint]++
		if round == len(rounds) {
			rounds = append(rounds, make(map[string]*root.ExpiringKey))
		}
		rounds[round][key.Hint] = root.NewExpiringKey(loaded, time.Time{}, time.Time{})
	}
	materials := make([]root.TrustedMaterial, 0, len(rounds))
	for _, keys := range rounds {
		materials = append(materials, root.NewTrustedPublicKeyMaterialFromMapping(keys))
	}
	return materials, nil
}
