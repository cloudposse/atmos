# Planfile Artifact Should Bundle Plan + Lock File as Zip

> Related: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Interfaces](../framework/interfaces.md) | [Planfile Metadata Embed Artifact](planfile-metadata-embed-artifact.md)

## Problem Statement

The planfile artifact currently stores only the raw `.tfplan` binary. The `.terraform.lock.hcl` file — which pins exact provider versions and hashes — is not included. This creates two problems during `terraform apply` with a downloaded plan:

### 1. Provider version mismatch on apply

When `terraform apply` runs with a downloaded plan, Terraform validates that provider versions match. If the runner's `.terraform.lock.hcl` was generated from a different `terraform init` invocation (or is absent), Terraform may refuse the plan or silently use different provider binaries. Bundling the lock file with the plan ensures the exact provider versions that produced the plan are enforced on apply.

### 2. Raw binary storage lacks integrity envelope

Each store currently handles the planfile `io.Reader` differently:
- **Local store**: writes a raw file + sidecar `.metadata.json`, computes MD5/SHA256 on the raw bytes.
- **S3 store**: uploads a raw file + sidecar metadata object.
- **GitHub store**: already zips plan + metadata into an archive.

There is no uniform bundle format. Adding a second file (lock) to the local and S3 stores would require either a second key or changing the storage format. A uniform zip bundle solves this for all stores and makes the SHA256 checksum meaningful — it covers the entire artifact (plan + lock + metadata), not just the plan binary.

### 3. Store interface accepts single io.Reader

The `Store.Upload()` signature is:
```go
Upload(ctx context.Context, key string, data io.Reader, metadata *Metadata) error
```

This accepts a single data stream. To bundle multiple files, the caller must zip them before calling Upload, and the stores must unzip on Download. This keeps the Store interface stable while changing what flows through it.

## Desired State

1. **Zip bundle format**: All planfile artifacts use a zip archive containing:
   ```
   artifact.zip
   ├── plan.tfplan              # Terraform plan binary
   ├── .terraform.lock.hcl     # Provider lock file (if present)
   └── metadata.json           # Planfile metadata
   ```

2. **SHA256 on the zip**: The `metadata.SHA256` field reflects the checksum of the complete zip archive, not the raw plan bytes. This provides integrity verification across the entire bundle.

3. **Backward-compatible download**: If a store contains a raw (non-zip) planfile from before this change, `Download()` should detect it (zip files start with `PK\x03\x04`) and fall back to returning the raw bytes as the plan data with no lock file.

4. **Store interface unchanged**: `Upload()` and `Download()` signatures remain the same. The zip/unzip logic lives in shared helper functions called by the plugin handlers, not inside each store.

## Architecture

### Bundle creation and extraction live in the plugin layer

The handlers (`handlers.go`) are responsible for:
- **Upload**: reading plan + lock files from disk → creating zip → passing zip `io.Reader` to `store.Upload()`
- **Download**: receiving zip `io.Reader` from `store.Download()` → extracting plan + lock → writing both to disk

This keeps stores simple (they store/retrieve opaque blobs) and centralizes the bundle format in one place.

### Shared bundle helpers in `pkg/ci/plugins/terraform/planfile/bundle.go`

```go
// BundleFiles represents the files in a planfile bundle.
type BundleFiles struct {
    Plan     io.Reader  // Required: the .tfplan binary
    LockFile io.Reader  // Optional: .terraform.lock.hcl
    Metadata *Metadata  // Required: planfile metadata
}

// CreateBundle creates a zip archive from the bundle files.
// Returns the zip data and the SHA256 of the zip.
func CreateBundle(files *BundleFiles) (io.Reader, string, error)

// ExtractBundle extracts plan, lock file, and metadata from a zip archive.
// Falls back to treating data as raw plan if it's not a zip.
func ExtractBundle(data io.Reader) (*BundleFiles, error)
```

### GitHub store simplification

The GitHub store's `createArtifactZip()` / `extractPlanFromZip()` already implement a zip bundle. After this change:
- Those functions are deleted.
- The GitHub store receives an already-zipped `io.Reader` from the handler (same as local/S3).
- The GitHub store's `Upload()` wraps the zip in its own artifact-level zip (required by GitHub Actions runtime API), or can be refactored to upload the bundle zip directly.

## Implementation Steps

### Step 1: Create `pkg/ci/plugins/terraform/planfile/bundle.go`

Create `BundleFiles` struct and two functions:

**`CreateBundle(files *BundleFiles) ([]byte, error)`**:
- Create zip archive in memory using `archive/zip`.
- Write `plan.tfplan` entry from `files.Plan`.
- Write `.terraform.lock.hcl` entry from `files.LockFile` (skip if nil).
- Marshal `files.Metadata` as JSON, write `metadata.json` entry.
- Compute SHA256 of the final zip bytes, set `files.Metadata.SHA256`.
- Return zip bytes.

**`ExtractBundle(data io.Reader) (*BundleFiles, error)`**:
- Read all data into memory.
- Check if first 4 bytes are `PK\x03\x04` (zip magic).
- If zip: open as `zip.Reader`, extract `plan.tfplan` (required), `.terraform.lock.hcl` (optional), `metadata.json` (optional).
- If not zip: return raw data as `Plan`, nil `LockFile`, nil `Metadata` (backward compat).

### Step 2: Create `pkg/ci/plugins/terraform/planfile/bundle_test.go`

Table-driven tests:
- Round-trip: create bundle → extract → verify plan, lock, metadata match.
- Bundle without lock file: plan + metadata only, extract succeeds with nil LockFile.
- Bundle with lock file: all three files present.
- Raw planfile fallback: non-zip data returns raw bytes as Plan.
- SHA256 is set on metadata after CreateBundle.
- Empty plan data returns error.
- Corrupted zip returns error.

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
- After resolving `planfilePath`, derive lock file path: `filepath.Join(filepath.Dir(planfilePath), "..", ".terraform.lock.hcl")`. The planfile lives inside the `.terraform` subdirectory (e.g., `components/terraform/vpc/.terraform/plan-dev.tfplan`), so the lock file is one directory up.
- Open lock file if it exists (non-fatal if missing).
- Call `planfile.CreateBundle()` with plan reader, lock reader (or nil), and metadata.
- `CreateBundle` sets `metadata.SHA256` on the zip.
- Remove per-store SHA256 computation (now done in bundle creation).
- Pass zip bytes as `io.Reader` to `store.Upload()`.

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
- Write `bundle.Plan` to `planfilePath`.
- If `bundle.LockFile != nil`, write it to `filepath.Join(filepath.Dir(planfilePath), "..", ".terraform.lock.hcl")`.
- Backward-compatible: raw (non-zip) downloads produce a bundle with only Plan set.

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

- After download, use `ExtractBundle()` to split the zip.
- Write plan to the output path.
- Write lock file alongside the plan (or to `--lockfile` flag if provided).

### Step 7: Remove GitHub store zip logic

- Delete `createArtifactZip()` and `extractPlanFromZip()` from `github/store.go`.
- GitHub `Upload()` now receives zip bytes (the bundle), wraps them in the GitHub Actions artifact zip format.
- GitHub `Download()` extracts the outer GitHub zip, returns the inner bundle zip as `io.Reader` — the handler calls `ExtractBundle()`.
- Update `github/store_test.go` accordingly.

### Step 8: Remove per-store SHA256 computation

- **Local store** (`local/store.go`): Remove SHA256 computation from `Upload()`. Keep MD5 for backward compatibility (planfile-specific checksum). The bundle's SHA256 is set by `CreateBundle()` before reaching the store.
- **GitHub store** (`github/store.go`): Remove SHA256 computation from `Upload()`.
- Stores remain simple: they store/retrieve opaque blobs.

### Step 9: Update adapter store

The adapter wraps `artifact.Store` which handles multi-file bundles natively. Two options:
- **Option A**: Pass the zip bundle as a single file to the artifact store (consistent with local/S3/GitHub).
- **Option B**: Extract bundle files and pass individually to `artifact.Store.Upload()`.

Recommend **Option A** for consistency — the adapter passes the zip as a single `FileEntry`.

### Step 10: Update tests

- `handlers_test.go`: Update `uploadPlanfile`/`downloadPlanfile` tests to verify bundle creation/extraction and lock file handling.
- `local/store_test.go`: Verify stores handle zip data as opaque blobs.
- `github/store_test.go`: Remove `createArtifactZip`/`extractPlanFromZip` tests, update upload/download tests.
- `cmd/terraform/planfile/upload_test.go`: Test `--lockfile` flag.
- Add `bundle_test.go` (Step 2).

## File Impact Summary

| File | Change |
|------|--------|
| `pkg/ci/plugins/terraform/planfile/bundle.go` | **New** — CreateBundle, ExtractBundle, BundleFiles |
| `pkg/ci/plugins/terraform/planfile/bundle_test.go` | **New** — bundle round-trip tests |
| `pkg/ci/plugins/terraform/handlers.go` | **Modify** — uploadPlanfile/downloadPlanfile use bundle |
| `pkg/ci/plugins/terraform/handlers_test.go` | **Modify** — update upload/download tests |
| `cmd/terraform/planfile/upload.go` | **Modify** — add --lockfile flag, create bundle |
| `cmd/terraform/planfile/download.go` | **Modify** — extract bundle, write lock file |
| `pkg/ci/plugins/terraform/planfile/github/store.go` | **Modify** — remove createArtifactZip/extractPlanFromZip |
| `pkg/ci/plugins/terraform/planfile/github/store_test.go` | **Modify** — remove zip tests, update upload/download |
| `pkg/ci/plugins/terraform/planfile/local/store.go` | **Modify** — remove SHA256 computation |
| `pkg/ci/plugins/terraform/planfile/adapter/store.go` | **Modify** — pass zip as single FileEntry |

## Verification

1. `go build ./...`
2. `go test ./pkg/ci/plugins/terraform/planfile/...`
3. `go test ./pkg/ci/plugins/terraform/...`
4. `go test ./cmd/terraform/planfile/...`
5. `make lint`
6. Manual: `atmos terraform planfile upload --stack dev --component vpc` → verify zip contains plan + lock + metadata
7. Manual: `atmos terraform planfile download --stack dev --component vpc` → verify plan + lock extracted
