package artifact

import (
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// PinnedReference is an immutable reference suitable for consumer lockfiles. It is not a write
// lock and not a substitute for signature verification; trust policy is reevaluated on use.
type PinnedReference struct {
	// Ref is the reference that was pinned, as requested.
	Ref Ref `json:"ref"`

	// Repository is the repository locator (a registry path, bucket and prefix, or directory),
	// never credentials.
	Repository string `json:"repository"`

	// ResolvedDigest is the OCI manifest/index digest, or the record digest for local and S3.
	ResolvedDigest digest.Digest `json:"resolved_digest"`

	// RecordDigest is the digest of the canonical Record bytes, when a Record is bound.
	RecordDigest digest.Digest `json:"record_digest,omitempty"`

	// ContentDigest is the portable content digest from the Record, when a Record is bound.
	ContentDigest digest.Digest `json:"content_digest,omitempty"`

	// Evidence lists the signature evidence descriptors known at pin time.
	Evidence []ocispec.Descriptor `json:"evidence,omitempty"`

	// Platform is the selected platform when a multi-platform index was narrowed.
	Platform *ocispec.Platform `json:"platform,omitempty"`
}
