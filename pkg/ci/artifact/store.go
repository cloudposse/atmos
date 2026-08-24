package artifact

import (
	"context"
	"io"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

//go:generate mockgen -package artifact -destination mock_store.go github.com/cloudposse/atmos/pkg/ci/artifact Store

// FileEntry represents a file to be uploaded as part of an artifact bundle.
type FileEntry struct {
	// Name is the filename within the artifact bundle.
	Name string

	// Data is the file content reader.
	Data io.Reader

	// Size is the file size in bytes (-1 if unknown).
	Size int64
}

// FileResult represents a file downloaded from an artifact bundle.
type FileResult struct {
	// Name is the filename within the artifact bundle.
	Name string

	// Data is the file content reader. Callers must close it when done.
	Data io.ReadCloser

	// Size is the file size in bytes (-1 if unknown).
	Size int64
}

// Store defines the interface for artifact storage backends.
// Implementations include S3, Azure Blob, GCS, GitHub Artifacts, and local filesystem.
type Store interface {
	// Name returns the store type name (e.g., "aws/s3", "azure/blob", "google/gcs", "github/artifacts", "local/dir").
	Name() string

	// Upload uploads an artifact bundle to the store.
	Upload(ctx context.Context, name string, files []FileEntry, metadata *Metadata) error

	// Download downloads an artifact bundle from the store.
	Download(ctx context.Context, name string) ([]FileResult, *Metadata, error)

	// Delete deletes an artifact from the store.
	Delete(ctx context.Context, name string) error

	// List lists artifacts matching the given query.
	List(ctx context.Context, query Query) ([]ArtifactInfo, error)

	// Exists checks if an artifact exists.
	Exists(ctx context.Context, name string) (bool, error)

	// GetMetadata retrieves metadata for an artifact without downloading the content.
	GetMetadata(ctx context.Context, name string) (*Metadata, error)
}

// StoreOptions contains options for creating a store.
type StoreOptions struct {
	// Type is the store type (s3, azure, gcs, github, local).
	Type string

	// Options contains type-specific configuration options.
	Options map[string]any

	// AtmosConfig is the Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration

	// Identity is the Atmos auth identity name. When non-empty, identity-aware
	// backends defer client initialization until Resolver supplies credentials.
	Identity string

	// Resolver supplies cloud-specific auth credentials for Identity.
	Resolver store.AuthContextResolver
}

// AuthContextResolver is re-exported so backends import a single package.
type AuthContextResolver = store.AuthContextResolver

// AWSAuthConfig is re-exported so backends import a single package.
type AWSAuthConfig = store.AWSAuthConfig

// IdentityAwareBackend is implemented by backends that authenticate via an
// Atmos auth identity. The registry calls SetAuthContext after construction.
type IdentityAwareBackend interface {
	Backend
	// SetAuthContext injects the resolver and identity name. An empty
	// identityName preserves the identity supplied at construction.
	SetAuthContext(resolver AuthContextResolver, identityName string)
}

// StoreFactory is a function that creates a Store from options.
// Deprecated: Use BackendFactory instead. Backends are wrapped in BundledStore by the registry.
type StoreFactory func(opts StoreOptions) (Store, error)
