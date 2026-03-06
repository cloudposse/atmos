# Planfile Storage Validation & Git SHA Resolution — SHIPPED

> Related: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Interfaces](../framework/interfaces.md) | [Hooks Integration](../framework/hooks-integration.md) | [Generic Provider](../providers/generic.md)

## Status: SHIPPED

All steps implemented: generic provider git SHA fallback, `getArtifactKey()` refactored to use `KeyPattern`, metadata validation added.

## Problem Statement

Running `atmos terraform plan mycomponent -s prod --ci` with the generic CI provider and local planfile storage produces a planfile artifact with an **empty SHA**. This makes the artifact useless for downstream operations (apply verification, audit trail, artifact lookup by commit).

### Root Causes

There are three independent failures that combine to produce this bug:

**1. `getArtifactKey()` bypasses `KeyPattern` validation entirely**

`pkg/ci/plugins/terraform/plugin.go:130-153` builds keys using a hardcoded `fmt.Sprintf("%s/%s.tfplan", stack, component)` pattern. It never calls `planfile.KeyPattern.GenerateKey()`, which means:

- The configurable `KeyPattern` with `{{ .SHA }}` is ignored.
- The `validateKeyContext()` function that would reject empty SHA is never invoked.
- SHA is not part of the key at all, so multiple plans for the same stack/component overwrite each other.

**2. Generic provider returns empty SHA when CI env vars are absent**

`pkg/ci/providers/generic/provider.go:78` calls `getFirstEnv("ATMOS_CI_SHA", "GIT_COMMIT", "CI_COMMIT_SHA", "COMMIT_SHA")`. When running locally with `--ci`, none of these env vars are set. The provider returns `SHA: ""` without error. It does not fall back to `git rev-parse HEAD`, despite `pkg/git.DefaultGitRepo.GetCurrentCommitSHA()` being available.

**3. No store-level validation of required metadata fields**

All three planfile store implementations (local, S3, GitHub) accept `Upload()` calls with metadata containing empty `SHA`, `Stack`, or `Component` fields. The `planfile.Metadata` struct has no validation method. There is no guard at the storage boundary.

### Impact

- **Broken artifact identity**: Planfiles cannot be looked up by commit SHA.
- **Silent overwrites**: Without SHA in the key, consecutive plans for the same stack/component overwrite each other.
- **Broken apply lifecycle**: `before.terraform.apply` cannot reliably download the correct planfile for a commit.
- **Audit gap**: Stored metadata has empty SHA, making it impossible to trace which commit a plan belongs to.

## Desired State

### 1. SHA-aware artifact keys via `KeyPattern`

`getArtifactKey()` must use `planfile.KeyPattern.GenerateKey()` with the configured pattern (defaulting to `{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan`). This ensures:

- SHA is part of the key (artifacts are commit-scoped).
- `validateKeyContext()` rejects empty required fields.
- Key format is user-configurable via `components.terraform.planfiles.key_pattern`.

```go
// Before (current) — hardcoded, no SHA
func (p *Plugin) getArtifactKey(info *schema.ConfigAndStacksInfo, _ string) string {
    return fmt.Sprintf("%s/%s.tfplan", stack, component)
}

// After (proposed) — uses KeyPattern, includes SHA, validates
func (p *Plugin) getArtifactKey(info *schema.ConfigAndStacksInfo, ciCtx *provider.Context) (string, error) {
    pattern := planfile.DefaultKeyPattern()
    // TODO: override from config if set

    keyCtx := &planfile.KeyContext{
        Stack:     info.Stack,
        Component: info.ComponentFromArg,
        SHA:       ciCtx.SHA,
    }

    return pattern.GenerateKey(keyCtx)
}
```

### 2. Generic provider resolves SHA from git when env vars are absent

The generic provider's `Context()` should fall back to `git rev-parse HEAD` (via `pkg/git.DefaultGitRepo.GetCurrentCommitSHA()`) when no CI SHA env var is found. This covers the local `--ci` use case where the developer is in a git repo but has no CI env vars set.

```go
// Before (current) — empty SHA when no env vars
SHA: getFirstEnv("ATMOS_CI_SHA", "GIT_COMMIT", "CI_COMMIT_SHA", "COMMIT_SHA"),

// After (proposed) — git fallback
SHA: getFirstEnvOrGitSHA("ATMOS_CI_SHA", "GIT_COMMIT", "CI_COMMIT_SHA", "COMMIT_SHA"),
```

The git fallback is best-effort: if the repo is not a git repo or HEAD is detached, SHA remains empty and the upload will fail at the key generation step (desired behavior — you shouldn't store planfiles without knowing which commit they belong to).

### 3. Planfile metadata validation on upload

Add a `Validate()` method to `planfile.Metadata` that enforces required fields. Call it from `uploadPlanfile()` before invoking the store.

```go
func (m *Metadata) Validate() error {
    if m.Stack == "" || m.Component == "" || m.SHA == "" {
        return ErrPlanfileMetadataInvalid
    }
    return nil
}
```

This is a defense-in-depth measure. The primary validation happens at key generation (`validateKeyContext`), but metadata validation catches cases where the key pattern doesn't include all fields.

## Files to Modify

### Step 1: Add git SHA fallback to generic provider

**`pkg/ci/providers/generic/provider.go`**
- Add a `getFirstEnvOrGitSHA()` helper that tries env vars first, then falls back to `pkg/git.DefaultGitRepo.GetCurrentCommitSHA()`.
- Use it for the `SHA` field in `Context()`.
- Similarly add git fallback for `Branch` field using `pkg/git` (best-effort).

### Step 2: Refactor `getArtifactKey()` to use `KeyPattern`

**`pkg/ci/plugins/terraform/plugin.go`**
- Change `getArtifactKey()` signature to accept `*provider.Context` (for SHA) and return `(string, error)`.
- Build a `planfile.KeyContext` from `info` and `ciCtx`.
- Call `planfile.DefaultKeyPattern().GenerateKey(keyCtx)`.
- Return the error to the caller (upload will be skipped on validation failure).

**`pkg/ci/plugins/terraform/handlers.go`**
- Update `uploadPlanfile()` and `downloadPlanfile()` callers to pass `ctx.CICtx` to `getArtifactKey()`.
- Handle the new error return from `getArtifactKey()`.
- For `downloadPlanfile()`: the caller must also have access to SHA to construct the key. This means the CI context must be available (which it already is via `ctx.CICtx`).

### Step 3: Add metadata validation

**`pkg/ci/plugins/terraform/planfile/interface.go`**
- Add `Validate() error` method to `Metadata`.
- Add `ErrPlanfileMetadataInvalid` sentinel error to `errors/errors.go`.

**`pkg/ci/plugins/terraform/handlers.go`**
- Call `metadata.Validate()` in `uploadPlanfile()` before `store.Upload()`.
- Return a fatal error if validation fails (same severity as other upload failures).

### Step 4: Update tests

**`pkg/ci/providers/generic/provider_test.go`**
- Add test: `Context()` returns git HEAD SHA when no CI env vars are set (in a git repo).
- Add test: `Context()` returns empty SHA when not in a git repo and no env vars (graceful degradation).

**`pkg/ci/plugins/terraform/plugin_test.go`**
- Update `getArtifactKey()` tests for new signature and SHA-inclusive keys.
- Add test: empty SHA returns `ErrPlanfileKeyInvalid`.
- Add test: empty stack returns `ErrPlanfileKeyInvalid`.
- Add test: empty component returns `ErrPlanfileKeyInvalid`.

**`pkg/ci/plugins/terraform/handlers_test.go`**
- Update `uploadPlanfile` tests to provide `CICtx` with SHA.
- Add test: upload fails when SHA is empty.

**`pkg/ci/plugins/terraform/planfile/interface_test.go`**
- Add tests for `Metadata.Validate()`.

## Edge Cases

### What if the working directory is not a git repo?

`GetCurrentCommitSHA()` returns an error. The generic provider logs a debug message and returns empty SHA. The upload will fail at key generation with `ErrPlanfileKeyInvalid`. This is correct behavior — without a commit identity, planfiles should not be stored.

### What if HEAD is detached?

`go-git` handles detached HEAD correctly — `repo.Head()` returns the commit hash. No special handling needed.

### What about dirty working trees?

The SHA represents the commit, not the working tree state. A dirty tree still has a HEAD commit. This is consistent with how GitHub Actions' `GITHUB_SHA` works — it's the commit SHA, not a tree hash.

### What about `downloadPlanfile()` on apply?

The apply command also needs the SHA to construct the download key. Since `ctx.CICtx.SHA` is populated from the provider context (which now includes git fallback), this works automatically. The same SHA that was used to upload during `plan` will be used to download during `apply`, provided the git HEAD hasn't changed between plan and apply.

### What about existing planfiles stored without SHA in the key?

This is a breaking change to the key format. Planfiles stored under the old `stack/component.tfplan` key will not be found under the new `stack/component/sha.tfplan` key. This is acceptable because:

- The feature is new and behind `--ci` flag.
- Old planfiles without SHA are unreliable anyway (no commit provenance).
- Users can re-run plan to generate new correctly-keyed artifacts.

## Verification

1. `go build ./...`
2. `go test ./pkg/ci/providers/generic/...`
3. `go test ./pkg/ci/plugins/terraform/...`
4. `go test ./pkg/ci/plugins/terraform/planfile/...`
5. `make lint`
6. Manual: `atmos terraform plan vpc -s dev --ci` with no CI env vars — verify planfile stored with git SHA in key and metadata.
7. Manual: `atmos terraform plan vpc -s dev --ci` outside git repo — verify clear error about missing SHA.
