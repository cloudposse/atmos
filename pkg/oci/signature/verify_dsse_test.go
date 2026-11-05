package signature

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
)

func TestVerifyDSSEStatementBindsOCIIndexDigest(t *testing.T) {
	authority := signaturetest.New(t)
	artifact := []byte(`{"schemaVersion":2,"manifests":[]}`)
	digest := sha256.Sum256(artifact)
	reference := "sha256:" + hex.EncodeToString(digest[:])
	identity := Identity{Identity: "builder@example.com", Issuer: "https://issuer.example.com"}
	verifier := Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }}
	raw := authority.SignStatement(t, identity.Identity, identity.Issuer, artifact)
	result, err := verifier.Verify(t.Context(), raw, reference, []Identity{identity}, true)
	require.NoError(t, err)
	require.Equal(t, &Result{Identity: identity.Identity, Issuer: identity.Issuer}, result)
	result, err = verifier.Verify(t.Context(), raw, "sha256:"+strings.Repeat("0", 64), []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.Nil(t, result)
	// The enclosing OCI artifact is not enough: changing the signed statement fails too.
	var changed map[string]any
	require.NoError(t, json.Unmarshal(raw, &changed))
	changed["dsseEnvelope"].(map[string]any)["payload"] = base64.StdEncoding.EncodeToString([]byte(`{"_type":"https://in-toto.io/Statement/v1","subject":[]}`))
	tampered, err := json.Marshal(changed)
	require.NoError(t, err)
	result, err = verifier.Verify(t.Context(), tampered, reference, []Identity{identity}, true)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.Nil(t, result)
}
