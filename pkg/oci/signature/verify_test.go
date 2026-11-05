package signature

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
)

func TestVerifyRealSigstoreEvidence(t *testing.T) {
	authority := signaturetest.New(t)
	verifier := Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }}
	artifact := []byte("immutable release index")
	digest := sha256.Sum256(artifact)
	reference := "sha256:" + hex.EncodeToString(digest[:])
	identity := Identity{Identity: "https://github.com/acme/plugin/.github/workflows/release.yml@refs/tags/v1.0.0", Issuer: "https://token.actions.githubusercontent.com"}
	raw := authority.Sign(t, identity.Identity, identity.Issuer, artifact)
	result, err := verifier.Verify(t.Context(), raw, reference, []Identity{identity}, true)
	require.NoError(t, err)
	require.Equal(t, &Result{Identity: identity.Identity, Issuer: identity.Issuer}, result)
	wildcard := identity
	wildcard.Identity = "https://github.com/acme/*"
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
		{"anchored identity", reference, Identity{Identity: "github.com/acme/*", Issuer: identity.Issuer}, raw},
		{"malformed", reference, identity, []byte("{}")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := verifier.Verify(t.Context(), tc.raw, tc.digest, []Identity{tc.identity}, true)
			require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
		})
	}
	var changed map[string]any
	require.NoError(t, json.Unmarshal(raw, &changed))
	changed["verificationMaterial"].(map[string]any)["tlogEntries"] = []any{}
	tampered, err := json.Marshal(changed)
	require.NoError(t, err)
	_, err = verifier.Verify(t.Context(), tampered, reference, []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	other := signaturetest.New(t)
	verifier.LoadRoot = func(context.Context, bool) (root.TrustedMaterial, error) { return other, nil }
	_, err = verifier.Verify(t.Context(), raw, reference, []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
}
