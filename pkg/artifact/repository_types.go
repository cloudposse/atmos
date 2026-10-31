package artifact

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Label is an exact key/value selector or assignment.
type Label struct {
	// Key is the label key.
	Key string `json:"key"`

	// Value is the label value.
	Value string `json:"value"`
}

// Inspection is the metadata view of one artifact, available without downloading the payload.
type Inspection struct {
	// Pinned is the immutable reference that was inspected.
	Pinned PinnedReference `json:"pinned"`

	// Record is the portable Record; nil for a foreign OCI graph that carries none.
	Record *Record `json:"record,omitempty"`

	// Representation describes how the artifact is stored.
	Representation Representation `json:"representation"`

	// Evidence lists signature evidence found for the artifact. It is untrusted until verified.
	Evidence []SignatureEvidence `json:"evidence,omitempty"`
}

// DocumentStream is an open, digest-verifying reader over one package document.
type DocumentStream struct {
	// Document is the descriptor from the Record.
	Document Document

	// Reader yields the document bytes and fails with an integrity error at EOF on mismatch.
	// The caller closes it. Callers bound reads to [MaxDocumentSize].
	Reader io.ReadCloser
}

// ContentStream is one verified, closeable payload stream.
type ContentStream struct {
	// Descriptor identifies the blob.
	Descriptor ocispec.Descriptor

	// Role is payload or document.
	Role BlobRole

	// Path is the inventory path for per-file payloads; empty for archives.
	Path string

	// Reader yields the blob bytes and verifies the descriptor digest and size.
	Reader io.ReadCloser
}

// GraphSource exposes an existing OCI graph for transfer without importing a registry SDK.
type GraphSource interface {
	// Root returns the original manifest or index descriptor.
	Root() ocispec.Descriptor

	// Fetch returns the raw bytes of a graph node by descriptor.
	Fetch(ctx context.Context, descriptor ocispec.Descriptor) (io.ReadCloser, error)

	// Successors lists the descriptors a node references (config, layers, children).
	Successors(ctx context.Context, descriptor ocispec.Descriptor) ([]ocispec.Descriptor, error)
}

// Content is the result of reading an artifact: either verified payload streams or an OCI graph
// source. The caller must Close it.
type Content struct {
	// Record is the portable Record; nil for a foreign OCI graph that carries none.
	Record *Record

	// Representation describes how the content is stored.
	Representation Representation

	// Streams are the verified payload and document streams. Empty when Graph is set.
	Streams []ContentStream

	// Graph is the OCI graph source when a native graph is carried.
	Graph GraphSource
}

// Close closes every stream, returning the first error.
func (c *Content) Close() error {
	defer perf.Track(nil, "artifact.Content.Close")()

	var first error
	for index := range c.Streams {
		reader := c.Streams[index].Reader
		if reader == nil {
			continue
		}
		if err := reader.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// PublishOptions control one repository publication.
type PublishOptions struct {
	// ExpectedPrior is the current reference the service observed under the writer guard; nil
	// means the reference must not exist. Repositories use it for conditional updates.
	ExpectedPrior *PinnedReference

	// Force permits replacing a conflicting reference. It has already been checked by the service.
	Force bool

	// Lock is the writer lock already held by the service; nil if the repository locks internally.
	Lock LockHandle
}

// PublishResult is the outcome of a repository publication.
type PublishResult struct {
	// Outcome is created, unchanged, or replaced.
	Outcome Outcome

	// Pinned is the immutable reference of the published artifact.
	Pinned PinnedReference

	// Representation describes the stored form.
	Representation Representation

	// Location is the primary resolved location.
	Location string

	// Aliases lists additional locations or tags resolving to the same content.
	Aliases []string

	// LockScope is the actual lock scope that coordinated the write.
	LockScope LockScope
}

// ListQuery selects one page of managed references. The service consumes pagination.
type ListQuery struct {
	// Name restricts the listing to versions of one exact name; empty enumerates the collection.
	Name string

	// Labels keep only entries whose labels match every selector exactly.
	Labels []Label

	// Cursor is the opaque cursor from the previous page; empty starts at the beginning.
	Cursor string

	// PageSize bounds the page; zero lets the repository choose.
	PageSize int

	// Refresh bypasses cached discovery snapshots.
	Refresh bool
}

// ListEntry summarizes one artifact in a listing.
type ListEntry struct {
	// Pinned is the immutable reference.
	Pinned PinnedReference `json:"pinned"`

	// Name is the artifact name.
	Name string `json:"name"`

	// Version is the version label.
	Version string `json:"version"`

	// Kind is the artifact kind.
	Kind string `json:"kind"`

	// Description is the metadata description.
	Description string `json:"description,omitempty"`

	// Labels are the metadata labels.
	Labels map[string]string `json:"labels,omitempty"`

	// Documents are the document descriptors, never the Markdown itself.
	Documents []Document `json:"documents,omitempty"`

	// Repository is the configured repository alias the entry came from.
	Repository string `json:"repository,omitempty"`
}

// ListPage is one page of entries plus a cursor.
type ListPage struct {
	// Entries are the page's artifacts, sorted lexically by name then version.
	Entries []ListEntry

	// NextCursor is empty when the page is the last.
	NextCursor string
}

// DeleteOptions control one repository deletion.
type DeleteOptions struct {
	// ExpectedCurrent is the reference the service observed; nil skips the staleness check.
	ExpectedCurrent *PinnedReference

	// Scope is the explicit deletion scope. A tag-safe delete must not remove sibling references.
	Scope DeleteSemantics

	// Lock is the writer lock already held by the service.
	Lock LockHandle
}

// DeleteResult reports what a deletion removed and affected.
type DeleteResult struct {
	// Deleted lists removed locations.
	Deleted []string

	// Affected lists other references that resolved to the removed content.
	Affected []Ref

	// Digest is the removed resolved digest.
	Digest digest.Digest
}

// SignOptions control evidence storage.
type SignOptions struct {
	// OperationID is the logical operation identifier.
	OperationID string

	// Lock is the writer lock already held by the service.
	Lock LockHandle
}
