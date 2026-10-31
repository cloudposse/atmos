// Package signaturetest creates real certificates, signatures and transparency
// proofs without contacting a public identity provider or transparency log.
package signaturetest

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	ct "github.com/google/certificate-transparency-go"
	"github.com/google/certificate-transparency-go/tls"
	ctx509 "github.com/google/certificate-transparency-go/x509"
	"github.com/google/certificate-transparency-go/x509util"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	common "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	rekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"github.com/sigstore/rekor/pkg/util"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tlog"
	sig "github.com/sigstore/sigstore/pkg/signature"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cloudposse/atmos/pkg/perf"
)

var fulcioIssuerOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 1}

// Authority owns isolated CA, CT and Rekor keys and exposes only public trust material.
type Authority struct {
	root.TrustedMaterial
	certificate        *x509.Certificate
	key, ctKey, logKey *ecdsa.PrivateKey
}

// New creates a fresh trust domain for one test.
func New(t *testing.T) *Authority {
	defer perf.Track(nil, "signaturetest.New")()

	t.Helper()
	a := &Authority{key: newKey(t), ctKey: newKey(t), logKey: newKey(t)}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test Fulcio"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	a.certificate = issue(t, template, template, &a.key.PublicKey, a.key)
	// Expose the isolated root CA, CT verifier, and Rekor verifier through the
	// library's trust material. Timestamping authorities are empty: bundles use
	// authenticated Rekor timestamps.
	trusted, err := root.NewTrustedRoot(root.TrustedRootMediaType01,
		[]root.CertificateAuthority{&root.FulcioCertificateAuthority{Root: a.certificate}},
		logs(a.ctKey), nil, logs(a.logKey))
	require.NoError(t, err)
	a.TrustedMaterial = trusted
	return a
}

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

func issue(t *testing.T, template, parent *x509.Certificate, pub crypto.PublicKey, key crypto.Signer) *x509.Certificate {
	t.Helper()
	raw, err := x509.CreateCertificate(rand.Reader, template, parent, pub, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(raw)
	require.NoError(t, err)
	return cert
}

func keyID(t *testing.T, key crypto.PublicKey) []byte {
	t.Helper()
	raw, err := x509.MarshalPKIXPublicKey(key)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	return sum[:]
}

func logs(key *ecdsa.PrivateKey) map[string]*root.TransparencyLog {
	raw, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	id := sha256.Sum256(raw)
	return map[string]*root.TransparencyLog{hex.EncodeToString(id[:]): {BaseURL: "https://rekor.test", ValidityPeriodStart: time.Now().Add(-time.Hour), ValidityPeriodEnd: time.Now().Add(time.Hour), ID: id[:], HashFunc: crypto.SHA256, SignatureHashFunc: crypto.SHA256, PublicKey: &key.PublicKey}}
}

// Sign returns a v0.3 bundle signing the artifact bytes, including a genuine SCT,
// signed Rekor entry and single-leaf inclusion proof/checkpoint.
func (a *Authority) Sign(t *testing.T, identity, issuer string, artifact []byte) []byte {
	defer perf.Track(nil, "signaturetest.Authority.Sign")()

	t.Helper()
	key := newKey(t)
	cert := a.leaf(t, key, identity, issuer)
	hash := sha256.Sum256(artifact)
	signature, err := ecdsa.SignASN1(rand.Reader, key, hash[:])
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	body, err := json.Marshal(map[string]any{"apiVersion": "0.0.1", "kind": "hashedrekord", "spec": map[string]any{"data": map[string]any{"hash": map[string]any{"algorithm": "sha256", "value": hex.EncodeToString(hash[:])}}, "signature": map[string]any{"content": base64.StdEncoding.EncodeToString(signature), "publicKey": map[string]any{"content": base64.StdEncoding.EncodeToString(certPEM)}}}})
	require.NoError(t, err)
	body, err = jsoncanonicalizer.Transform(body)
	require.NoError(t, err)
	entry := a.logEntry(t, body)
	bundle := &protobundle.Bundle{MediaType: "application/vnd.dev.sigstore.bundle.v0.3+json", VerificationMaterial: &protobundle.VerificationMaterial{Content: &protobundle.VerificationMaterial_Certificate{Certificate: &common.X509Certificate{RawBytes: cert.Raw}}, TlogEntries: []*rekor.TransparencyLogEntry{entry}}, Content: &protobundle.Bundle_MessageSignature{MessageSignature: &common.MessageSignature{MessageDigest: &common.HashOutput{Algorithm: common.HashAlgorithm_SHA2_256, Digest: hash[:]}, Signature: signature}}}
	raw, err := protojson.Marshal(bundle)
	require.NoError(t, err)
	return raw
}

func (a *Authority) leaf(t *testing.T, key *ecdsa.PrivateKey, identity, issuer string) *x509.Certificate {
	t.Helper()
	template := &x509.Certificate{SerialNumber: big.NewInt(2), EmailAddresses: []string{identity}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}, ExtraExtensions: []pkix.Extension{{Id: fulcioIssuerOID, Value: []byte(issuer)}}}
	precert := issue(t, template, a.certificate, &key.PublicKey, a.key)
	id := keyID(t, &a.ctKey.PublicKey)
	sct := ct.SignedCertificateTimestamp{SCTVersion: ct.V1, Timestamp: uint64(time.Now().UnixMilli())}
	copy(sct.LogID.KeyID[:], id)
	entry := ct.LogEntry{Leaf: ct.MerkleTreeLeaf{Version: ct.V1, LeafType: ct.TimestampedEntryLeafType, TimestampedEntry: &ct.TimestampedEntry{Timestamp: sct.Timestamp, EntryType: ct.PrecertLogEntryType, PrecertEntry: &ct.PreCert{IssuerKeyHash: sha256.Sum256(a.certificate.RawSubjectPublicKeyInfo), TBSCertificate: precert.RawTBSCertificate}}}}
	input, err := ct.SerializeSCTSignatureInput(sct, entry)
	require.NoError(t, err)
	digest := sha256.Sum256(input)
	signed, err := ecdsa.SignASN1(rand.Reader, a.ctKey, digest[:])
	require.NoError(t, err)
	sct.Signature = ct.DigitallySigned{Algorithm: tls.SignatureAndHashAlgorithm{Hash: tls.SHA256, Signature: tls.ECDSA}, Signature: signed}
	list, err := x509util.MarshalSCTsIntoSCTList([]*ct.SignedCertificateTimestamp{&sct})
	require.NoError(t, err)
	encoded, err := tls.Marshal(*list)
	require.NoError(t, err)
	encoded, err = asn1.Marshal(encoded)
	require.NoError(t, err)
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{Id: asn1.ObjectIdentifier(ctx509.OIDExtensionCTSCT), Value: encoded})
	return issue(t, template, a.certificate, &key.PublicKey, a.key)
}

func (a *Authority) logEntry(t *testing.T, body []byte) *rekor.TransparencyLogEntry {
	t.Helper()
	id := keyID(t, &a.logKey.PublicKey)
	now := time.Now().Unix()
	payload, err := json.Marshal(tlog.RekorPayload{Body: base64.StdEncoding.EncodeToString(body), IntegratedTime: now, LogIndex: 0, LogID: hex.EncodeToString(id)})
	require.NoError(t, err)
	payload, err = jsoncanonicalizer.Transform(payload)
	require.NoError(t, err)
	hash := sha256.Sum256(payload)
	set, err := ecdsa.SignASN1(rand.Reader, a.logKey, hash[:])
	require.NoError(t, err)
	rootHash := sha256.Sum256(append([]byte{0}, body...))
	signer, err := sig.LoadECDSASignerVerifier(a.logKey, crypto.SHA256)
	require.NoError(t, err)
	checkpoint, err := util.CreateAndSignCheckpoint(context.Background(), "rekor.test", 0, 1, rootHash[:], signer)
	require.NoError(t, err)
	return &rekor.TransparencyLogEntry{LogIndex: 0, LogId: &common.LogId{KeyId: id}, IntegratedTime: now, KindVersion: &rekor.KindVersion{Kind: "hashedrekord", Version: "0.0.1"}, CanonicalizedBody: body, InclusionPromise: &rekor.InclusionPromise{SignedEntryTimestamp: set}, InclusionProof: &rekor.InclusionProof{LogIndex: 0, RootHash: rootHash[:], TreeSize: 1, Checkpoint: &rekor.Checkpoint{Envelope: string(checkpoint)}}}
}
