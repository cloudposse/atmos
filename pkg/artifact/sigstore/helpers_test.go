package sigstore

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/signature/signaturetest"
)

const (
	testIdentity = "https://github.com/acme/images/.github/workflows/release.yml@refs/heads/main"
	testIssuer   = "https://token.actions.githubusercontent.com"
	keyHint      = signaturetest.KeyHint
)

// fixture is one signed-subject test setup.
type fixture struct {
	t       *testing.T
	auth    *signaturetest.Authority
	data    []byte
	subject artifact.SignatureSubject
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	data := []byte(`{"name":"team/app","version":"1.0.0"}`)
	return &fixture{
		t:    t,
		auth: signaturetest.New(t),
		data: data,
		subject: artifact.SignatureSubject{
			Kind: artifact.SubjectRecord, Digest: digest.FromBytes(data),
			MediaType: artifact.MediaTypeConfig, Size: int64(len(data)),
		},
	}
}

func (f *fixture) loader(material root.TrustedMaterial) RootLoader {
	return func(context.Context, bool) (root.TrustedMaterial, error) { return material, nil }
}

func (f *fixture) verifier(opts ...VerifierOption) *Verifier {
	return NewVerifier(append([]VerifierOption{WithRootLoader(f.loader(f.auth))}, opts...)...)
}

func (f *fixture) blob(format artifact.EvidenceFormat, data []byte) artifact.EvidenceBlob {
	return artifact.EvidenceBlob{
		Evidence: artifact.SignatureEvidence{
			Subject: f.subject, Format: format,
			Descriptor: ocispec.Descriptor{MediaType: "application/vnd.dev.sigstore.bundle.v0.3+json", Digest: digest.FromBytes(data), Size: int64(len(data))},
		},
		Data: data,
	}
}

// keyless signs the subject bytes directly (the raw-digest compatibility form).
func (f *fixture) keyless(identity, issuer string) artifact.EvidenceBlob {
	return f.blob(artifact.EvidenceSigstoreBundle, f.auth.Sign(f.t, identity, issuer, f.data))
}

// keylessStatement signs the subject statement through the Signer (the statement form).
func (f *fixture) keylessStatement(identity, issuer string) artifact.EvidenceBlob {
	signer := NewSigner(&authoritySigner{t: f.t, auth: f.auth, identity: identity, issuer: issuer})
	payload, err := signer.Sign(context.Background(), f.subject)
	require.NoError(f.t, err)
	return f.blob(payload.Format, payload.Data)
}

// keyed signs the subject bytes with a fresh key and returns the evidence and the key.
func (f *fixture) keyed() (artifact.EvidenceBlob, signaturetest.KeySignature) {
	signed := f.auth.SignWithKey(f.t, f.data)
	return f.blob(artifact.EvidencePublicKeyBundle, signed.Bundle), signed
}

func keyPolicy(refs ...string) artifact.VerificationPolicy {
	return artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{{Keys: refs}}}
}

func identityPolicy(identity, issuer string) artifact.VerificationPolicy {
	return artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{{Identities: []artifact.TrustedIdentity{{Identity: identity, Issuer: issuer}}}}}
}

// authoritySigner is a signature.BlobSigner backed by the isolated test authority.
type authoritySigner struct {
	t        *testing.T
	auth     *signaturetest.Authority
	identity string
	issuer   string
	// replace, when set, is signed instead of the requested bytes.
	replace []byte
}

func (a *authoritySigner) Sign(_ context.Context, data []byte) ([]byte, error) {
	if a.replace != nil {
		data = a.replace
	}
	return a.auth.Sign(a.t, a.identity, a.issuer, data), nil
}

func codes(result *artifact.VerificationResult) []string {
	out := make([]string, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		out = append(out, diagnostic.Code)
	}
	return out
}

func pemFor(t *testing.T, key signaturetest.KeySignature) (encoded string, fingerprint string) {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key.PublicKey)
	require.NoError(t, err)
	sum := sha256.Sum256(der)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), hex.EncodeToString(sum[:])
}
