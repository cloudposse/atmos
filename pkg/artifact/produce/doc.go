// Package produce implements the generic "files" artifact producer and the seams that
// kind-specific producers (scaffold, component, and so on) reuse.
//
// Preparation snapshots the source into bounded disk staging (see package snapshot), resolves the
// artifact name and version, merges labels, recognizes the reserved .atmos/artifact distribution
// directory and explicit README/CHANGELOG overrides, seals a canonical Record, and stages the
// payload either as per-file blobs or as a deterministic tar, tgz, or zip archive. The user's
// source tree is never modified; the generated manifest.json exists only inside the packaged
// archive and never appears in the Record's inventory.
package produce
