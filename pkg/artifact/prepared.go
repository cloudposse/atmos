package artifact

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloudposse/atmos/pkg/perf"
)

// BlobRole identifies what a staged blob contains.
type BlobRole string

// Staged blob roles.
const (
	// BlobPayload is payload content (an archive or one file).
	BlobPayload BlobRole = "payload"

	// BlobDocument is a README or CHANGELOG document.
	BlobDocument BlobRole = "document"
)

// StagedBlob is one blob ready to be uploaded. Open returns a fresh reader each time, so a retry
// within one invocation reuses the same prepared content.
type StagedBlob struct {
	// Descriptor identifies the blob by media type, digest, and size.
	Descriptor ocispec.Descriptor

	// Role is payload or document.
	Role BlobRole

	// DocumentRole is the document role when Role is [BlobDocument].
	DocumentRole DocumentRole

	// Path is the inventory path for per-file payloads; empty for archives.
	Path string

	// Open returns a new reader positioned at the start of the blob. The caller closes it.
	Open func(ctx context.Context) (io.ReadCloser, error)
}

// Exclusion records one source entry that a kind-specific producer left out of the payload, and the
// visible rule that excluded it. Exclusions are reported with the plan; they are not part of the
// Record, so they never affect its digests.
type Exclusion struct {
	// Path is the slash-separated source path relative to the source root. A directory is listed
	// once, without its descendants.
	Path string `json:"path"`

	// Rule is the default rule that excluded the entry, for example ".terraform/**".
	Rule string `json:"rule"`
}

// PreparedArtifact is the output of a [Producer]: a sealed Record plus staged blobs. The preparer
// captured one UTC preparation time and one logical operation ID before the first attempt.
//
// A PreparedArtifact owns temporary resources. Whoever finally consumes it must call Close, which
// runs the cleanup function exactly once. It must not be copied after first use.
type PreparedArtifact struct {
	// Record is the sealed Record to publish.
	Record Record

	// IntentDigest is the Record's intent digest.
	IntentDigest digest.Digest

	// OperationID is the logical operation identifier, stable across retries.
	OperationID string

	// PreparedAt is the single UTC preparation time.
	PreparedAt time.Time

	// Packaging is the packaging mode of the staged payload.
	Packaging Packaging

	// Blobs are the staged payload and document blobs.
	Blobs []StagedBlob

	// Exclusions lists the source entries a kind-specific producer left out of the payload.
	Exclusions []Exclusion

	// Cleanup releases temporary resources owned by this operation; may be nil.
	Cleanup func() error

	closeOnce sync.Once
	closeErr  error
}

// Close releases the temporary resources owned by the PreparedArtifact. It is idempotent: the
// cleanup function runs at most once and later calls return the first result.
func (p *PreparedArtifact) Close() error {
	defer perf.Track(nil, "artifact.PreparedArtifact.Close")()

	p.closeOnce.Do(func() {
		if p.Cleanup != nil {
			p.closeErr = p.Cleanup()
		}
	})
	return p.closeErr
}
