// Package produce implements the generic "files" artifact producer and the seams that
// kind-specific producers (scaffold, component, and so on) reuse.
//
// Preparation snapshots the source into bounded disk staging (see package snapshot), resolves the
// artifact name and version, merges labels, recognizes the reserved .atmos/artifact distribution
// directory and explicit README/CHANGELOG overrides, seals a canonical Record, and stages the
// payload either as per-file blobs or as a deterministic tar, tgz, or zip archive. The user's
// source tree is never modified; the generated manifest.json exists only inside the packaged
// archive and never appears in the Record's inventory.
//
// Kind-specific producers reuse that pipeline and add their own validation, metadata, and visible
// default exclusions. [NewScaffold] validates scaffold configuration (through a [ScaffoldValidator]
// that the scaffold command registers) without generating output or running hooks.
// [NewTerraformComponent] leaves out Terraform working directories, state, and saved plans and
// reports each exclusion with its rule. [RegisterDefaults] registers all three. The generic files
// producer never applies another kind's exclusions.
package produce
