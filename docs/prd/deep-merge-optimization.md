# PRD: Reflection-Free Native Deep Merge and Terraform Workspace Resilience

**Status:** Implemented
**Author:** rb ([@nitrocode](https://github.com/nitrocode))
**PR:** [#2201](https://github.com/cloudposse/atmos/pull/2201)

Two improvements shipped together: a 3.5x faster merge implementation and a fix for false-positive Terraform workspace errors on Windows.

## Problem Statement

### 1. Merge Performance

`MergeWithOptions` is called approximately 118,000 times per `atmos describe stacks` run (measured on a representative large monorepo). For every input it:

1. Calls `DeepCopyMap(current)` to work around a `mergo` mutation bug (mergo mutates the source pointer inside a loop).
2. Calls `mergo.Merge` which uses Go reflection for all type dispatch.

Every input is fully deep-copied before being handed to `mergo`, which then walks the copy again using reflection. On large monorepos (100+ components), profiling shows this to be a dominant cost inside `MergeWithOptions` and increases GC pause frequency.

### 2. Terraform Workspace Edge Case

When Atmos switches workspaces it tries `workspace select <name>`, then falls back to `workspace new <name>`. If both fail with exit code 1, `ExecuteTerraform` aborts.

On Windows, file locking can leave `.terraform/environment` intact while `terraform.tfstate.d/<workspace>/` is deleted. In that state:

- `workspace select nonprod` fails because the state file is missing.
- `workspace new nonprod` fails because the environment file already names `nonprod` as active.

The component is in the correct workspace but Atmos errors out. This also affects Linux under concurrent tests when shared fixtures leave stale `.terraform/environment` files.

## Goals

1. Reduce wall-clock time in `MergeWithOptions` by >= 3x with no change to merge semantics.
2. Eliminate false-positive workspace errors when the environment file and state directory are out of sync.
3. Fix test cleanup to use absolute paths computed before `t.Chdir`.

**Out of scope:** removing `mergo` from the module entirely; changing public function signatures.

## Success Criteria

| Criterion | Target |
|-----------|--------|
| `BenchmarkMergeWithOptions` speedup | >= 3.0x |
| `pkg/merge/merge_native.go` test coverage | 100% |
| `internal/exec/terraform_utils.go` test coverage | >= 95% |
| Existing merge tests | All pass |
| Windows workspace edge case | No error when env file matches target workspace |

## Solution Design

### 1. Native Deep Merge (`pkg/merge/merge_native.go`)

Replace `mergo.Merge` with `deepMergeNative(dst, src map[string]any, appendSlice, sliceDeepCopy bool) error`:

- Merges `src` into `dst` in place using type-switch dispatch (no reflection for `map[string]any`, `[]any`, primitives).
- Seeds `merged` from `DeepCopyMap(inputs[0])` once, then calls `deepMergeNative` for each subsequent input. This eliminates the per-input deep-copy that existed solely to work around the `mergo` mutation bug.
- Falls back to `deepCopyValue`/reflection for typed maps (e.g., `map[string]schema.Provider`) to preserve correctness for all callers.
- Returns a type-mismatch error when a slice is overridden with a non-slice value, matching `mergo.WithTypeCheck` behavior.

A `safeAdd(a, b int) int` helper clamps size-hint arithmetic to `math.MaxInt` to prevent integer-overflow alerts from GitHub Advanced Security.

**Behavioral contract** (identical to `mergo.WithOverride + mergo.WithTypeCheck`):

| Scenario | Result |
|----------|--------|
| Key only in `src` | Added to `dst` |
| Key only in `dst` | Preserved |
| Both values are maps | Recursed |
| `src` value is not a map | Overrides `dst` |
| `src` is a slice, `dst` is not | Type-mismatch error |
| Both slices, `appendSlice=true` | `src` appended to `dst` |
| Both slices, `sliceDeepCopy=true` | Element-wise deep merge |
| Typed map in `src` | Normalized to `map[string]any`, then recursed |

### 2. Terraform Workspace Resilience (`internal/exec/terraform_utils.go`)

New helper:

```go
// isTerraformCurrentWorkspace reports whether workspace matches the name in
// .terraform/environment (or $TF_DATA_DIR/environment) inside componentPath.
func isTerraformCurrentWorkspace(componentPath, workspace string) bool
```

Logic:
1. Use `$TF_DATA_DIR` if set (absolute or relative to `componentPath`); otherwise default to `componentPath/.terraform`.
2. Read the `environment` file and trim whitespace.
3. Return whether the trimmed content equals `workspace`.

In `ExecuteTerraform`, after `workspace new` fails with exit code 1:

```go
var newExitCodeErr errUtils.ExitCodeError
if errors.As(err, &newExitCodeErr) && newExitCodeErr.Code == 1 &&
    isTerraformCurrentWorkspace(componentPath, info.TerraformWorkspace) {
    log.Debug("Workspace is already the active workspace; proceeding",
        "workspace", info.TerraformWorkspace)
} else {
    return err
}
```

Only exit-code-1 errors are suppressed, and only when the environment file confirms the workspace is already active. All other errors propagate unchanged.

### 3. Test Cleanup Fixes

Two test files computed cleanup paths relative to the working directory after `t.Chdir`:

- `yaml_func_terraform_state_workspaces_disabled_test.go`
- `yaml_func_utils_test.go`

Both are updated to compute absolute paths via `filepath.Abs()` before `t.Chdir`, so cleanup defers always resolve correctly.

## Requirements

| ID | Requirement |
|----|-------------|
| R-1 | `deepMergeNative` produces results identical to `mergo.Merge(WithOverride, WithTypeCheck)` for all `map[string]any` inputs |
| R-2 | `deepMergeNative` handles typed maps by normalizing to `map[string]any` before merging |
| R-3 | `deepMergeNative` returns an error when a slice is overridden with a non-slice value |
| R-4 | `MergeWithOptions` deep-copies only the first input, not every input |
| R-5 | `safeAdd` prevents integer-overflow panics in size calculations |
| R-6 | `isTerraformCurrentWorkspace` returns `true` when `.terraform/environment` (trimmed) equals the target workspace |
| R-7 | `isTerraformCurrentWorkspace` returns `false` when the file is absent or the name does not match |
| R-8 | `isTerraformCurrentWorkspace` respects `TF_DATA_DIR` (absolute and relative forms) |
| R-9 | `ExecuteTerraform` proceeds without error when `workspace new` exits 1 and the workspace is already active |
| R-10 | `ExecuteTerraform` propagates all other `workspace new` errors unchanged |
| R-11 | Test cleanup paths use `filepath.Abs()` before `t.Chdir`, not relative paths after |
| R-12 | Race detector (`go test -race`) reports no data races in the merge package |
| R-13 | CodeQL reports no new alerts |

## Changed Files

| File | Change |
|------|--------|
| `pkg/merge/merge_native.go` | New: `deepMergeNative`, `mergeSlicesNative`, `appendSlices`, `safeAdd` |
| `pkg/merge/merge.go` | Replace `mergo.Merge` with `deepMergeNative`; seed from first input only |
| `pkg/merge/merge_native_test.go` | New: 100% branch coverage, benchmark |
| `internal/exec/terraform_utils.go` | New: `isTerraformCurrentWorkspace` |
| `internal/exec/terraform.go` | Skip error when workspace is already active |
| `internal/exec/terraform_utils_test.go` | New: 6 sub-tests for `isTerraformCurrentWorkspace` |
| `internal/exec/yaml_func_terraform_state_workspaces_disabled_test.go` | Absolute path before `t.Chdir` |
| `internal/exec/yaml_func_utils_test.go` | Absolute path before `t.Chdir` |
| `internal/exec/validate_stacks_test.go` | Dynamic deduplication threshold |
| `website/blog/2026-03-15-faster-deep-merge.mdx` | Blog post |
| `website/src/data/roadmap.js` | Roadmap milestone |

## References

- [PR #2201](https://github.com/cloudposse/atmos/pull/2201)
- [`mergo` mutation bug](https://github.com/imdario/mergo/issues/220)
- [`mergo` slice deep-copy fix](https://github.com/imdario/mergo/pull/231)
- [`TF_DATA_DIR` documentation](https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_data_dir)
