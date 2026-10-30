package signature

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
)

func TestVerifyRejectsGenuinePublicKeyOnlySignature(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte("OCI release index signed without a certificate")
	digest := sha256.Sum256(artifact)
	raw, trusted := authority.SignWithPublicKey(t, artifact)
	verifier := Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) { return trusted, nil }}
	result, err := verifier.Verify(t.Context(), raw, "sha256:"+hex.EncodeToString(digest[:]), []Identity{{
		Identity: "builder@example.com", Issuer: "https://issuer.example.com",
	}}, true)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.ErrorContains(t, err, "SCTs required but bundle is signed with a public key")
	require.Nil(t, result)
}
