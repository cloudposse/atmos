package sigstore

import (
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/signature"
)

// BundleMediaTypePrefix is the prefix of every Sigstore bundle media type.
const BundleMediaTypePrefix = "application/vnd.dev.sigstore.bundle."

// defaultBundleMediaType is used when a bundle does not declare its own media type.
const defaultBundleMediaType = BundleMediaTypePrefix + "v0.3+json"

// statementVersion is the version of the subject statement schema.
const statementVersion = 1

// subjectStatement is the document the Signer signs. Field order is alphabetical by JSON key, so
// serialization is deterministic.
type subjectStatement struct {
	Digest    string `json:"digest"`
	Kind      string `json:"kind"`
	MediaType string `json:"media_type"`
	Size      int64  `json:"size"`
	Version   int    `json:"version"`
}

// SubjectStatement returns the deterministic canonical JSON bytes that bind a subject's kind,
// digest, media type, and size: no whitespace and no trailing newline.
func SubjectStatement(subject artifact.SignatureSubject) []byte {
	defer perf.Track(nil, "sigstore.SubjectStatement")()

	raw, err := json.Marshal(subjectStatement{
		Digest:    subject.Digest.String(),
		Kind:      string(subject.Kind),
		MediaType: subject.MediaType,
		Size:      subject.Size,
		Version:   statementVersion,
	})
	if err != nil {
		// A struct of strings and integers always marshals.
		return nil
	}
	return raw
}

// bundleShape is the part of a Sigstore bundle needed to classify it.
type bundleShape struct {
	MediaType            string `json:"mediaType"`
	VerificationMaterial struct {
		Certificate          json.RawMessage `json:"certificate"`
		X509CertificateChain json.RawMessage `json:"x509CertificateChain"`
		PublicKey            json.RawMessage `json:"publicKey"`
	} `json:"verificationMaterial"`
}

// EvidenceFormatOf classifies a Sigstore bundle by its verification material and returns the
// evidence format and the bundle's own media type. A certificate or certificate chain means a
// keyless bundle; a public key identifier means a public-key bundle. A bundle that carries neither,
// or both, is malformed.
func EvidenceFormatOf(bundle []byte) (artifact.EvidenceFormat, string, error) {
	defer perf.Track(nil, "sigstore.EvidenceFormatOf")()

	if len(bundle) == 0 || int64(len(bundle)) > artifact.MaxEvidenceSize || len(bundle) > signature.MaxBundleSize {
		return "", "", fmt.Errorf("%w: bundle must be 1 to %d bytes", errUtils.ErrSignatureInvalid, artifact.MaxEvidenceSize)
	}
	var shape bundleShape
	if err := json.Unmarshal(bundle, &shape); err != nil {
		return "", "", fmt.Errorf("%w: %w", errUtils.ErrSignatureInvalid, err)
	}
	format, err := formatOfMaterial(&shape)
	if err != nil {
		return "", "", err
	}
	mediaType := shape.MediaType
	if mediaType == "" {
		mediaType = defaultBundleMediaType
	}
	return format, mediaType, nil
}

// formatOfMaterial maps the verification material to an evidence format.
func formatOfMaterial(shape *bundleShape) (artifact.EvidenceFormat, error) {
	material := shape.VerificationMaterial
	hasCertificate := isPresent(material.Certificate) || isPresent(material.X509CertificateChain)
	hasKey := isPresent(material.PublicKey)
	switch {
	case hasCertificate && hasKey:
		return "", fmt.Errorf("%w: bundle carries both a certificate and a public key", errUtils.ErrSignatureInvalid)
	case hasCertificate:
		return artifact.EvidenceSigstoreBundle, nil
	case hasKey:
		return artifact.EvidencePublicKeyBundle, nil
	default:
		return "", fmt.Errorf("%w: bundle carries neither a certificate nor a public key", errUtils.ErrSignatureInvalid)
	}
}

func isPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}
