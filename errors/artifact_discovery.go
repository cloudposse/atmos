package errors

import "errors"

// Artifact discovery cache and catalog sentinels. They classify outcomes of enumerating artifact
// repositories through the shared discovery cache and of maintaining optional artifact catalogs.
var (
	// ErrArtifactDiscoveryOffline is returned when offline discovery has no valid snapshot to serve,
	// or when a refresh is requested while offline. Offline discovery never reaches a repository.
	ErrArtifactDiscoveryOffline = errors.New("artifact discovery is unavailable offline")

	// ErrArtifactDiscoveryTooLarge is returned when a discovery snapshot would exceed the cache size bound.
	ErrArtifactDiscoveryTooLarge = errors.New("artifact discovery snapshot exceeds the size limit")

	// ErrArtifactDiscoveryIncomplete is returned when a collection cannot be presented as complete
	// because a source failed, was unauthorized, or only part of it could be read.
	ErrArtifactDiscoveryIncomplete = errors.New("artifact discovery result is incomplete")

	// ErrArtifactCatalogInvalid is returned when a catalog or inventory manifest fails validation, or
	// references a location outside the repository it is bound to.
	ErrArtifactCatalogInvalid = errors.New("invalid artifact catalog")

	// ErrArtifactCatalogConflict is returned when a catalog changed since the caller read it, so a
	// stale revision or digest must not be published over newer metadata.
	ErrArtifactCatalogConflict = errors.New("artifact catalog changed; reload it before publishing")

	// ErrArtifactCatalogNotConfigured is returned by a mutating catalog command for a repository
	// that has no catalog reference configured.
	ErrArtifactCatalogNotConfigured = errors.New("artifact repository has no catalog configured")
)
