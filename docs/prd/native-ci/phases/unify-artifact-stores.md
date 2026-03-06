# Unify Planfile Stores into Artifact Store Layer

> Related: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Interfaces](../framework/interfaces.md) | [Planfile Metadata Embed Artifact](planfile-metadata-embed-artifact.md) | [Planfile Bundle with Lockfile](planfile-bundle-with-lockfile.md)

## Prerequisites

This PRD **requires** the following PRDs to be implemented first:

1. **[Planfile Bundle with Lockfile](planfile-bundle-with-lockfile.md)** — Establishes the tar bundle format for plan + lock files and the `CreateBundle`/`ExtractBundle` functions.
2. **[Planfile Metadata Embed Artifact](planfile-metadata-embed-artifact.md)** — Makes `planfile.Metadata` embed `artifact.Metadata`, enabling unified metadata handling.

## Scope

This PRD has two dimensions:

1. **Interface alignment** — Changes `planfile.Store` from single-blob to multi-file interface. This affects **all** store implementations (adapter, S3, GitHub).
2. **Artifact-layer porting** — Moves store implementations from `planfile/` to `artifact/`. This PRD ports **local only**. S3 and GitHub are ported in follow-up PRDs.

All stores are updated to implement `artifact.Store` and register via `artifact.Register()`. The planfile registry is deleted entirely. The adapter wraps any `artifact.Store` → `planfile.Store`.

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

### 5. Interface divergence prevents reuse

The `planfile.Store` interface accepts `planfile.Metadata` in Upload and returns it from Download. The `artifact.Store` interface accepts `artifact.Metadata`. Since `planfile.Metadata` embeds `artifact.Metadata`, the planfile layer only needs to serialize/deserialize the extra fields — which JSON handles automatically via the embedded struct. The store itself never interprets the metadata; it just stores and retrieves it as JSON.

Additionally, `planfile.Store` uses a single-blob interface (`io.Reader`) while `artifact.Store` uses a multi-file interface (`[]FileEntry`). This forces the adapter to wrap/unwrap files, adding unnecessary complexity.

## Desired State

1. **All backends implement `artifact.Store` and register via `artifact.Register()`.** Local lives in `pkg/ci/artifact/local/`. S3 and GitHub temporarily remain in `planfile/s3/` and `planfile/github/` but implement `artifact.Store` (ported to `artifact/` in follow-up PRDs).

2. **Planfile local store is removed.** `planfile/local/` is deleted. The local backend is served by `artifact/local/store.go` via the adapter.

3. **Single registry.** `artifact.Register()` is the only store registry. The planfile registry is deleted entirely. Store type names (`local`, `s3`, `github-artifacts`) are shared.

4. **Bundling moves into artifact stores.** Each artifact store implementation handles persistence format internally — callers pass `[]FileEntry` and receive `[]FileResult`. The planfile layer no longer pre-bundles files into tar archives. `planfile/bundle.go` is deleted. Shared tar helpers move to `pkg/ci/artifact/tar.go`.

5. **planfile.Store interface aligns with artifact.Store.** `planfile.Store` changes from single-blob (`io.Reader`) to multi-file (`[]FileEntry`/`[]FileResult`), and `List` changes from prefix-string to `Query`-based. Types are aliases from artifact: `type FileEntry = artifact.FileEntry`, `type FileResult = artifact.FileResult`, `type Query = artifact.Query`. The adapter becomes a thin metadata-conversion wrapper.

6. **Planfile config stays separate.** Planfile store configuration (`atmos.yaml` section, `--store` flag, env vars) remains in the planfile layer. `CreatePlanfileStore()` internally creates an `artifact.Store` from the artifact registry, wraps it with the adapter, and returns `planfile.Store`. Handlers are unchanged.

7. **Both error sentinel types are kept.** Artifact stores use `ErrArtifact*` sentinels. The planfile adapter wraps artifact errors as `ErrPlanfile*` for planfile-layer consumers.

## Architecture

### Current architecture

```
planfile.Store (single-blob interface: io.Reader)
├── planfile/local/store.go     → implements planfile.Store
├── planfile/s3/store.go        → implements planfile.Store
├── planfile/github/store.go    → implements planfile.Store
└── planfile/adapter/store.go   → wraps artifact.Store → planfile.Store

artifact.Store (multi-file interface: []FileEntry)
└── artifact/local/store.go     → implements artifact.Store
```

### Target architecture (this PRD)

```
artifact.Store (multi-file interface: []FileEntry)
├── artifact/local/store.go     → implements artifact.Store (exists, unchanged)
├── planfile/s3/store.go        → implements artifact.Store (updated, stays in planfile dir)
└── planfile/github/store.go    → implements artifact.Store (updated, stays in planfile dir)

artifact.Register() — single registry for all backends
├── "local"             → artifact/local
├── "s3"                → planfile/s3 (temporarily)
└── "github-artifacts"  → planfile/github (temporarily)

planfile.Store (multi-file interface: []FileEntry, aligned with artifact.Store)
└── planfile/adapter/store.go   → thin wrapper: artifact.Store → planfile.Store (metadata conversion only)
```

### Target architecture (after follow-up PRDs — S3 + GitHub ported)

```
artifact.Store (multi-file interface: []FileEntry)
├── artifact/local/store.go     → implements artifact.Store
├── artifact/s3/store.go        → implements artifact.Store (moved from planfile/s3)
└── artifact/github/store.go    → implements artifact.Store (moved from planfile/github)

planfile.Store (multi-file interface: []FileEntry, aligned with artifact.Store)
└── planfile/adapter/store.go   → thin wrapper: artifact.Store → planfile.Store (metadata conversion only)
```

### Interface alignment

The `planfile.Store` interface changes to align with `artifact.Store`:

**Current planfile.Store:**
```go
Upload(ctx context.Context, key string, data io.Reader, metadata *Metadata) error
Download(ctx context.Context, key string) (io.ReadCloser, *Metadata, error)
List(ctx context.Context, prefix string) ([]PlanfileInfo, error)
```

**Target planfile.Store:**
```go
Upload(ctx context.Context, name string, files []FileEntry, metadata *Metadata) error
Download(ctx context.Context, name string) ([]FileResult, *Metadata, error)
List(ctx context.Context, query Query) ([]PlanfileInfo, error)
```

Types are aliases from the artifact package:
```go
type FileEntry = artifact.FileEntry
type FileResult = artifact.FileResult
type Query = artifact.Query
```

The `artifact.Store` interface is **unchanged**:
```go
Upload(ctx context.Context, name string, files []FileEntry, metadata *Metadata) error
Download(ctx context.Context, name string) ([]FileResult, *Metadata, error)
List(ctx context.Context, query Query) ([]ArtifactInfo, error)
```

### Adapter simplification

With aligned interfaces, the adapter becomes a thin metadata-conversion layer:

- **Upload**: Convert `planfile.Metadata` → `artifact.Metadata` via Custom map. Pass `[]FileEntry` through (type aliases, no conversion needed). Call `artifact.Store.Upload()`.
- **Download**: Call `artifact.Store.Download()`. Convert `artifact.Metadata` → `planfile.Metadata`. Pass `[]FileResult` through. Return.
- **List**: Pass `Query` through. Call `artifact.Store.List()`. Convert `[]artifact.ArtifactInfo` → `[]planfile.PlanfileInfo`. Return.
- **Error wrapping**: Wrap `ErrArtifact*` errors as `ErrPlanfile*` errors.

No file wrapping/unwrapping, no bundle logic — just metadata and type conversion.

### Well-known filenames

Planfile callers use well-known filenames when constructing `FileEntry` items:

```go
// planfile package constants
const (
    PlanFilename = "plan.tfplan"
    LockFilename = ".terraform.lock.hcl"
)
```

Upload: handler creates `[]FileEntry{{Name: PlanFilename, Data: planReader}, {Name: LockFilename, Data: lockReader}}`.

Download: handler receives `[]FileResult`, finds entries by `Name` to get plan and lock file data.

### Tar helpers

Shared tar archive helpers live in `pkg/ci/artifact/tar.go`:
- `CreateTarArchive(files []FileEntry) (io.Reader, error)` — creates a tar archive from file entries.
- `ExtractTarArchive(data io.Reader) ([]FileResult, error)` — extracts file entries from a tar archive.

S3 store uses these to bundle `[]FileEntry` into a single S3 object and unbundle on download. GitHub store uses zip (GitHub API requirement) with its own helpers.

### Factory pattern

`CreatePlanfileStore()` internally:
1. Reads planfile-specific config (store type, options from `atmos.yaml` / env vars / CLI flags).
2. Maps config to `artifact.StoreOptions`.
3. Calls `artifact.NewStore(opts)` to create the backend (all stores register via `artifact.Register()`).
4. Wraps with `adapter.NewStore(backend)` to get `planfile.Store`.
5. Returns `planfile.Store` to the handler.

Handlers call `ctx.CreatePlanfileStore()` unchanged. Planfile config stays separate from any future artifact config.

### Metadata handling

Stores are metadata-agnostic. They serialize whatever metadata struct is passed to them as JSON, and deserialize it on download. The `artifact.Metadata` struct remains the common base. The planfile adapter converts `planfile.Metadata` ↔ `artifact.Metadata` using the Custom map for planfile-specific fields (same as today). Metadata is always stored in artifact format (planfile-specific fields in the Custom map).

### Error handling

Both error sentinel types are preserved:

- **Artifact stores** use `ErrArtifact*` sentinels (`ErrArtifactUploadFailed`, `ErrArtifactDownloadFailed`, etc.).
- **Planfile adapter** wraps artifact errors as `ErrPlanfile*` for planfile-layer consumers (`ErrPlanfileUploadFailed`, `ErrPlanfileDownloadFailed`, etc.).

This preserves backward compatibility for existing error checks in CLI commands and plugin handlers.

## Implementation Steps

### Step 1: Create shared tar helpers in `pkg/ci/artifact/tar.go`

Create `pkg/ci/artifact/tar.go` with reusable tar bundle/extract functions:
- `CreateTarArchive(files []FileEntry) ([]byte, error)` — creates a tar archive from file entries.
- `ExtractTarArchive(data io.Reader) ([]FileResult, error)` — extracts file entries from a tar archive.

These are used by S3 store (and potentially other stores that need single-object bundling).

Add `pkg/ci/artifact/tar_test.go` with round-trip tests.

### Step 2: Align `planfile.Store` interface with `artifact.Store`

Update `planfile/interface.go`:

```go
// Type aliases from artifact package
type FileEntry = artifact.FileEntry
type FileResult = artifact.FileResult
type Query = artifact.Query

// Before
Upload(ctx context.Context, key string, data io.Reader, metadata *Metadata) error
Download(ctx context.Context, key string) (io.ReadCloser, *Metadata, error)
List(ctx context.Context, prefix string) ([]PlanfileInfo, error)

// After
Upload(ctx context.Context, name string, files []FileEntry, metadata *Metadata) error
Download(ctx context.Context, name string) ([]FileResult, *Metadata, error)
List(ctx context.Context, query Query) ([]PlanfileInfo, error)
```

Add well-known filename constants:
```go
const (
    PlanFilename = "plan.tfplan"
    LockFilename = ".terraform.lock.hcl"
)
```

Regenerate `mock_store.go` if it exists.

### Step 3: Update S3 store to implement `artifact.Store`

Update `planfile/s3/store.go`:
- Change the interface from `planfile.Store` to `artifact.Store`.
- Replace `planfile.Metadata` with `artifact.Metadata` in all signatures.
- Replace `planfile.StoreOptions` with `artifact.StoreOptions`.
- Replace `planfile.PlanfileInfo` with `artifact.ArtifactInfo`.
- Replace `errUtils.ErrPlanfile*` with `errUtils.ErrArtifact*` error sentinels.
- Change `planfile.Register` to `artifact.Register`.
- **Upload**: Accept `[]FileEntry`, use `artifact.CreateTarArchive()` to create a tar, upload as single S3 object + metadata sidecar.
- **Download**: Download tar from S3, use `artifact.ExtractTarArchive()` to get `[]FileResult`, return alongside metadata.
- **List**: Accept `Query`, convert to prefix-based S3 listing internally.
- The S3 logic (PutObject, GetObject, sidecar metadata) remains identical.

Port `planfile/s3/store_test.go` to use `artifact.Store` interface in assertions.

### Step 4: Update GitHub store to implement `artifact.Store`

Update `planfile/github/store.go`:
- Change the interface from `planfile.Store` to `artifact.Store`.
- Replace metadata types and error sentinels (same as S3).
- Change `planfile.Register` to `artifact.Register`.
- **Upload**: Accept `[]FileEntry`, create zip entries for each file + metadata sidecar, upload via GitHub Actions artifact API.
- **Download**: Download zip, extract `[]FileResult` entries + metadata, return.
- **List**: Accept `Query`, implement based on GitHub artifact listing.
- The GitHub API logic (runtime uploader, JWT parsing) remains identical.

Port `planfile/github/store_test.go` to use `artifact.Store` interface in assertions.

### Step 5: Update adapter for aligned interfaces

Simplify `planfile/adapter/store.go`:
- **Upload**: Pass `[]FileEntry` through (type aliases). Convert `planfile.Metadata` → `artifact.Metadata` via Custom map. Call `artifact.Store.Upload()`.
- **Download**: Call `artifact.Store.Download()`. Pass `[]FileResult` through. Convert `artifact.Metadata` → `planfile.Metadata`. Return.
- **List**: Pass `Query` through. Convert `[]artifact.ArtifactInfo` → `[]planfile.PlanfileInfo`. Return.
- **Error wrapping**: Wrap `ErrArtifact*` errors as `ErrPlanfile*` errors.

Remove all bundle/tar wrapping logic from the adapter.

### Step 6: Delete `planfile/bundle.go`

Delete `pkg/ci/plugins/terraform/planfile/bundle.go` and `pkg/ci/plugins/terraform/planfile/bundle_test.go`. Bundling responsibility has moved into store implementations (S3 uses shared tar helpers, GitHub uses zip, local uses filesystem directories). Well-known filename constants are in `planfile/interface.go`.

### Step 7: Delete planfile local store

Delete `pkg/ci/plugins/terraform/planfile/local/` (entire directory). The local backend is now served by `artifact/local/store.go` via the adapter.

### Step 8: Delete planfile registry

Delete `pkg/ci/plugins/terraform/planfile/registry.go`. All stores now register via `artifact.Register()`. The planfile layer no longer has its own store registry.

### Step 9: Update `CreatePlanfileStore()` factory

Update the factory (in executor or wherever `CreatePlanfileStore` is defined):
1. Read planfile store config (type, options).
2. Map config to `artifact.StoreOptions`.
3. Call `artifact.NewStore(opts)` — all stores register via `artifact.Register()`.
4. Wrap with `adapter.NewStore(backend)` to get `planfile.Store`.
5. Return `planfile.Store`.

No conditional routing — all stores go through the artifact registry.

### Step 10: Update handlers

In `pkg/ci/plugins/terraform/handlers.go`:

Update `uploadPlanfile()`: Instead of pre-bundling with `planfile.CreateBundle()`, pass plan and lock file as separate `FileEntry` items to `planfile.Store.Upload()`.

Update `downloadPlanfile()`: Instead of calling `planfile.ExtractBundle()`, receive `[]FileResult` from `planfile.Store.Download()`. Find plan by `PlanFilename`, find lock by `LockFilename`. Write each to the appropriate output path.

### Step 11: Update CLI commands

Update `cmd/terraform/planfile/upload.go`:
- Change blank imports to artifact store registrations:
  ```go
  _ "github.com/cloudposse/atmos/pkg/ci/artifact/local"
  _ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/s3"
  _ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/github"
  ```
- Update `runUpload()` to pass plan + lock as `[]FileEntry` instead of pre-bundled tar.

Update `cmd/terraform/planfile/download.go`:
- Same import changes.
- Update `downloadToFile()` to handle `[]FileResult` instead of extracting bundles.

Update `cmd/terraform/planfile/list.go`:
- CLI keeps accepting prefix string from user.
- Internally convert prefix to `Query` before calling `planfile.Store.List()`.

Update `cmd/terraform/planfile/show.go`:
- Same import changes.

### Step 12: Add artifact-level error sentinels

Add any missing error sentinels in `errors/errors.go`:
- `ErrArtifactUploadFailed`
- `ErrArtifactDownloadFailed`
- `ErrArtifactDeleteFailed`
- `ErrArtifactListFailed`
- `ErrArtifactMetadataFailed`
- `ErrArtifactNotFound`

Some of these may already exist. Reuse existing ones where possible.

### Step 13: Update tests

- `artifact/tar_test.go`: Round-trip tests for tar helpers.
- `planfile/adapter/store_test.go`: Update for simplified adapter (metadata conversion, error wrapping).
- `planfile/s3/store_test.go`: Update for `artifact.Store` interface, multi-file.
- `planfile/github/store_test.go`: Update for `artifact.Store` interface, multi-file.
- `cmd/terraform/planfile/*_test.go`: Update imports and store creation.
- `handlers_test.go`: Update for multi-file upload/download, remove bundle creation/extraction.
- `artifact/local/store_test.go`: Add test cases for planfile-style file entries (plan + lock).

### Step 14: Clean up StoreOptions and StoreFactory

Remove `planfile.StoreOptions` and `planfile.StoreFactory` types — they duplicate `artifact.StoreOptions` and `artifact.StoreFactory`. All store creation goes through the artifact registry.

## File Impact Summary

| File | Change |
|------|--------|
| `pkg/ci/artifact/tar.go` | **New** — shared tar archive helpers |
| `pkg/ci/artifact/tar_test.go` | **New** — tar helper tests |
| `pkg/ci/plugins/terraform/planfile/interface.go` | **Modify** — align Store interface to multi-file, add type aliases, add well-known filename constants |
| `pkg/ci/plugins/terraform/planfile/adapter/store.go` | **Modify** — simplify to metadata-only conversion |
| `pkg/ci/plugins/terraform/planfile/adapter/store_test.go` | **Modify** — update for simplified adapter |
| `pkg/ci/plugins/terraform/planfile/s3/store.go` | **Modify** — implement `artifact.Store`, register via `artifact.Register()`, internal tar bundling |
| `pkg/ci/plugins/terraform/planfile/s3/store_test.go` | **Modify** — update for `artifact.Store` interface |
| `pkg/ci/plugins/terraform/planfile/github/store.go` | **Modify** — implement `artifact.Store`, register via `artifact.Register()`, internal zip bundling |
| `pkg/ci/plugins/terraform/planfile/github/store_test.go` | **Modify** — update for `artifact.Store` interface |
| `pkg/ci/plugins/terraform/planfile/bundle.go` | **Delete** |
| `pkg/ci/plugins/terraform/planfile/bundle_test.go` | **Delete** |
| `pkg/ci/plugins/terraform/planfile/local/` | **Delete** — entire directory |
| `pkg/ci/plugins/terraform/planfile/registry.go` | **Delete** |
| `pkg/ci/plugins/terraform/handlers.go` | **Modify** — pass files as FileEntry, handle FileResult on download |
| `pkg/ci/artifact/local/store.go` | **Verify** — should work with planfile file entries |
| `pkg/ci/artifact/mock_store.go` | **Regenerate** if needed |
| `cmd/terraform/planfile/upload.go` | **Modify** — change imports, pass FileEntry items |
| `cmd/terraform/planfile/download.go` | **Modify** — change imports, handle FileResult |
| `cmd/terraform/planfile/show.go` | **Modify** — change imports |
| `cmd/terraform/planfile/list.go` | **Modify** — convert prefix to Query internally |
| `errors/errors.go` | **Modify** — add artifact error sentinels if missing |

## Follow-up PRDs

These are **out of scope** for this PRD and will be handled separately:

1. **Move S3 store to `pkg/ci/artifact/s3/`** — Move S3 implementation from `planfile/s3/` to `artifact/s3/`. The store already implements `artifact.Store` and registers via `artifact.Register()` (done in this PRD). The follow-up just moves the code to its proper package.

2. **Move GitHub store to `pkg/ci/artifact/github/`** — Move GitHub implementation from `planfile/github/` to `artifact/github/`. Same as S3 — already implements `artifact.Store`, just needs to move.

After both follow-up PRDs, all backends live in `pkg/ci/artifact/`, and `planfile/adapter/` is the only planfile-layer store code.

## Migration Notes

### No backward compatibility concerns

The planfile store implementations are internal packages. No external consumers depend on `planfile.Store` directly. The CLI commands and plugin handlers are the only callers, and they are updated in this PRD.

### Store type names remain the same

The registered store type names (`local`, `s3`, `github-artifacts`) do not change. Configuration in `atmos.yaml` under `terraform.planfiles.stores` continues to work.

### Local store format change

The local backend changes from storing a single tar blob to storing individual files in a directory. Existing local planfiles are not forward-compatible — they need re-upload. This is acceptable because:
- Planfiles are ephemeral (tied to a specific commit SHA).
- Local storage is primarily for development/testing.
- The feature is not yet released to users.

### S3 and GitHub stores updated but not moved

S3 and GitHub stores are updated to implement `artifact.Store` and register via `artifact.Register()`, but their code remains in `planfile/s3/` and `planfile/github/` until follow-up PRDs move them. This is a temporary state — the code works correctly from the artifact registry regardless of package location.

## Verification

1. `go build ./...`
2. `go test ./pkg/ci/artifact/...`
3. `go test ./pkg/ci/plugins/terraform/...`
4. `go test ./cmd/terraform/planfile/...`
5. `make lint`
6. Manual: `atmos terraform planfile upload --stack dev --component vpc` with local backend
7. Manual: `atmos terraform planfile download --stack dev --component vpc` with local backend
8. Manual: Verify S3 and GitHub backends still work (updated to artifact.Store but in planfile layer)
