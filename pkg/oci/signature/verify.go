// Package signature adapts the shared Sigstore verification primitives in pkg/signature to
// OCI attachments and plugin trust requirements. Verification itself lives in pkg/signature; this
// package keeps the OCI-specific transport (referrer fetch, offline evidence files) and the
// sentinel errors plugin callers already match.
package signature

import (
	"context"
	"errors"

	"github.com/sigstore/sigstore-go/pkg/root"

	errUtils "github.com/cloudposse/atmos/errors"
	sigcore "github.com/cloudposse/atmos/pkg/signature"
)

// MaxBundleSize bounds one Sigstore bundle.
const MaxBundleSize = sigcore.MaxBundleSize

// Identity binds a certificate SAN (exact or * wildcard) to an exact OIDC issuer.
type Identity = sigcore.Identity

// Result contains authenticated certificate claims, never claims from unverified metadata.
type Result = sigcore.Result

// Verifier uses a trusted root independent of the artifact and its publisher.
// LoadRoot is injectable for private trust deployments and isolated cryptographic tests.
type Verifier struct {
	LoadRoot func(context.Context, bool) (root.TrustedMaterial, error)
}

// Verify requires a certificate, SCT, transparency inclusion and authenticated signing time.
// The release digest is always supplied independently of the bundle. Failures carry the plugin
// signature sentinels that existing callers match.
//
//nolint:lintroller // Delegates to the tracked shared verifier.
func (v Verifier) Verify(ctx context.Context, raw []byte, digest string, identities []Identity, offline bool) (*Result, error) {
	result, err := sigcore.Verifier{LoadRoot: v.LoadRoot}.Verify(ctx, raw, digest, identities, offline)
	if err != nil {
		return nil, pluginError(err)
	}
	return result, nil
}

// pluginError translates shared sentinels to the plugin sentinels, preserving message text.
func pluginError(err error) error {
	var shared *sigcore.Error
	if !errors.As(err, &shared) {
		return err
	}
	legacy := errUtils.ErrPluginSignatureInvalid
	switch {
	case errors.Is(shared.Kind, errUtils.ErrSignatureConfigInvalid):
		legacy = errUtils.ErrPluginInvalidConfig
	case errors.Is(shared.Kind, errUtils.ErrSignatureOffline):
		legacy = errUtils.ErrPluginOfflineArtifact
	}
	if shared.Cause == nil {
		return legacy
	}
	return errors.Join(legacy, shared.Cause)
}
