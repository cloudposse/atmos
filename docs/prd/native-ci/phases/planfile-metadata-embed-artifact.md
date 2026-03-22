# Planfile Metadata Should Embed Artifact Metadata — SHIPPED

> Related: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Interfaces](../framework/interfaces.md) | [Planfile Storage Validation](planfile-storage-validation.md)

## Status: SHIPPED

All steps implemented: `artifact.Metadata.Validate()`, `planfile.Metadata` embeds `artifact.Metadata`, adapter simplified, metadata construction sites updated, JSON backward-compatible.

## Problem Statement

`planfile.Metadata` and `artifact.Metadata` share 11 identical fields (Stack, Component, SHA, BaseSHA, Branch, PRNumber, RunID, Repository, CreatedAt, ExpiresAt, Custom) but are defined as completely independent structs. This creates several problems:

### 1. Field drift between artifact and planfile metadata

`artifact.Metadata` has `SHA256` and `AtmosVersion` fields that `planfile.Metadata` does not. When the artifact layer adds new fields (e.g., `AtmosVersion`), planfile metadata silently loses them. The adapter's `artifactToPlanfileMeta()` drops `SHA256` and `AtmosVersion` because `planfile.Metadata` has no place to store them.

### 2. Duplicated validation logic

`planfile.Metadata.Validate()` checks Stack, Component, SHA. There is no corresponding `artifact.Metadata.Validate()`. If artifact metadata gains validation, planfile would need to duplicate it. Validation should live on the base type and be extended by the planfile type.

### 3. Lossy adapter conversions

The adapter in `pkg/ci/plugins/terraform/planfile/adapter/store.go` manually copies 11 fields between the two structs. The `planfileToArtifactMeta()` function does not map `MD5` to `SHA256` or into `Custom`, silently losing it. The `artifactToPlanfileMeta()` function drops `SHA256` and `AtmosVersion`. With embedding, the shared fields would not require manual copying — only the planfile-specific extensions need conversion.

### 4. Inconsistent checksum fields

`artifact.Metadata` uses `SHA256`. `planfile.Metadata` uses `MD5`. Neither type carries both. The local artifact store computes SHA-256 on upload, but the adapter never surfaces it to the planfile layer. The local planfile store computes MD5, but the adapter drops it when converting to artifact metadata.

### 5. Missing validation in CLI upload path

`cmd/terraform/planfile/upload.go:buildUploadMetadata()` constructs a `planfile.Metadata` but never calls `Validate()` before passing it to the store. If `Validate()` lived on the embedded base type, it would be more visible and harder to skip.

## Desired State

### 1. `planfile.Metadata` embeds `artifact.Metadata`

```go
// pkg/ci/artifact/metadata.go
type Metadata struct {
    Stack        string            `json:"stack"`
    Component    string            `json:"component"`
    SHA          string            `json:"sha"`
    BaseSHA      string            `json:"base_sha,omitempty"`
    Branch       string            `json:"branch,omitempty"`
    PRNumber     int               `json:"pr_number,omitempty"`
    RunID        string            `json:"run_id,omitempty"`
    Repository   string            `json:"repository,omitempty"`
    CreatedAt    time.Time         `json:"created_at"`
    ExpiresAt    *time.Time        `json:"expires_at,omitempty"`
    SHA256       string            `json:"sha256,omitempty"`
    AtmosVersion string            `json:"atmos_version,omitempty"`
    Custom       map[string]string `json:"custom,omitempty"`
}

// Validate checks that required base metadata fields are present.
func (m *Metadata) Validate() error {
    if m.Stack == "" || m.Component == "" || m.SHA == "" {
        return ErrArtifactMetadataInvalid
    }
    return nil
}
```

```go
// pkg/ci/plugins/terraform/planfile/interface.go
type Metadata struct {
    artifact.Metadata

    // Planfile-specific fields.
    ComponentPath    string `json:"component_path"`
    PlanSummary      string `json:"plan_summary,omitempty"`
    HasChanges       bool   `json:"has_changes"`
    Additions        int    `json:"additions"`
    Changes          int    `json:"changes"`
    Destructions     int    `json:"destructions"`
    MD5              string `json:"md5,omitempty"`
    TerraformVersion string `json:"terraform_version,omitempty"`
    TerraformTool    string `json:"terraform_tool,omitempty"`
}
```

With this embedding:
- `planfile.Metadata` automatically gets all current and future `artifact.Metadata` fields.
- `m.Stack`, `m.SHA`, etc. still work directly (Go embedding promotes fields).
- `m.Validate()` calls the base `artifact.Metadata.Validate()` (can be overridden if planfile needs stricter validation).
- JSON serialization is unchanged — embedded struct fields are flattened.

### 2. Simplified adapter conversion

The adapter no longer needs to copy shared fields one by one. It copies the embedded `artifact.Metadata` as a whole and only converts the planfile-specific extensions:

```go
func planfileToArtifactMeta(meta *planfile.Metadata) *artifact.Metadata {
    if meta == nil { return nil }

    // Start from the embedded base — all shared fields come for free.
    artMeta := meta.Metadata // copy the embedded artifact.Metadata

    // Ensure Custom map exists.
    if artMeta.Custom == nil {
        artMeta.Custom = make(map[string]string)
    }

    // Copy planfile-specific fields into Custom.
    if meta.ComponentPath != "" { artMeta.Custom[customKeyComponentPath] = meta.ComponentPath }
    // ... (remaining planfile-specific fields)

    return &artMeta
}

func artifactToPlanfileMeta(meta *artifact.Metadata) *planfile.Metadata {
    if meta == nil { return nil }

    result := &planfile.Metadata{
        Metadata: *meta,  // embed the full artifact metadata (SHA256, AtmosVersion preserved)
    }

    // Extract planfile-specific fields from Custom.
    for k, v := range meta.Custom {
        switch k {
        case customKeyComponentPath: result.ComponentPath = v
        // ... (remaining)
        }
    }

    // Clean planfile-specific keys from the Custom map.
    // ...

    return result
}
```

### 3. Validation on `artifact.Metadata`

Move the base validation (Stack + Component + SHA required) to `artifact.Metadata.Validate()`. Planfile can override with additional checks if needed. The CLI upload command should also call `Validate()`.

### 4. `PlanfileInfo` embeds `artifact.ArtifactInfo`

The two info types are structurally identical except for `Key` vs `Name`. Unify them:

```go
// pkg/ci/plugins/terraform/planfile/interface.go
type PlanfileInfo struct {
    artifact.ArtifactInfo
}
```

The `Key` field in `PlanfileInfo` maps to `Name` in `ArtifactInfo`. Since `PlanfileInfo.Key` is used in the CLI list command and JSON output, we need to keep backward compatibility. The simplest approach: rename `ArtifactInfo.Name` to `Key`, or keep `PlanfileInfo` as a thin wrapper that aliases `Name` to `Key` for JSON.

**Decision**: Keep `PlanfileInfo` with its own `Key` field for now (JSON contract stability), but have the adapter simply copy `ArtifactInfo.Name` → `PlanfileInfo.Key` as it does today. This avoids a breaking change to the JSON output format.

## Files to Modify

### Step 1: Add `Validate()` to `artifact.Metadata`

**`pkg/ci/artifact/metadata.go`**
- Add `Validate() error` method to `Metadata` that requires Stack, Component, SHA.

**`errors/errors.go`**
- Add `ErrArtifactMetadataInvalid` sentinel error.

### Step 2: Embed `artifact.Metadata` in `planfile.Metadata`

**`pkg/ci/plugins/terraform/planfile/interface.go`**
- Add `import "github.com/cloudposse/atmos/pkg/ci/artifact"`.
- Replace the 11 shared field declarations with `artifact.Metadata` embedding.
- Keep planfile-specific fields as-is.
- Update `Validate()` to delegate to `m.Metadata.Validate()` (base validation), then add planfile-specific checks if any.

### Step 3: Simplify adapter conversion

**`pkg/ci/plugins/terraform/planfile/adapter/store.go`**
- Simplify `planfileToArtifactMeta()`: copy embedded `meta.Metadata` directly, then only convert planfile-specific fields into Custom.
- Simplify `artifactToPlanfileMeta()`: assign `Metadata: *meta` directly, then extract planfile-specific fields from Custom.
- `SHA256` and `AtmosVersion` are now preserved automatically (no more silent loss).

### Step 4: Update all metadata construction sites

**`pkg/ci/plugins/terraform/handlers.go` — `buildPlanfileMetadata()`**
- Update to set fields on the embedded `artifact.Metadata`:
  ```go
  metadata := &planfile.Metadata{
      Metadata: artifact.Metadata{
          Stack:     ctx.Info.Stack,
          Component: ctx.Info.ComponentFromArg,
          CreatedAt: time.Now(),
      },
      ComponentPath: ctx.Info.ComponentFolderPrefix,
  }
  ```

**`cmd/terraform/planfile/upload.go` — `buildUploadMetadata()`**
- Update to use embedded struct.
- Call `Validate()` before upload.

### Step 5: Update store implementations

**`pkg/ci/plugins/terraform/planfile/local/store.go`**
- Update metadata JSON read/write to handle embedded struct. Go's `json.Marshal`/`Unmarshal` flattens embedded structs, so existing `.metadata.json` files remain compatible.

**`pkg/ci/plugins/terraform/planfile/s3/store.go`**
- Same JSON compatibility — no structural changes needed.

**`pkg/ci/plugins/terraform/planfile/github/store.go`**
- Same JSON compatibility — no structural changes needed.

### Step 6: Update tests

**`pkg/ci/artifact/metadata_test.go`** (new file)
- Add tests for `artifact.Metadata.Validate()`.

**`pkg/ci/plugins/terraform/planfile/interface_test.go`**
- Update `TestMetadataValidate` to verify delegation to base `Validate()`.
- Add test: planfile metadata inherits artifact fields (SHA256, AtmosVersion).

**`pkg/ci/plugins/terraform/planfile/adapter/store_test.go`**
- Update conversion tests to verify SHA256 and AtmosVersion are preserved through round-trip.
- Verify MD5 is not silently lost.

**`pkg/ci/plugins/terraform/handlers_test.go`**
- Update `TestBuildPlanfileMetadata` to use embedded struct field access.

**`cmd/terraform/planfile/upload_test.go`**
- Update `buildUploadMetadata` tests for embedded struct.

### Step 7: Update list command data mapping

**`cmd/terraform/planfile/list.go`**
- No changes needed — field access like `f.Metadata.Stack` works with embedding (Go promotes embedded fields).

## Edge Cases

### JSON backward compatibility

Go's `encoding/json` flattens embedded structs. A `planfile.Metadata` with `artifact.Metadata` embedded serializes to the same flat JSON as the current non-embedded struct, provided there are no field name collisions. Since all shared fields have identical names and JSON tags, existing `.metadata.json` files deserialize correctly into the new embedded struct.

### Field name collisions

`planfile.Metadata` must not declare any field with the same name as an `artifact.Metadata` field. Currently there are no collisions. The `Custom` field exists in both — with embedding, planfile's `Custom` will come from the embedded `artifact.Metadata.Custom`. This is correct behavior.

### Validate() override

With embedding, calling `m.Validate()` on a `planfile.Metadata` calls `planfile.Metadata.Validate()` if defined, not the embedded `artifact.Metadata.Validate()`. The planfile `Validate()` should call `m.Metadata.Validate()` explicitly to chain base validation. If planfile needs no additional validation, it can be removed entirely and the embedded method will be promoted.

### Import cycle risk

`planfile` importing `artifact` creates a one-way dependency: `planfile → artifact`. This is the correct direction — artifact is the base, planfile is the extension. The adapter already imports artifact, so no new dependency direction is introduced.

## Verification

1. `go build ./...`
2. `go test ./pkg/ci/artifact/...`
3. `go test ./pkg/ci/plugins/terraform/planfile/...`
4. `go test ./pkg/ci/plugins/terraform/...`
5. `go test ./cmd/terraform/planfile/...`
6. `make lint`
7. Verify JSON round-trip: serialize planfile.Metadata → JSON → deserialize → verify all fields (including SHA256, AtmosVersion from artifact base).
8. Verify existing `.metadata.json` files (from local store) deserialize correctly into new embedded struct.
