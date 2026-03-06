# Unify Planfile Stores into Artifact Store Layer

> Related: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Interfaces](../framework/interfaces.md) | [Planfile Metadata Embed Artifact](planfile-metadata-embed-artifact.md) | [Planfile Bundle with Lockfile](planfile-bundle-with-lockfile.md)

## Prerequisites

This PRD **requires** the following PRDs to be implemented first:

1. **[Planfile Bundle with Lockfile](planfile-bundle-with-lockfile.md)** — Establishes the tar bundle format for plan + lock files and the `CreateBundle`/`ExtractBundle` functions.
2. **[Planfile Metadata Embed Artifact](planfile-metadata-embed-artifact.md)** — Makes `planfile.Metadata` embed `artifact.Metadata`, enabling unified metadata handling.

These are prerequisites because this PRD moves bundling responsibility from the planfile layer into the artifact store layer and relies on the metadata embedding for the adapter's metadata conversion.

## Problem Statement

The codebase has two parallel store architectures that implement the same storage backends with nearly identical code:

- **`pkg/ci/artifact/`** — Generic artifact store interface with a local filesystem implementation.
- **`pkg/ci/plugins/terraform/planfile/`** — Terraform planfile-specific store interface with local, S3, and GitHub implementations.

This creates several problems:

### 1. Duplicated store implementations

The planfile `local/store.go` (342 lines) and `s3/store.go` (321 lines) duplicate storage logic that has nothing to do with Terraform planfiles. Both implement the same pattern: blob + JSON sidecar metadata. The artifact `local/store.go` (360 lines) implements the same pattern independently. Three files contain nearly identical logic for:
- Path traversal validation (`validateKey` / `validateName`)
- Metadata sidecar loading/saving (`loadMetadata`)
- Empty directory cleanup (`cleanupEmptyDirs`)
- Error wrapping patterns

### 2. Missing artifact-level S3 and GitHub backends

`artifact.Store` has only a local implementation. S3 and GitHub backends exist only under `planfile/`. Any future non-planfile artifact (e.g., drift detection reports, cost estimates) would need to duplicate these backends again or wait for them to be ported.

### 3. Planfile stores contain no planfile-specific logic

After the bundle PRD, planfile stores are pure blob stores: they receive a tar archive as `io.Reader` and store it alongside a JSON sidecar. The only planfile-aware code is the `planfile.Metadata` type used for the sidecar — but with embedding, this is just `artifact.Metadata` plus a few extra fields serialized to JSON.

### 4. Duplicated registry pattern

`planfile/registry.go` and `artifact/registry.go` are 61 lines each with identical logic (Register, NewStore, GetRegisteredTypes). The only differences are error sentinel names and perf tracking labels.

### 5. Adapter exists as a workaround

`planfile/adapter/store.go` (262 lines) bridges `artifact.Store` to `planfile.Store`. It exists because the two interfaces differ:
- `artifact.Store.Upload` takes `[]FileEntry` (multi-file), `planfile.Store.Upload` takes `io.Reader` (single blob).
- `artifact.Store.List` takes `Query`, `planfile.Store.List` takes prefix string.

If planfile storage used `artifact.Store` directly, the adapter would be simpler.

### 6. Interface divergence prevents reuse

The `planfile.Store` interface accepts `planfile.Metadata` in Upload and returns it from Download. The `artifact.Store` interface accepts `artifact.Metadata`. Since `planfile.Metadata` embeds `artifact.Metadata`, the planfile layer only needs to serialize/deserialize the extra fields — which JSON handles automatically via the embedded struct. The store itself never interprets the metadata; it just stores and retrieves it as JSON.

## Desired State

1. **S3, GitHub, and local backends live in `pkg/ci/artifact/`** as implementations of `artifact.Store`. They are generic multi-file artifact stores with no planfile awareness. Each store internally decides its persistence format (e.g., local uses filesystem directories, S3 uses tar archives, GitHub uses zip archives).

2. **Planfile stores are removed.** `planfile/local/`, `planfile/s3/`, `planfile/github/` are deleted. The planfile layer uses `artifact.Store` via the adapter.

3. **Single registry.** `artifact.Register()` is the only store registry. The planfile registry is removed. Store type names (`local`, `s3`, `github-artifacts`) are shared.

4. **Bundling moves into artifact stores.** Each artifact store implementation handles bundling internally — callers pass `[]FileEntry` and receive `[]FileResult`. The planfile layer no longer pre-bundles files into tar archives. `planfile/bundle.go` is deleted.

5. **planfile.Store interface is kept as a thin wrapper.** The adapter converts between `planfile.Store` (single-blob, planfile-specific metadata) and `artifact.Store` (multi-file, generic metadata). The adapter handles metadata conversion via the Custom map and file entry conversion.

6. **Both error sentinel types are kept.** Artifact stores use `ErrArtifact*` sentinels. The planfile adapter wraps artifact errors as `ErrPlanfile*` for planfile-layer consumers.

## Architecture

### Current architecture

```
planfile.Store (single-blob interface)
├── planfile/local/store.go     → implements planfile.Store
├── planfile/s3/store.go        → implements planfile.Store
├── planfile/github/store.go    → implements planfile.Store
└── planfile/adapter/store.go   → wraps artifact.Store → planfile.Store

artifact.Store (multi-file interface)
└── artifact/local/store.go     → implements artifact.Store
```

### Target architecture

```
artifact.Store (multi-file interface, unchanged)
├── artifact/local/store.go     → implements artifact.Store (exists)
├── artifact/s3/store.go        → implements artifact.Store (ported from planfile/s3)
└── artifact/github/store.go    → implements artifact.Store (ported from planfile/github)

planfile layer (no store implementations)
├── planfile/interface.go       → planfile.Store, Metadata, KeyPattern, KeyContext
└── planfile/adapter/store.go   → thin wrapper: artifact.Store → planfile.Store
```

### Interface preservation

The `artifact.Store` interface **keeps** its multi-file signature unchanged:

```go
// artifact.Store — unchanged
Upload(ctx context.Context, name string, files []FileEntry, metadata *Metadata) error
Download(ctx context.Context, name string) ([]FileResult, *Metadata, error)
```

The `planfile.Store` interface **keeps** its single-blob signature unchanged:

```go
// planfile.Store — unchanged
Upload(ctx context.Context, key string, data io.Reader, metadata *Metadata) error
Download(ctx context.Context, key string) (io.ReadCloser, *Metadata, error)
```

The adapter bridges these two interfaces, same as today.

### Bundling responsibility

Bundling moves **into** artifact store implementations. Each store internally decides how to persist multiple files:

- **Local store**: Stores each `FileEntry` as a separate file in a directory (e.g., `{name}/plan.tfplan`, `{name}/.terraform.lock.hcl`), plus `{name}.metadata.json` sidecar.
- **S3 store**: Creates a tar archive from `[]FileEntry`, uploads as a single S3 object, plus `{key}.metadata.json` sidecar object.
- **GitHub store**: Creates a zip archive from `[]FileEntry` (GitHub Actions artifact API requires zip), plus `metadata.json` entry inside the zip.

Callers never deal with tar/zip — they pass named files in and get named files out.

### Download and well-known filenames

On download, `artifact.Store.Download()` returns `[]FileResult` where each result has a `Name` field. The planfile adapter maps results by well-known names defined as constants:

```go
// planfile package constants (already exist from bundle PRD)
const (
    BundlePlanFilename = "plan.tfplan"
    BundleLockFilename = ".terraform.lock.hcl"
)
```

The adapter:
1. Calls `artifact.Store.Download()` → gets `[]FileResult`.
2. Finds the entry with `Name == BundlePlanFilename` → returns its data as the planfile `io.ReadCloser`.
3. Finds the optional entry with `Name == BundleLockFilename` → if present, returns lock file data alongside.

The planfile handler then writes each file to the appropriate output path.

### Metadata handling

Stores are metadata-agnostic. They serialize whatever metadata struct is passed to them as JSON, and deserialize it on download. The `artifact.Metadata` struct remains the common base. The planfile adapter converts `planfile.Metadata` ↔ `artifact.Metadata` using the Custom map for planfile-specific fields (same as today).

### Error handling

Both error sentinel types are preserved:

- **Artifact stores** use `ErrArtifact*` sentinels (`ErrArtifactUploadFailed`, `ErrArtifactDownloadFailed`, etc.).
- **Planfile adapter** wraps artifact errors as `ErrPlanfile*` for planfile-layer consumers (`ErrPlanfileUploadFailed`, `ErrPlanfileDownloadFailed`, etc.).

This preserves backward compatibility for existing error checks in CLI commands and plugin handlers.

## Implementation Steps

### Step 1: Move S3 store to `pkg/ci/artifact/s3/`

Create `pkg/ci/artifact/s3/store.go` by porting `planfile/s3/store.go`:
- Change the interface from `planfile.Store` to `artifact.Store`.
- Replace `planfile.Metadata` with `artifact.Metadata` in all signatures.
- Replace `planfile.StoreOptions` with `artifact.StoreOptions`.
- Replace `planfile.Register` with `artifact.Register`.
- Replace `planfile.PlanfileInfo` with `artifact.ArtifactInfo`.
- Replace `errUtils.ErrPlanfile*` with `errUtils.ErrArtifact*` error sentinels.
- **Upload**: Accept `[]FileEntry`, create a tar archive internally from the file entries, upload as single S3 object + metadata sidecar.
- **Download**: Download tar from S3, extract into `[]FileResult` entries, return alongside metadata.
- The S3 logic (PutObject, GetObject, sidecar metadata) remains identical.

Port `planfile/s3/store_test.go` to `artifact/s3/store_test.go`, updating for multi-file interface.

### Step 2: Move GitHub store to `pkg/ci/artifact/github/`

Create `pkg/ci/artifact/github/store.go` by porting `planfile/github/store.go`:
- Change the interface from `planfile.Store` to `artifact.Store`.
- Replace metadata types and error sentinels (same as S3).
- **Upload**: Accept `[]FileEntry`, create a zip archive internally from the file entries + metadata sidecar, upload via GitHub Actions artifact API.
- **Download**: Download zip, extract `[]FileResult` entries + metadata, return.
- The GitHub API logic (runtime uploader, JWT parsing) remains identical.

Port `planfile/github/store_test.go` to `artifact/github/store_test.go`, updating for multi-file interface.

### Step 3: Update local store for multi-file consistency

The existing `artifact/local/store.go` already implements `artifact.Store` with `[]FileEntry`/`[]FileResult`. Verify it works correctly with planfile file entries (plan + lock file). No interface changes needed — just ensure the implementation handles the planfile use case properly.

### Step 4: Update adapter

Update `planfile/adapter/store.go`:
- **Upload**: Convert the single `io.Reader` blob into a `[]FileEntry` with name `BundlePlanFilename`. If the planfile handler passes lock file data, add a second `FileEntry` with name `BundleLockFilename`. Convert `planfile.Metadata` → `artifact.Metadata` via Custom map. Call `artifact.Store.Upload()`.
- **Download**: Call `artifact.Store.Download()` → receive `[]FileResult`. Find the plan entry by `BundlePlanFilename`, return its `io.ReadCloser`. Convert `artifact.Metadata` → `planfile.Metadata`. Close unused file results.
- **Error wrapping**: Wrap `ErrArtifact*` errors as `ErrPlanfile*` errors.

### Step 5: Delete `planfile/bundle.go`

Delete `pkg/ci/plugins/terraform/planfile/bundle.go` and `pkg/ci/plugins/terraform/planfile/bundle_test.go`. Bundling is now handled internally by each artifact store implementation. Keep the `BundlePlanFilename` and `BundleLockFilename` constants in `planfile/interface.go` — they are used by the adapter for file entry naming.

### Step 6: Remove planfile store implementations

Delete:
- `pkg/ci/plugins/terraform/planfile/local/` (entire directory)
- `pkg/ci/plugins/terraform/planfile/s3/` (entire directory)
- `pkg/ci/plugins/terraform/planfile/github/` (entire directory)

### Step 7: Remove planfile registry

Delete `pkg/ci/plugins/terraform/planfile/registry.go`. The planfile layer no longer has its own store registry.

Update all callers that call `planfile.NewStore()` to use the adapter factory pattern:
- `cmd/terraform/planfile/upload.go` — create an `artifact.Store` and wrap it in the adapter.
- `cmd/terraform/planfile/download.go` — same.
- `cmd/terraform/planfile/show.go` — same.
- `cmd/terraform/planfile/list.go` — same.

### Step 8: Update CLI commands

The CLI commands currently import planfile store registrations via blank imports:
```go
_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/github"
_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/local"
_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/s3"
```

Change to:
```go
_ "github.com/cloudposse/atmos/pkg/ci/artifact/github"
_ "github.com/cloudposse/atmos/pkg/ci/artifact/local"
_ "github.com/cloudposse/atmos/pkg/ci/artifact/s3"
```

Update `getStoreOptions()` in `upload.go` to create `artifact.StoreOptions` and wrap the resulting `artifact.Store` with the adapter.

### Step 9: Update handler planfile store creation

In `pkg/ci/plugins/terraform/handlers.go`, `uploadPlanfile()` and `downloadPlanfile()` call `ctx.CreatePlanfileStore()` and cast to `planfile.Store`. Update to:
- Create an `artifact.Store` from the artifact registry.
- Wrap it with the adapter to get `planfile.Store`.

Update `uploadPlanfile()`: Instead of pre-bundling with `planfile.CreateBundle()`, pass plan and lock file data directly. The adapter converts to `[]FileEntry` and the artifact store handles bundling internally.

Update `downloadPlanfile()`: Instead of calling `planfile.ExtractBundle()`, the adapter returns the plan data directly from `[]FileResult`. Lock file data is returned separately if present.

### Step 10: Add artifact-level error sentinels

Add any missing error sentinels in `errors/errors.go`:
- `ErrArtifactUploadFailed`
- `ErrArtifactDownloadFailed`
- `ErrArtifactDeleteFailed`
- `ErrArtifactListFailed`
- `ErrArtifactStatFailed`
- `ErrArtifactMetadataFailed`
- `ErrArtifactNotFound`

Some of these may already exist. Reuse existing ones where possible.

### Step 11: Update tests

- `artifact/local/store_test.go`: Verify multi-file upload/download with plan + lock entries.
- `artifact/s3/store_test.go`: Port from planfile S3 tests, update for multi-file interface.
- `artifact/github/store_test.go`: Port from planfile GitHub tests, update for multi-file interface.
- `planfile/adapter/store_test.go`: Update for adapter changes (file entry conversion, error wrapping).
- `cmd/terraform/planfile/*_test.go`: Update imports and store creation.
- `handlers_test.go`: Update mock store setup, remove bundle creation/extraction.

### Step 12: Clean up StoreOptions and StoreFactory

Verify that `planfile.StoreOptions` and `planfile.StoreFactory` types are no longer needed. If the adapter handles all conversion, these can be removed. If CLI commands still need them for the `--store` flag, keep `planfile.StoreOptions` as a thin alias or wrapper.

## File Impact Summary

| File | Change |
|------|--------|
| `pkg/ci/artifact/s3/store.go` | **New** — ported from planfile/s3, multi-file interface |
| `pkg/ci/artifact/s3/store_test.go` | **New** — ported from planfile/s3 |
| `pkg/ci/artifact/github/store.go` | **New** — ported from planfile/github, multi-file interface |
| `pkg/ci/artifact/github/store_test.go` | **New** — ported from planfile/github |
| `pkg/ci/artifact/local/store.go` | **Verify** — should work with planfile file entries |
| `pkg/ci/artifact/mock_store.go` | **Regenerate** |
| `pkg/ci/plugins/terraform/planfile/bundle.go` | **Delete** |
| `pkg/ci/plugins/terraform/planfile/bundle_test.go` | **Delete** |
| `pkg/ci/plugins/terraform/planfile/local/` | **Delete** — entire directory |
| `pkg/ci/plugins/terraform/planfile/s3/` | **Delete** — entire directory |
| `pkg/ci/plugins/terraform/planfile/github/` | **Delete** — entire directory |
| `pkg/ci/plugins/terraform/planfile/registry.go` | **Delete** |
| `pkg/ci/plugins/terraform/planfile/interface.go` | **Modify** — keep Store interface, keep Metadata/KeyPattern/KeyContext, add well-known filename constants |
| `pkg/ci/plugins/terraform/planfile/adapter/store.go` | **Modify** — update for file entry conversion, error wrapping |
| `pkg/ci/plugins/terraform/planfile/adapter/store_test.go` | **Modify** — update for adapter changes |
| `pkg/ci/plugins/terraform/handlers.go` | **Modify** — remove bundle creation/extraction, pass files directly |
| `cmd/terraform/planfile/upload.go` | **Modify** — change imports, store creation |
| `cmd/terraform/planfile/download.go` | **Modify** — change imports, remove bundle extraction |
| `cmd/terraform/planfile/show.go` | **Modify** — change imports |
| `cmd/terraform/planfile/list.go` | **Modify** — change imports |
| `errors/errors.go` | **Modify** — add artifact error sentinels if missing |

## Migration Notes

### No backward compatibility concerns

The planfile store implementations are internal packages. No external consumers depend on `planfile.Store` directly. The CLI commands and plugin handlers are the only callers, and they are updated in this PRD.

### Store type names remain the same

The registered store type names (`local`, `s3`, `github-artifacts`) do not change. Configuration in `atmos.yaml` under `terraform.planfiles.stores` continues to work because the adapter maps planfile store options to artifact store options.

### Bundle format changes per backend

- **Local store**: Files stored individually in directories (no tar). This is a format change from the current local planfile store which stores a single blob. Existing local planfiles are not forward-compatible — they need re-upload.
- **S3 store**: Tar archive format is preserved (tar is efficient for S3). Existing S3 planfiles remain compatible.
- **GitHub store**: Zip archive format is preserved (GitHub API requirement). Individual files are stored as zip entries instead of a tar-in-zip. Existing GitHub planfiles are not forward-compatible.

### planfile.Store interface preserved

The `planfile.Store` interface is unchanged. All existing code that depends on `planfile.Store` (handlers, CLI commands) continues to work via the adapter. Only the adapter's internal implementation changes.

## Verification

1. `go build ./...`
2. `go test ./pkg/ci/artifact/...`
3. `go test ./pkg/ci/plugins/terraform/...`
4. `go test ./cmd/terraform/planfile/...`
5. `make lint`
6. Manual: `atmos terraform planfile upload --stack dev --component vpc` with local, S3, and GitHub backends
7. Manual: `atmos terraform planfile download --stack dev --component vpc` with all backends
