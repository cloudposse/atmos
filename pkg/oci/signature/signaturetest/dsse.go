package signaturetest

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/secure-systems-lab/go-securesystemslib/dsse"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	common "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	protodsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	rekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SignStatement signs a DSSE in-toto statement naming the artifact's SHA-256
// digest, as used by Cosign 3 OCI signatures, with real SCT and log proofs.
func (a *Authority) SignStatement(t *testing.T, identity, issuer string, artifact []byte) []byte {
	defer perf.Track(nil, "signaturetest.Authority.SignStatement")()

	t.Helper()
	key := newKey(t)
	cert := a.leaf(t, key, identity, issuer)
	artifactHash := sha256.Sum256(artifact)
	statement, err := json.Marshal(map[string]any{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []any{map[string]any{
			"name": "oci-release", "digest": map[string]string{"sha256": hex.EncodeToString(artifactHash[:])},
		}},
		"predicateType": "https://sigstore.dev/cosign/sign/v1",
		"predicate":     map[string]any{},
	})
	require.NoError(t, err)
	const payloadType = "application/vnd.in-toto+json"
	hash := sha256.Sum256(dsse.PAE(payloadType, statement))
	signed, err := ecdsa.SignASN1(rand.Reader, key, hash[:])
	require.NoError(t, err)
	envelope := &protodsse.Envelope{
		PayloadType: payloadType, Payload: statement,
		Signatures: []*protodsse.Signature{{Sig: signed}},
	}
	entry := a.logEntry(t, dsseLogBody(t, envelope, cert))
	entry.KindVersion = &rekor.KindVersion{Kind: "dsse", Version: "0.0.1"}
	bundle := &protobundle.Bundle{
		MediaType: "application/vnd.dev.sigstore.bundle.v0.3+json",
		VerificationMaterial: &protobundle.VerificationMaterial{
			Content:     &protobundle.VerificationMaterial_Certificate{Certificate: &common.X509Certificate{RawBytes: cert.Raw}},
			TlogEntries: []*rekor.TransparencyLogEntry{entry},
		},
		Content: &protobundle.Bundle_DsseEnvelope{DsseEnvelope: envelope},
	}
	raw, err := protojson.Marshal(bundle)
	require.NoError(t, err)
	return raw
}

func dsseLogBody(t *testing.T, envelope *protodsse.Envelope, cert *x509.Certificate) []byte {
	t.Helper()
	encoded, err := protojson.Marshal(envelope)
	require.NoError(t, err)
	envelopeHash := sha256.Sum256(encoded)
	payloadHash := sha256.Sum256(envelope.Payload)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	body, err := json.Marshal(map[string]any{
		"apiVersion": "0.0.1", "kind": "dsse",
		"spec": map[string]any{
			"envelopeHash": map[string]string{"algorithm": "sha256", "value": hex.EncodeToString(envelopeHash[:])},
			"payloadHash":  map[string]string{"algorithm": "sha256", "value": hex.EncodeToString(payloadHash[:])},
			"signatures": []any{map[string]string{
				"signature": base64.StdEncoding.EncodeToString(envelope.Signatures[0].Sig),
				"verifier":  base64.StdEncoding.EncodeToString(certPEM),
			}},
		},
	})
	require.NoError(t, err)
	body, err = jsoncanonicalizer.Transform(body)
	require.NoError(t, err)
	return body
}
