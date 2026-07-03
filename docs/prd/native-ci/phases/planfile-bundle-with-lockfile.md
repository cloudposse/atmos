# Planfile Artifact Should Bundle Plan + Lock File as Tar Archive — SHIPPED

> Related: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Interfaces](../framework/interfaces.md) | [Planfile Metadata Embed Artifact](planfile-metadata-embed-artifact.md)

## Status: SHIPPED

All steps implemented: shared tar helpers in `artifact/tar.go`, `planfile.Store` aligned to multi-file interface, well-known filename constants, CLI `--lockfile` flag, default key pattern updated to `.tfplan.tar`.

## Problem Statement

The planfile artifact currently stores only the raw `.tfplan` binary. The `.terraform.lock.hcl` file — which pins exact provider versions and hashes — is not included. This creates two problems during `terraform apply` with a downloaded plan:

### 1. Provider version mismatch on apply

When `terraform apply` runs with a downloaded plan, Terraform validates that provider versions match. If the runner's `.terraform.lock.hcl` was generated from a different `terraform init` invocation (or is absent), Terraform may refuse the plan or silently use different provider binaries. Bundling the lock file with the plan ensures the exact provider versions that produced the plan are enforced on apply.

### 2. Raw binary storage lacks integrity envelope

Each store currently handles the planfile `io.Reader` differently:
- **Local store**: writes a raw file + sidecar `.metadata.json`, computes MD5/SHA256 on the raw bytes.
- **S3 store**: uploads a raw file + sidecar metadata object.
- **GitHub store**: already zips plan + metadata into an archive.

There is no uniform bundle format. Adding a second file (lock) to the local and S3 stores would require either a second key or changing the storage format. A uniform tar bundle solves this for all stores and makes the SHA256 checksum meaningful — it covers the entire artifact (plan + lock), not just the plan binary.

### 3. Store interface accepts single io.Reader

The `Store.Upload()` signature is:
```go
Upload(ctx context.Context, key string, data io.Reader, metadata *Metadata) error
```

This accepts a single data stream. To bundle multiple files, the caller must tar them before calling Upload, and the receiver must untar on Download. This keeps the Store interface stable while changing what flows through it.

## Desired State

1. **Tar bundle format**: All planfile artifacts use a tar archive containing:
   ```
   bundle.tar
   ├── plan.tfplan              # Terraform plan binary
   └── .terraform.lock.hcl     # Provider lock file (if present)
   ```
   Metadata is **not** included in the tar. It is stored as a sidecar by each store backend (same as today).

2. **SHA256 on the tar**: The `metadata.SHA256` field reflects the checksum of the complete tar archive. It is computed by `CreateBundle()` and returned separately for the caller to set on metadata, not stored inside the tar (avoids circular dependency).

3. **No backward compatibility**: Old raw (non-tar) planfiles are not supported. All uploads use the new bundle format. Downloads expect tar bundles.

4. **Store interface unchanged**: `Upload()` and `Download()` signatures remain the same. The tar/untar logic lives in shared helper functions called by the plugin handlers, not inside each store.

## Architecture

### Bundle creation and extraction live in the plugin layer

The handlers (`handlers.go`) are responsible for:
- **Upload**: reading plan + lock files from disk → creating tar → passing tar `io.Reader` to `store.Upload()`
- **Download**: receiving tar `io.Reader` from `store.Download()` → extracting plan + lock → writing both to disk

This keeps stores simple (they store/retrieve opaque blobs) and centralizes the bundle format in one place.

### Shared bundle helpers in `pkg/ci/plugins/terraform/planfile/bundle.go`

```go
// CreateBundle creates a tar archive containing the plan and optional lock file.
// Returns the tar bytes and the SHA256 hex string of the tar.
// The caller is responsible for setting metadata.SHA256 from the returned hash.
func CreateBundle(plan io.Reader, lockFile io.Reader) ([]byte, string, error)

// ExtractBundle extracts plan and lock file from a tar archive.
// Returns the plan data, lock file data (nil if not present), and error.
func ExtractBundle(data io.Reader) (plan []byte, lockFile []byte, err error)
```

### GitHub store simplification

The GitHub store's `createArtifactZip()` / `extractPlanFromZip()` already implement a zip bundle. After this change:
- `createArtifactZip()` is simplified — it no longer needs to add `plan.tfplan` or `metadata.json` entries. It wraps the incoming tar bundle as a single entry in the outer artifact zip (the GitHub Actions runtime API requires zip).
- `extractPlanFromZip()` is simplified — it extracts the single entry from the outer artifact zip and returns it as-is. The handler calls `ExtractBundle()` on the inner tar.
- The GitHub store treats the bundle as an opaque blob, same as local/S3.

## Implementation Steps

### Step 1: Create `pkg/ci/plugins/terraform/planfile/bundle.go`

Two functions with simple signatures:

**`CreateBundle(plan io.Reader, lockFile io.Reader) ([]byte, string, error)`**:
- Create tar archive in memory using `archive/tar`.
- Write `plan.tfplan` entry from `plan` reader (required — error if nil).
- Write `.terraform.lock.hcl` entry from `lockFile` reader (skip if nil).
- Compute SHA256 of the final tar bytes.
- Return tar bytes and SHA256 hex string. Caller sets `metadata.SHA256` explicitly.

**`ExtractBundle(data io.Reader) (plan []byte, lockFile []byte, err error)`**:
- Read tar archive using `archive/tar`.
- Iterate entries: extract `plan.tfplan` (required — error if missing), `.terraform.lock.hcl` (optional).
- Return plan bytes and lock file bytes (nil if not present).

### Step 2: Create `pkg/ci/plugins/terraform/planfile/bundle_test.go`

Table-driven tests:
- Round-trip: create bundle → extract → verify plan and lock match.
- Bundle without lock file: plan only, extract succeeds with nil lockFile.
- Bundle with lock file: both files present.
- SHA256 is returned and non-empty after CreateBundle.
- Nil plan reader returns error.
- Corrupted tar returns error.
- Missing `plan.tfplan` entry returns error.

### Step 3: Update `uploadPlanfile()` in `handlers.go`

Current flow:
```
open planfile → store.Upload(key, planReader, metadata)
```

New flow:
```
open planfile → open lockfile (from component working dir) → CreateBundle → store.Upload(key, bundleReader, metadata)
```

Changes:
- After resolving `planfilePath`, derive lock file path: `filepath.Join(filepath.Dir(planfilePath), ".terraform.lock.hcl")`. The planfile lives in the component working directory (e.g., `components/terraform/vpc/plat-ue2-dev-vpc.planfile`), and `.terraform.lock.hcl` is in the same directory.
- Open lock file if it exists (non-fatal if missing).
- Call `planfile.CreateBundle()` with plan reader and lock reader (or nil).
- Set `metadata.SHA256` from the returned hash string.
- Pass tar bytes as `io.Reader` to `store.Upload()`.

### Step 4: Update `downloadPlanfile()` in `handlers.go`

Current flow:
```
store.Download(key) → io.Copy to planfile path
```

New flow:
```
store.Download(key) → ExtractBundle → write plan to planfile path → write lock to component dir
```

Changes:
- Call `planfile.ExtractBundle()` on the downloaded reader.
- Write plan bytes to `planfilePath`.
- If lockFile bytes are non-nil, write to `filepath.Join(filepath.Dir(planfilePath), ".terraform.lock.hcl")`.

### Step 5: Update CLI upload command `cmd/terraform/planfile/upload.go`

The `runUpload` function opens a single planfile. Update to:
- Accept optional `--lockfile` flag (or auto-detect from `--planfile` path).
- If `--planfile` is provided, look for lock file relative to it.
- If `--component`/`--stack` derive the path, use the component working directory.
- Create bundle before calling `store.Upload()`.

Add flag:
```go
flags.WithStringFlag("lockfile", "", "", "Path to .terraform.lock.hcl (default: auto-detected from planfile path)"),
flags.WithEnvVars("lockfile", "ATMOS_PLANFILE_LOCKFILE"),
```

### Step 6: Update CLI download command `cmd/terraform/planfile/download.go`

- After download, use `ExtractBundle()` to split the tar.
- Write plan to the output path.
- Write lock file alongside the plan (or to `--lockfile` flag if provided).

### Step 7: Simplify GitHub store zip logic

- Simplify `createArtifactZip()` in `github/store.go` — wrap the tar bundle as a single `bundle.tar` entry in the outer artifact zip. Remove plan/metadata entry logic.
- Simplify `extractPlanFromZip()` — extract the single `bundle.tar` entry from the outer artifact zip and return its bytes. The handler calls `ExtractBundle()` on the result.
- GitHub `Upload()` receives tar bundle bytes, wraps in outer zip (GitHub API requirement), uploads via runtime API.
- GitHub `Download()` extracts outer zip, returns inner tar bundle bytes — handler extracts plan + lock.
- Update `github/store_test.go` accordingly.

### Step 8: Update default key pattern

Change the default key pattern in `DefaultKeyPattern()` (`interface.go`) from:
```
{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan
```
to:
```
{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan.tar
```

The `.tfplan.tar` extension makes the bundle format explicit in the storage key. Update all tests that assert on the default pattern or generated keys.

### Step 9: Remove per-store checksum computation

- **Local store** (`local/store.go`): Remove both MD5 and SHA256 computation from `Upload()`. The bundle's SHA256 is set by the caller via `CreateBundle()` return value. Stores are now simple blob storage.
- **GitHub store** (`github/store.go`): Remove SHA256 computation from `Upload()`.
- Stores remain simple: they store/retrieve opaque blobs.

### Step 9b: Remove MD5 field from `planfile.Metadata`

- Delete the `MD5 string` field from `Metadata` in `interface.go`.
- Remove MD5 from all metadata construction sites, tests, and display output (`list.go`, `show.go`).
- Remove `customKeyMD5` from adapter conversion if present.
- SHA256 (computed on the tar bundle) replaces MD5 as the sole integrity checksum.

### Step 10: Update adapter store

The adapter wraps `artifact.Store` which handles multi-file bundles natively. Two options:
- **Option A**: Pass the tar bundle as a single file to the artifact store (consistent with local/S3/GitHub).
- **Option B**: Extract bundle files and pass individually to `artifact.Store.Upload()`.

Recommend **Option A** for consistency — the adapter passes the tar as a single `FileEntry`.

### Step 11: Update tests

- `handlers_test.go`: Update `uploadPlanfile`/`downloadPlanfile` tests to verify bundle creation/extraction and lock file handling.
- `local/store_test.go`: Verify stores handle tar data as opaque blobs.
- `github/store_test.go`: Simplify `createArtifactZip`/`extractPlanFromZip` tests, update upload/download tests.
- `cmd/terraform/planfile/upload_test.go`: Test `--lockfile` flag.
- Add `bundle_test.go` (Step 2).

## File Impact Summary

| File | Change |
|------|--------|
| `pkg/ci/plugins/terraform/planfile/bundle.go` | **New** — CreateBundle, ExtractBundle |
| `pkg/ci/plugins/terraform/planfile/bundle_test.go` | **New** — bundle round-trip tests |
| `pkg/ci/plugins/terraform/handlers.go` | **Modify** — uploadPlanfile/downloadPlanfile use bundle |
| `pkg/ci/plugins/terraform/handlers_test.go` | **Modify** — update upload/download tests |
| `cmd/terraform/planfile/upload.go` | **Modify** — add --lockfile flag, create bundle |
| `cmd/terraform/planfile/download.go` | **Modify** — extract bundle, write lock file |
| `pkg/ci/plugins/terraform/planfile/github/store.go` | **Modify** — simplify createArtifactZip/extractPlanFromZip |
| `pkg/ci/plugins/terraform/planfile/github/store_test.go` | **Modify** — update zip tests for tar-in-zip |
| `pkg/ci/plugins/terraform/planfile/local/store.go` | **Modify** — remove MD5/SHA256 computation |
| `pkg/ci/plugins/terraform/planfile/interface.go` | **Modify** — remove MD5 field, update default key pattern to `.tfplan.tar` |
| `pkg/ci/plugins/terraform/planfile/adapter/store.go` | **Modify** — pass tar as single FileEntry |

## Verification

1. `go build ./...`
2. `go test ./pkg/ci/plugins/terraform/planfile/...`
3. `go test ./pkg/ci/plugins/terraform/...`
4. `go test ./cmd/terraform/planfile/...`
5. `make lint`
6. Manual: `atmos terraform planfile upload --stack dev --component vpc` → verify tar contains plan + lock
7. Manual: `atmos terraform planfile download --stack dev --component vpc` → verify plan + lock extracted
