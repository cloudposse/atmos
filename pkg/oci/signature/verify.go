// Package signature verifies Sigstore bundles against caller-selected identities.
package signature

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const MaxBundleSize = 4 << 20

// Identity binds a certificate SAN (exact or * wildcard) to an exact OIDC issuer.
type Identity struct{ Identity, Issuer string }

// Result contains authenticated certificate claims, never claims from unverified metadata.
type Result struct{ Identity, Issuer string }

// Verifier uses a trusted root independent of the artifact and its publisher.
// LoadRoot is injectable for private trust deployments and isolated cryptographic tests.
type Verifier struct {
	LoadRoot func(context.Context, bool) (root.TrustedMaterial, error)
}

// Verify requires a certificate, SCT, transparency inclusion and authenticated
// signing time. The release digest is always supplied independently of the bundle.
func (v Verifier) Verify(ctx context.Context, raw []byte, digest string, identities []Identity, offline bool) (*Result, error) {
	defer perf.Track(nil, "signature.Verifier.Verify")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hash, signed, err := parseBundle(raw, digest)
	if err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		return nil, errUtils.ErrPluginSignatureInvalid
	}
	options, err := identityOptions(identities)
	if err != nil {
		return nil, err
	}
	verifier, err := v.trustedVerifier(ctx, offline)
	if err != nil {
		return nil, err
	}
	result, err := verifier.Verify(signed, verify.NewPolicy(verify.WithArtifactDigest("sha256", hash), options...))
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result.Signature == nil || result.Signature.Certificate == nil {
		return nil, errUtils.ErrPluginSignatureInvalid
	}
	return &Result{Identity: result.Signature.Certificate.SubjectAlternativeName, Issuer: result.Signature.Certificate.Issuer}, nil
}

func parseBundle(raw []byte, digest string) ([]byte, *bundle.Bundle, error) {
	if len(raw) > MaxBundleSize || len(raw) == 0 {
		return nil, nil, errUtils.ErrPluginSignatureInvalid
	}
	hash, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
	if err != nil || !strings.HasPrefix(digest, "sha256:") || len(hash) != sha256.Size {
		return nil, nil, errUtils.ErrPluginSignatureInvalid
	}
	var signed bundle.Bundle
	if err := json.Unmarshal(raw, &signed); err != nil {
		return nil, nil, errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	return hash, &signed, nil
}

func (v Verifier) trustedVerifier(ctx context.Context, offline bool) (*verify.Verifier, error) {
	loader := v.LoadRoot
	if loader == nil {
		loader = loadPublicRoot
	}
	trusted, err := loader(ctx, offline)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	// Require a certificate identity, a witnessed signing time and inclusion in
	// a trusted transparency log. Bare-key bundles cannot establish the requested
	// OIDC identity; the key-only rejection test guards this boundary.
	verifier, err := verify.NewVerifier(trusted, verify.WithSignedCertificateTimestamps(1), verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	return verifier, nil
}

func identityOptions(identities []Identity) ([]verify.PolicyOption, error) {
	options := make([]verify.PolicyOption, 0, len(identities))
	for _, identity := range identities {
		if identity.Identity == "" || identity.Issuer == "" {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(identity.Identity), `\*`, ".*") + "$"
		match, err := verify.NewShortCertificateIdentity(identity.Issuer, "", "", pattern)
		if err != nil {
			return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
		}
		options = append(options, verify.WithCertificateIdentity(match))
	}
	return options, nil
}
