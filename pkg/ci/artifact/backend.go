package artifact

import (
	"context"
	"io"
)

//go:generate mockgen -package artifact -destination mock_backend.go github.com/cloudposse/atmos/pkg/ci/artifact Backend

// Backend defines the interface for low-level artifact storage.
// Backends store a single data stream (typically a tar archive) plus a metadata sidecar.
// The BundledStore wraps a Backend to provide the higher-level Store interface
// that handles bundling []FileEntry into a tar archive.
type Backend interface {
	// Name returns the backend type name (e.g., "aws/s3", "local/dir", "github/artifacts").
	Name() string

	// Upload uploads a single data stream with the given name and metadata.
	Upload(ctx context.Context, name string, data io.Reader, size int64, metadata *Metadata) error

	// Download downloads a single data stream by name.
	// Returns the data reader, metadata, and any error.
	// Callers must close the returned io.ReadCloser when done.
	Download(ctx context.Context, name string) (io.ReadCloser, *Metadata, error)

	// Delete deletes an artifact by name.
	Delete(ctx context.Context, name string) error

	// List lists artifacts matching the given query.
	List(ctx context.Context, query Query) ([]ArtifactInfo, error)

	// Exists checks if an artifact exists.
	Exists(ctx context.Context, name string) (bool, error)

	// GetMetadata retrieves metadata for an artifact without downloading the content.
	GetMetadata(ctx context.Context, name string) (*Metadata, error)
}

// BackendFactory is a function that creates a Backend from options.
type BackendFactory func(opts StoreOptions) (Backend, error)
