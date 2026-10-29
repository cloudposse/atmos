package signature

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
)

func signedFixture(t *testing.T) (Verifier, []byte, string, Identity) {
	t.Helper()
	authority := signaturetest.New(t)
	artifact := []byte("signed OCI release index")
	digest := sha256.Sum256(artifact)
	identity := Identity{Identity: "builder+release@example.com", Issuer: "https://issuer.example.com"}
	return Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) {
			return authority, nil
		}}, authority.Sign(t, identity.Identity, identity.Issuer, artifact),
		"sha256:" + hex.EncodeToString(digest[:]), identity
}

func TestVerifyRejectsInvalidPolicyAndInputBeforeLoadingTrust(t *testing.T) {
	_, raw, digest, identity := signedFixture(t)
	verifier := Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) {
		t.Fatal("invalid inputs must fail before loading trust")
		return nil, nil
	}}
	tests := []struct {
		name       string
		raw        []byte
		digest     string
		identities []Identity
		want       error
	}{
		{"empty bundle", nil, digest, []Identity{identity}, errUtils.ErrPluginSignatureInvalid},
		{"oversized bundle", make([]byte, MaxBundleSize+1), digest, []Identity{identity}, errUtils.ErrPluginSignatureInvalid},
		{"no publishers", raw, digest, nil, errUtils.ErrPluginSignatureInvalid},
		{"unsupported algorithm", raw, strings.Replace(digest, "sha256:", "sha512:", 1), []Identity{identity}, errUtils.ErrPluginSignatureInvalid},
		{"invalid digest hex", raw, "sha256:" + strings.Repeat("g", 64), []Identity{identity}, errUtils.ErrPluginSignatureInvalid},
		{"short digest", raw, "sha256:abcd", []Identity{identity}, errUtils.ErrPluginSignatureInvalid},
		{"missing issuer", raw, digest, []Identity{{Identity: identity.Identity}}, errUtils.ErrPluginInvalidConfig},
		{"missing identity", raw, digest, []Identity{{Issuer: identity.Issuer}}, errUtils.ErrPluginInvalidConfig},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := verifier.Verify(t.Context(), tc.raw, tc.digest, tc.identities, true)
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result)
		})
	}
}

func TestVerifyCancellationAndRootFailure(t *testing.T) {
	verifier, raw, digest, identity := signedFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := verifier.Verify(ctx, raw, digest, []Identity{identity}, true)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	original := verifier.LoadRoot
	verifier.LoadRoot = func(ctx context.Context, offline bool) (root.TrustedMaterial, error) {
		require.True(t, offline, "verification must preserve the caller's offline policy")
		return nil, os.ErrPermission
	}
	result, err = verifier.Verify(t.Context(), raw, digest, []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.ErrorIs(t, err, os.ErrPermission)
	require.Nil(t, result)
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	verifier.LoadRoot = func(ctx context.Context, offline bool) (root.TrustedMaterial, error) {
		cancel()
		return original(ctx, offline)
	}
	result, err = verifier.Verify(ctx, raw, digest, []Identity{identity}, true)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}

func TestVerifyDefaultTrustFailsClosedWithoutOfflineCache(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	_, raw, digest, identity := signedFixture(t)
	result, err := (Verifier{}).Verify(t.Context(), raw, digest, []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.Nil(t, result)
}

func TestVerifyIdentityRulesEscapeRegexAndAllowAnyConfiguredPublisher(t *testing.T) {
	verifier, raw, digest, identity := signedFixture(t)
	allowed := []Identity{{Identity: "other@example.com", Issuer: identity.Issuer}, identity}
	result, err := verifier.Verify(t.Context(), raw, digest, allowed, true)
	require.NoError(t, err)
	require.Equal(t, &Result{Identity: identity.Identity, Issuer: identity.Issuer}, result)
	// A plus is literal, not the regex "one or more" operator; dots are literal too.
	allowed = []Identity{{Identity: "builder.release@example.com", Issuer: identity.Issuer}}
	result, err = verifier.Verify(t.Context(), raw, digest, allowed, true)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.Nil(t, result)
}

func TestVerifyRejectsTamperedSignatureAndTransparencyEvidence(t *testing.T) {
	verifier, raw, digest, identity := signedFixture(t)
	for _, field := range []string{"signature", "checkpoint", "integrated time", "inclusion proof", "certificate"} {
		t.Run(field, func(t *testing.T) {
			var changed map[string]any
			require.NoError(t, json.Unmarshal(raw, &changed))
			material := changed["verificationMaterial"].(map[string]any)
			entry := material["tlogEntries"].([]any)[0].(map[string]any)
			switch field {
			case "signature":
				changed["messageSignature"].(map[string]any)["signature"] = "AAAA"
			case "checkpoint":
				entry["inclusionProof"].(map[string]any)["checkpoint"] = map[string]any{"envelope": "forged"}
			case "integrated time":
				entry["integratedTime"] = "1"
			case "inclusion proof":
				delete(entry, "inclusionProof")
			case "certificate":
				material["certificate"].(map[string]any)["rawBytes"] = "AAAA"
			}
			tampered, err := json.Marshal(changed)
			require.NoError(t, err)
			result, err := verifier.Verify(t.Context(), tampered, digest, []Identity{identity}, true)
			require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
			require.Nil(t, result)
		})
	}
}

func TestVerifyRequiresTrustedCertificateTransparencyLog(t *testing.T) {
	verifier, raw, digest, identity := signedFixture(t)
	trusted, err := verifier.LoadRoot(t.Context(), true)
	require.NoError(t, err)
	withoutCT, err := root.NewTrustedRoot(root.TrustedRootMediaType01, trusted.FulcioCertificateAuthorities(), nil, nil, trusted.RekorLogs())
	require.NoError(t, err)
	verifier.LoadRoot = func(context.Context, bool) (root.TrustedMaterial, error) { return withoutCT, nil }
	result, err := verifier.Verify(t.Context(), raw, digest, []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.ErrorContains(t, err, "signed certificate timestamp")
	require.Nil(t, result)
}
