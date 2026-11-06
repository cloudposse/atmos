package artifact

import (
	"fmt"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SubjectKind identifies what a signature is bound to.
type SubjectKind string

// Signature subject kinds.
const (
	// SubjectRecord is the canonical portable Record of an Atmos file package. Evidence survives
	// representation changes because the Record binds name, version, metadata, inventory,
	// content_digest, and document digests.
	SubjectRecord SubjectKind = "record"

	// SubjectOCIRoot is the original OCI manifest or index digest of a foreign image or an
	// existing plugin. It is never rewritten on copy and never replaced by an archive checksum.
	SubjectOCIRoot SubjectKind = "oci-root"
)

// EvidenceFormat identifies a signature evidence encoding.
type EvidenceFormat string

// Evidence formats supported in V1. Other formats require dedicated tested providers; unsupported
// required evidence fails closed.
const (
	// EvidenceSigstoreBundle is a Sigstore bundle verified against trusted certificate identities.
	EvidenceSigstoreBundle EvidenceFormat = "sigstore-bundle"

	// EvidencePublicKeyBundle is a bundle verified against an explicitly configured public key.
	// It never satisfies an OIDC issuer/identity requirement.
	EvidencePublicKeyBundle EvidenceFormat = "public-key-bundle"
)

// SignatureSubject is the immutable thing a signature covers.
type SignatureSubject struct {
	// Kind is record or oci-root.
	Kind SubjectKind `json:"kind"`

	// Digest is the immutable subject digest (record digest or OCI root digest).
	Digest digest.Digest `json:"digest"`

	// MediaType is the subject media type.
	MediaType string `json:"media_type"`

	// Size is the subject size in bytes.
	Size int64 `json:"size"`
}

// Validate checks the subject is well formed.
func (s SignatureSubject) Validate() error {
	defer perf.Track(nil, "artifact.SignatureSubject.Validate")()

	if s.Kind != SubjectRecord && s.Kind != SubjectOCIRoot {
		return fmt.Errorf("%w: unsupported signature subject kind %q", errUtils.ErrArtifactRefInvalid, s.Kind)
	}
	if err := ValidateDigest(s.Digest); err != nil {
		return err
	}
	if s.Size < 0 {
		return fmt.Errorf("%w: signature subject has a negative size", errUtils.ErrArtifactRefInvalid)
	}
	return nil
}

// SignatureEvidence locates stored evidence for a subject. It is untrusted until verified.
type SignatureEvidence struct {
	// Subject is the subject the evidence claims to cover.
	Subject SignatureSubject `json:"subject"`

	// Format is the evidence encoding.
	Format EvidenceFormat `json:"format"`

	// Descriptor identifies the evidence blob (media type, digest, size).
	Descriptor ocispec.Descriptor `json:"descriptor"`

	// Locator is the backend-specific storage location, for example
	// ".atmos-artifacts/v1/signatures/<subject-kind>/<subject-digest>/<evidence-digest>.json".
	Locator string `json:"locator,omitempty"`
}

// EvidenceBlob is evidence together with its bytes, bounded by [MaxEvidenceSize].
type EvidenceBlob struct {
	// Evidence locates and describes the blob.
	Evidence SignatureEvidence

	// Data holds the evidence bytes.
	Data []byte
}

// VerificationStatus is the outcome of a verification.
type VerificationStatus string

// Verification statuses.
const (
	// VerificationVerified means a valid permitted signature satisfied every required trust set.
	VerificationVerified VerificationStatus = "verified"

	// VerificationUnchecked means no trust configuration was supplied, so nothing was claimed.
	VerificationUnchecked VerificationStatus = "unchecked"

	// VerificationFailed means required verification did not succeed.
	VerificationFailed VerificationStatus = "failed"
)

// SignerClaim is an authenticated signer identity extracted by a verifier. Metadata claims,
// labels, catalog entries, and cached booleans never produce claims.
type SignerClaim struct {
	// Identity is the certificate identity (SAN) for keyless signing; empty for key signing.
	Identity string `json:"identity,omitempty"`

	// Issuer is the OIDC issuer for keyless signing; empty for key signing.
	Issuer string `json:"issuer,omitempty"`

	// KeyID is the fingerprint of the trusted public key for key-based signing.
	KeyID string `json:"key_id,omitempty"`
}

// VerificationResult is the authenticated result of verifying a subject against a policy.
type VerificationResult struct {
	// Subject is the verified subject.
	Subject SignatureSubject `json:"subject"`

	// PolicyFingerprint is [VerificationPolicy.Fingerprint] of the policy that was applied.
	PolicyFingerprint string `json:"policy_fingerprint,omitempty"`

	// Claims are the verified signer claims.
	Claims []SignerClaim `json:"claims,omitempty"`

	// EvidenceDigests lists the digests of the evidence blobs that were considered.
	EvidenceDigests []digest.Digest `json:"evidence_digests,omitempty"`

	// Status is verified, unchecked, or failed.
	Status VerificationStatus `json:"status"`

	// Diagnostics describes the failure reason and any bounded malformed or unsupported evidence.
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}
