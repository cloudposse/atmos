package catalog

import "context"

//go:generate mockgen -package catalog -destination mock_maintainer.go github.com/cloudposse/atmos/pkg/artifact/discovery/catalog Maintainer

// Maintainer is the catalog maintenance API the CLI consumes. [Service] implements it.
type Maintainer interface {
	// Build resolves references into a catalog snapshot without writing anything.
	Build(ctx context.Context, request *BuildRequest) (*BuildResult, error)

	// Publish replaces the configured catalog with a built snapshot.
	Publish(ctx context.Context, request *PublishRequest) (*Result, error)

	// Add lists releases in the latest catalog.
	Add(ctx context.Context, request *AddRequest) (*Result, error)

	// Remove unlists a version without deleting the artifact.
	Remove(ctx context.Context, request *RemoveRequest) (*Result, error)

	// Sync merges inventory releases into the latest catalog and keeps the other entries.
	Sync(ctx context.Context, request *SyncRequest) (*Result, error)
}

var _ Maintainer = (*Service)(nil)
