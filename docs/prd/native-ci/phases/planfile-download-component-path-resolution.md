# Planfile Download: Component Path Resolution & Integrity Check — SHIPPED

> Related: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Artifact Storage](../framework/artifact-storage.md) | [Hooks Integration](../framework/hooks-integration.md)

## Status: SHIPPED

All steps implemented: SHA256 verification in `BundledStore.Download()`, CLI download resolves component path via `ProcessStacks()`, shared `WritePlanfileResults()` helper used by both CLI and CI hook handler.

## Problem Statement

`atmos terraform planfile download` writes files to the current working directory instead of the component's terraform directory. The planfile store correctly returns `[]FileResult` with metadata, but callers (CLI command and CI hook handler) do not resolve the component directory from stack configuration. Additionally, the artifact layer does not verify SHA256 checksums on download, so corrupted or tampered artifacts are silently accepted.

### Current Behavior

```bash
$ cd /project
$ atmos terraform planfile download mycomponent -s prod
✓ Downloaded planfile from local: prod/mycomponent/abc123.tfplan.tar -> plan.tfplan
```

Files are written to CWD:
```
/project/plan.tfplan              # wrong — should be in component dir
/project/.terraform.lock.hcl      # wrong — should be in component dir
```

### Expected Behavior

```bash
$ atmos terraform planfile download mycomponent -s prod
✓ Downloaded planfile from local: prod/mycomponent/abc123.tfplan.tar
  -> components/terraform/mycomponent/mycomponent-prod-abc123.tfplan
  -> components/terraform/mycomponent/.terraform.lock.hcl
```

Files are written to the resolved component directory with the correct planfile name.

## Root Causes

### 1. CLI command writes to CWD with hardcoded filename

`cmd/terraform/planfile/download.go` uses `--output` flag (default: `plan.tfplan`) and writes directly to that path without resolving the component directory:

```go
// Current: downloadToFile() writes to outputPath (CWD/plan.tfplan)
case planfile.PlanFilename:
    destPath = outputPath  // CWD/plan.tfplan
case planfile.LockFilename:
    destPath = filepath.Join(filepath.Dir(outputPath), planfile.LockFilename)  // CWD/.terraform.lock.hcl
```

No `ProcessStacks()` call to resolve the actual terraform component path.

### 2. CI hook handler has its own path resolution but duplicates file I/O

`pkg/ci/plugins/terraform/handlers.go` (`downloadPlanfile`) resolves the planfile path via `resolveArtifactPath()`, but duplicates the file-writing logic from the CLI command. Both callers iterate `[]FileResult` and write files independently.

### 3. No SHA256 integrity verification on download

`BundledStore.Upload()` computes SHA256 of the tar archive and stores it in metadata. But `BundledStore.Download()` never verifies the checksum — it extracts the tar without checking:

```go
// Current: no verification
func (s *BundledStore) Download(ctx context.Context, name string) ([]FileResult, *Metadata, error) {
    reader, metadata, err := s.backend.Download(ctx, name)
    defer reader.Close()
    files, err := ExtractTarArchive(reader)  // no SHA256 check
    return files, metadata, nil
}
```

## Desired State

### Layer Responsibilities

```
┌─────────────────────────────────────────────────────┐
│  Caller (CLI command / CI hook handler)             │
│  1. Call ProcessStacks() to resolve component dir   │
│  2. Determine planfile name from stack config       │
│  3. Call store.Download() to get []FileResult       │
│  4. Map FileResult entries to paths in component dir│
│  5. Write files to disk                             │
└──────────────────────┬──────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────┐
│  Planfile Store (adapter.Store)                     │
│  - Passes through to artifact store                 │
│  - Converts metadata (artifact ↔ planfile)          │
│  - Returns []FileResult + *Metadata                 │
│  - Does NOT write files or resolve paths            │
└──────────────────────┬──────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────┐
│  Artifact Store (BundledStore)                      │
│  - Downloads raw stream from backend                │
│  - Verifies SHA256 checksum against metadata        │
│  - Extracts tar into []FileResult                   │
│  - Returns []FileResult + *Metadata                 │
└─────────────────────────────────────────────────────┘
```

### 1. BundledStore.Download verifies SHA256 — DONE

Before extracting the tar archive, `BundledStore.Download` reads the full stream into `[]byte`, computes SHA256, and compares against `metadata.SHA256`. If they differ, returns `ErrArtifactIntegrityFailed` (already existed in `errors/errors.go` — no new sentinel error needed; the PRD originally proposed `ErrArtifactChecksumMismatch` but we reused the existing error).

```go
func (s *BundledStore) Download(ctx context.Context, name string) ([]FileResult, *Metadata, error) {
    reader, metadata, err := s.backend.Download(ctx, name)
    defer reader.Close()

    // Read full content for checksum verification.
    data, err := io.ReadAll(reader)

    // Verify SHA256 if metadata contains a checksum.
    if metadata != nil && metadata.SHA256 != "" {
        h := sha256.Sum256(data)
        actual := hex.EncodeToString(h[:])
        if actual != metadata.SHA256 {
            return nil, nil, fmt.Errorf("%w: expected %s, got %s",
                errUtils.ErrArtifactIntegrityFailed, metadata.SHA256, actual)
        }
    }

    // Extract tar archive.
    files, err := ExtractTarArchive(bytes.NewReader(data))
    return files, metadata, nil
}
```

### 2. CLI command resolves component path via ProcessStacks — DONE

`runDownload()` calls `resolveDownloadPlanfilePath()` which uses `exec.ProcessStacks()` + `exec.ConstructTerraformComponentPlanfilePath()` — the same pattern as `resolveUploadPlanfilePath()` in `upload.go`. This resolves:
- `componentPath` = `TerraformDirAbsolutePath/ComponentFolderPrefix/FinalComponent`
- Planfile name from stack config (or deterministic default)
- Workdir override if source vendoring is configured

**Implementation decision:** The `--output` flag is checked via `cmd.Flags().Changed("output")`. When explicitly set by the user, `ProcessStacks()` is skipped and the flag value is used directly. When not explicitly set (default `plan.tfplan`), the component path is resolved via `ProcessStacks()`.

The old `downloadToFile()` function was removed — replaced by `resolveDownloadPlanfilePath()` + `planfile.WritePlanfileResults()`.

### 3. CI hook handler uses shared helper — DONE

`downloadPlanfile()` in `handlers.go` replaced its inline file-writing loop with a call to `planfile.WritePlanfileResults()`. The path resolution via `resolveArtifactPath()` (which already calls `ProcessStacks` + `ConstructTerraformComponentPlanfilePath`) was kept as-is.

### 4. Shared helper for writing FileResults to component dir — DONE

Created `pkg/ci/plugins/terraform/planfile/write.go`:

```go
// WritePlanfileResults writes downloaded FileResult entries to the component directory.
// planfilePath is the full path for the plan file (e.g., /project/components/terraform/vpc/vpc-prod.tfplan).
// Files not matching known planfile entries are skipped.
// Parent directories are created as needed via os.MkdirAll.
func WritePlanfileResults(results []FileResult, planfilePath string) error
```

Tests in `write_test.go` cover: plan+lock writing, unknown filename skipping, parent directory creation, empty results.

## Files Modified

| File | Changes | Status |
|------|---------|--------|
| `pkg/ci/artifact/bundled_store.go` | Added SHA256 verification in `Download()` — reads full stream, computes checksum, compares before extracting | Done |
| `cmd/terraform/planfile/download.go` | Added `resolveDownloadPlanfilePath()` with `ProcessStacks()`, replaced `downloadToFile()` with shared helper, added `exec` import | Done |
| `pkg/ci/plugins/terraform/handlers.go` | Replaced inline file-writing in `downloadPlanfile()` with `planfile.WritePlanfileResults()`, removed unused `io` import | Done |
| `pkg/ci/plugins/terraform/planfile/write.go` | **Created** — shared `WritePlanfileResults()` helper with `MkdirAll` support | Done |
| `pkg/ci/artifact/bundled_store_test.go` | Added 3 SHA256 tests: match succeeds, mismatch returns `ErrArtifactIntegrityFailed`, empty SHA256 skips check | Done |
| `pkg/ci/plugins/terraform/planfile/write_test.go` | **Created** — 4 tests for `WritePlanfileResults()` | Done |

**Note:** No new sentinel error was needed — reused existing `ErrArtifactIntegrityFailed` from `errors/errors.go` instead of adding the originally proposed `ErrArtifactChecksumMismatch`.

## Edge Cases

### SHA256 missing from metadata

Old artifacts uploaded before SHA256 was computed, or backends that don't store metadata. If `metadata.SHA256` is empty, skip verification (warn, don't fail). This preserves backward compatibility.

### Metadata missing entirely

If `metadata` is nil (backend returned no sidecar), skip SHA256 verification entirely. The download still succeeds — integrity is best-effort when metadata is unavailable.

### Component path with workdir (source vendoring)

When the component uses source vendoring with workdir, `ProcessStacks()` sets `ComponentSection[WorkdirPathKey]`. The CLI command must check for this override (same pattern as `TerraformPlanDiff`).

### --output flag override

Keep the `--output` flag on the CLI command as an escape hatch. When explicitly set by the user (not the default), skip `ProcessStacks()` and write to the specified path. This supports scripting use cases where the user knows the destination.

## Verification

1. `go build ./...` — compiles cleanly
2. `go test ./pkg/ci/artifact/...` — SHA256 verification tests pass
3. `go test ./pkg/ci/plugins/terraform/planfile/...` — write helper tests pass
4. `go test ./cmd/terraform/planfile/...` — CLI download tests pass
5. Manual: `atmos terraform planfile download mycomponent -s prod` — files written to `components/terraform/mycomponent/`
6. Manual: corrupt a stored artifact — download fails with checksum mismatch error
