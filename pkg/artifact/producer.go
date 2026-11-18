package artifact

import (
	"context"
	"time"
)

//go:generate mockgen -package artifact -destination mock_producer.go github.com/cloudposse/atmos/pkg/artifact Producer

// PrepareRequest carries everything a Producer needs. The service captures PreparedAt (UTC) and
// OperationID once, before the first attempt, and reuses them on every retry.
type PrepareRequest struct {
	// Source is the local source file or directory.
	Source string

	// Name is the explicit artifact name; empty lets the producer supply one.
	Name string

	// Version is the explicit version; empty lets the producer supply one or the operation fails.
	Version string

	// Kind is the artifact kind; empty means [KindFiles].
	Kind string

	// Metadata is the effective user metadata (description, labels) merged by the service.
	Metadata Metadata

	// Readme overrides the README source path, relative to the source directory.
	Readme string

	// Changelog overrides the CHANGELOG source path, relative to the source directory.
	Changelog string

	// Packaging is the target repository's effective packaging mode.
	Packaging Packaging

	// OperationID is the logical operation identifier.
	OperationID string

	// PreparedAt is the single UTC preparation time.
	PreparedAt time.Time
}

// Producer turns a source into a sealed Record plus staged blobs. Producers walk sources with
// lstat, never follow links outside the source, and never mutate the user's source tree. The
// caller owns the returned PreparedArtifact and must Close it.
type Producer interface {
	// Kind returns the artifact kind this producer supplies, for example "files".
	Kind() string

	// Prepare reads the source and returns a stable, sealed PreparedArtifact.
	Prepare(ctx context.Context, request PrepareRequest) (*PreparedArtifact, error)
}
