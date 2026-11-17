package signaturetest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"testing"
	"time"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	common "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	rekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"github.com/sigstore/sigstore-go/pkg/root"
	sig "github.com/sigstore/sigstore/pkg/signature"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cloudposse/atmos/pkg/perf"
)

// KeySignature is a genuine key-only Sigstore bundle together with the public half of the key.
type KeySignature struct {
	// Bundle is the raw Sigstore bundle.
	Bundle []byte
	// Hint is the key identifier embedded in the bundle.
	Hint string
	// PublicKey verifies the signature; an operator would configure it as a trusted key.
	PublicKey crypto.PublicKey
	// Logs is the transparency-log trust material for this test authority.
	Logs root.TrustedMaterial
}

// KeyHint is the identifier SignWithKey embeds in every bundle.
const KeyHint = "fixture-key"

// SignWithPublicKey generates a genuine key-only signature and Rekor evidence.
// This intentionally unsupported signing mode must never satisfy a certificate policy.
func (a *Authority) SignWithPublicKey(t *testing.T, artifact []byte) ([]byte, root.TrustedMaterial) {
	defer perf.Track(nil, "signaturetest.Authority.SignWithPublicKey")()

	t.Helper()
	signed := a.SignWithKey(t, artifact)
	verifier, err := sig.LoadVerifier(signed.PublicKey, crypto.SHA256)
	require.NoError(t, err)
	keyTrust := root.NewTrustedPublicKeyMaterial(func(string) (root.TimeConstrainedVerifier, error) {
		return root.NewExpiringKey(verifier, time.Now().Add(-time.Hour), time.Now().Add(time.Hour)), nil
	})
	return signed.Bundle, root.TrustedMaterialCollection{a.TrustedMaterial, keyTrust}
}

// SignWithKey generates a genuine key-only signature with a fresh key and returns the public key
// so tests can configure it explicitly as trusted.
func (a *Authority) SignWithKey(t *testing.T, artifact []byte) KeySignature {
	defer perf.Track(nil, "signaturetest.Authority.SignWithKey")()

	t.Helper()
	key := newKey(t)
	hash := sha256.Sum256(artifact)
	signature, err := ecdsa.SignASN1(rand.Reader, key, hash[:])
	require.NoError(t, err)
	require.True(t, ecdsa.VerifyASN1(&key.PublicKey, hash[:], signature))
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public})
	body, err := json.Marshal(map[string]any{"apiVersion": "0.0.1", "kind": "hashedrekord", "spec": map[string]any{
		"data": map[string]any{"hash": map[string]string{"algorithm": "sha256", "value": hex.EncodeToString(hash[:])}},
		"signature": map[string]any{
			"content":   base64.StdEncoding.EncodeToString(signature),
			"publicKey": map[string]string{"content": base64.StdEncoding.EncodeToString(publicPEM)},
		},
	}})
	require.NoError(t, err)
	body, err = jsoncanonicalizer.Transform(body)
	require.NoError(t, err)
	bundle := &protobundle.Bundle{
		MediaType: "application/vnd.dev.sigstore.bundle.v0.3+json",
		VerificationMaterial: &protobundle.VerificationMaterial{
			Content:     &protobundle.VerificationMaterial_PublicKey{PublicKey: &common.PublicKeyIdentifier{Hint: KeyHint}},
			TlogEntries: []*rekor.TransparencyLogEntry{a.logEntry(t, body)},
		},
		Content: &protobundle.Bundle_MessageSignature{MessageSignature: &common.MessageSignature{
			MessageDigest: &common.HashOutput{Algorithm: common.HashAlgorithm_SHA2_256, Digest: hash[:]}, Signature: signature,
		}},
	}
	raw, err := protojson.Marshal(bundle)
	require.NoError(t, err)
	return KeySignature{Bundle: raw, Hint: KeyHint, PublicKey: &key.PublicKey, Logs: a.TrustedMaterial}
}
