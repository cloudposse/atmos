package artifact

import (
	"github.com/opencontainers/go-digest"
)

// Record is the portable, canonical description of one published artifact. It is independent of
// the repository alias, credentials, resolved storage paths, upload duration, and archive or OCI
// envelope digests; those belong to [Representation] and [OperationResult].
//
// A Record never contains its own hash. Serialize it with [Record.Canonical]; the struct's field
// order below is the canonical field order and is frozen by test vectors.
type Record struct {
	// SchemaVersion is always [SchemaVersion] (1).
	SchemaVersion int `json:"schema_version"`

	// Name is the artifact name, e.g. "components/terraform/vpc".
	Name string `json:"name"`

	// Version is the explicit version label.
	Version string `json:"version"`

	// Kind is the artifact kind, e.g. "files" or "component/terraform".
	Kind string `json:"kind"`

	// CreatedAt is the creation time, captured once per publication in UTC.
	CreatedAt Timestamp `json:"created_at"`

	// Metadata is the user-visible description, labels, and optional provenance.
	Metadata Metadata `json:"metadata"`

	// Inventory lists every payload entry, sorted by path in the canonical form.
	Inventory []Entry `json:"inventory"`

	// Documents lists the README and CHANGELOG descriptors, sorted by role in the canonical form.
	Documents []Document `json:"documents"`

	// ContentDigest is the SHA-256 of the canonical inventory and document descriptors.
	ContentDigest digest.Digest `json:"content_digest"`

	// KindData is optional kind-specific data. It is held as canonical JSON; see [KindData].
	KindData KindData `json:"kind_data,omitempty"`
}

// Metadata is the descriptive metadata of a Record. No stack, component, or SHA is required.
type Metadata struct {
	// Description is a short human-readable description.
	Description string `json:"description,omitempty"`

	// Labels are opaque string key/value pairs set by the user.
	Labels map[string]string `json:"labels,omitempty"`

	// Provenance is generated CI and source context. It is part of the record digest but excluded
	// from the intent digest so that retries and no-op detection are not defeated by run IDs.
	Provenance *Provenance `json:"provenance,omitempty"`
}

// Provenance carries optional CI and source context captured at publication time.
type Provenance struct {
	// CI describes the CI run that produced the artifact.
	CI *CIProvenance `json:"ci,omitempty"`

	// Source describes the source tree the artifact was built from.
	Source *SourceProvenance `json:"source,omitempty"`
}

// CIProvenance identifies the CI run that published an artifact.
type CIProvenance struct {
	// Provider is the CI provider name, e.g. "github".
	Provider string `json:"provider,omitempty"`

	// RunID is the provider's run identifier.
	RunID string `json:"run_id,omitempty"`

	// RunURL links to the run.
	RunURL string `json:"run_url,omitempty"`

	// Job is the job or step name.
	Job string `json:"job,omitempty"`
}

// SourceProvenance identifies the source revision an artifact was built from.
type SourceProvenance struct {
	// Repository is the source repository identifier or URL (without credentials).
	Repository string `json:"repository,omitempty"`

	// Commit is the source commit SHA.
	Commit string `json:"commit,omitempty"`

	// Ref is the branch or tag name.
	Ref string `json:"ref,omitempty"`
}

// EntryType is the type of an inventory entry.
type EntryType string

// Inventory entry types. Devices, sockets, and other special files are never representable.
const (
	// EntryFile is a regular file.
	EntryFile EntryType = "file"

	// EntryDirectory is a directory.
	EntryDirectory EntryType = "directory"

	// EntrySymlink is a symbolic link whose target stays inside the package.
	EntrySymlink EntryType = "symlink"
)

// Entry is one inventory item. Hard links are represented as independent regular files.
type Entry struct {
	// Path is the relative, slash-separated path inside the package.
	Path string `json:"path"`

	// Type is file, directory, or symlink.
	Type EntryType `json:"type"`

	// Size is the file size in bytes; zero for directories and symlinks.
	Size int64 `json:"size"`

	// Mode holds ordinary permission bits (0o777 mask); privileged bits are always cleared.
	// Symlinks have mode zero so equivalent trees share one canonical form.
	Mode uint32 `json:"mode"`

	// Digest is the SHA-256 of a regular file's bytes; empty for other types.
	Digest digest.Digest `json:"digest,omitempty"`

	// Target is a symlink's relative target; empty for other types.
	Target string `json:"target,omitempty"`
}

// DocumentRole is the role of a package document.
type DocumentRole string

// Package document roles.
const (
	// DocumentReadme is the package README.
	DocumentReadme DocumentRole = "readme"

	// DocumentChangelog is the package CHANGELOG.
	DocumentChangelog DocumentRole = "changelog"
)

// Document describes one distribution document. Catalogs carry descriptors, never full Markdown.
type Document struct {
	// Role is readme or changelog. At most one document exists per role.
	Role DocumentRole `json:"role"`

	// SourcePath is the original source path, relative to the publish source.
	SourcePath string `json:"source_path"`

	// MediaType is the document media type, normally [DocumentMediaType].
	MediaType string `json:"media_type"`

	// Size is the document size in bytes (at most [MaxDocumentSize]).
	Size int64 `json:"size"`

	// Digest is the SHA-256 of the document bytes.
	Digest digest.Digest `json:"digest"`
}
