package artifact

import (
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Representation describes how one Record is physically stored by a repository. It is mutable
// storage metadata and is never part of the Record bytes.
type Representation struct {
	// Packaging is the payload packaging mode.
	Packaging Packaging `json:"packaging"`

	// Payload lists the payload blob descriptors (archive, per-file objects, or image layers).
	Payload []ocispec.Descriptor `json:"payload,omitempty"`

	// Documents lists the README and CHANGELOG blob descriptors stored alongside the payload.
	Documents []ocispec.Descriptor `json:"documents,omitempty"`

	// Locator is the backend-specific location (path, object key, or registry reference).
	Locator string `json:"locator"`

	// RecordDigest is the digest of the canonical Record bytes this representation belongs to.
	RecordDigest digest.Digest `json:"record_digest"`

	// RepresentationDigest is the checksum of an archive or other transferred payload representation.
	RepresentationDigest digest.Digest `json:"representation_digest,omitempty"`

	// OriginalRoot is the original OCI manifest or index descriptor when a native graph is carried.
	OriginalRoot *ocispec.Descriptor `json:"original_root,omitempty"`
}
