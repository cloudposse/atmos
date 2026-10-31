// Package catalog maintains optional artifact catalogs: discovery snapshots published as generic
// artifact Records of kind "catalog" (artifact type application/vnd.atmos.artifact.catalog.v1) with
// one direct JSON payload layer (application/vnd.atmos.artifact.catalog.v1+json).
//
// A catalog is discoverable data, never a trust grant. It lists names, versions, digests, labels,
// and document descriptors of artifacts in the repository it is bound to, so a collection can be
// searched where the registry cannot enumerate repositories. Every entry is bound to the actual
// repository path (never credentials) and is validated against the destination repository before
// any use. Publishing, signing, or deleting an artifact never edits its catalog, and removing a
// catalog entry only unlists it: content, signatures, and consumer lock files are untouched.
//
// The AtmosArtifactCatalog and AtmosArtifactInventory manifests are separate from the plugin
// catalog schema. Version labels are opaque; names and versions sort lexically, with no SemVer
// semantics. Catalog writes require one externally serialized publisher spanning read-latest,
// build, and publish. Revision and digest checks reject already-stale input and the read inside the
// write guard sees the latest catalog, but registries offer no atomic compare-and-swap, so a
// preflight check alone does not prevent races. The repository coordinator (lock) is used when the
// repository provides one.
package catalog
