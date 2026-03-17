# Product Requirements Document: Reflection-Free Native Deep Merge & Terraform Workspace Resilience

## Overview

This PRD covers two closely related improvements shipped together in [PR #2201](https://github.com/cloudposse/atmos/pull/2201):

1. **Performance**: Replace the `mergo`-based stack merge loop with a custom, reflection-free `deepMergeNative` implementation that is **3.5× faster** and eliminates redundant deep-copy allocations.
2. **Correctness**: Fix a Terraform workspace edge-case on Windows (and other file-locking environments) where both `workspace select` and `workspace new` fail with exit code 1, causing Atmos to abort even though the correct workspace is already active.

---

## Problem Statement

### 1. Merge Performance

#### Current State

`MergeWithOptions` (called ~118,000 times per `atmos describe stacks` run) uses the following pattern for every non-empty input map:

```go
dataCurrent, _ = DeepCopyMap(current) // full deep copy of every input
mergo.Merge(&merged, dataCurrent, opts...) // reflection-based merge
```

This means **every** input map is fully deep-copied before being handed to `mergo`, which then walks the copy again using reflection.  The cumulative cost is proportional to `O(n × k)` where `n` is the number of inputs and `k` is the average map size.

#### Root Cause

`mergo` uses Go reflection for all type-dispatch, which carries significant overhead per key–value pair. More critically, there is a documented bug where `mergo.Merge` mutates the *source* map's original pointers in a loop, so the caller must deep-copy every input before passing it—doubling the allocation cost.

#### Impact

- `atmos describe stacks` and `atmos terraform plan` on large monorepos (100+ components) can spend 30–50% of total runtime inside `MergeWithOptions`.
- Memory pressure from unnecessary deep copies increases GC pause frequency.

### 2. Terraform Workspace Edge Case

#### Current State

When Atmos switches Terraform workspaces it runs:

```
terraform workspace select <name>   # attempt 1
terraform workspace new <name>      # attempt 2 if select fails
```

If both commands fail with exit code 1, `ExecuteTerraform` returns an error and aborts.

#### Root Cause

On Windows (and occasionally on Linux under concurrent test runs), file locking can leave `.terraform/environment` intact while the corresponding `terraform.tfstate.d/<workspace>/` directory is deleted.  In this state:

- `workspace select nonprod` exits 1 because the state file is missing.
- `workspace new nonprod` exits 1 because the environment file already names `nonprod` as the active workspace.

Neither command succeeds, yet the component is already in the correct workspace.

#### Impact

- CI pipelines on Windows fail intermittently with misleading "workspace select/new failed" errors.
- Tests that share the `tests/fixtures/components/terraform/mock` directory leave stale `.terraform/environment` files that poison subsequent test runs.

---

## Goals and Objectives

### Primary Goals

1. **Merge performance**: reduce wall-clock time spent in `MergeWithOptions` by ≥ 3×.
2. **Workspace correctness**: eliminate false-positive workspace errors when the environment file and state directory are out of sync.

### Non-Goals

- No change to merge semantics (exact behavioural parity with `mergo.WithOverride + mergo.WithTypeCheck`).
- No removal of the `mergo` dependency from the module (it may still be used elsewhere).
- No change to the public `MergeWithOptions` / `Merge` function signatures.

### Success Criteria

| Criterion | Target |
|-----------|--------|
| `BenchmarkMergeWithOptions` speedup | ≥ 3.0× |
| `pkg/merge` patch test coverage | ≥ 95% |
| `internal/exec/terraform_utils.go` patch test coverage | ≥ 95% |
| No regression in existing merge tests | All green |
| Windows workspace edge case handled | No error when env file matches target workspace |

---

## Solution Design

### 1. Reflection-Free Native Deep Merge

#### Approach

Replace the `mergo.Merge` call in `MergeWithOptions` with a new `deepMergeNative` function that:

- Accepts `dst` and `src` as `map[string]any` and merges `src` into `dst` **in place**.
- Seeds `merged` from a single `DeepCopyMap` of the first input, then calls `deepMergeNative` for every subsequent input—eliminating the pre-copy loop entirely.
- Uses type-switch dispatch instead of reflection for the common cases (`map[string]any`, `[]any`, primitives).
- Falls back to the existing `deepCopyValue`/reflection path for typed maps (e.g., `map[string]schema.Provider`) to maintain correctness for all callers.

#### Behavioural Contract

`deepMergeNative(dst, src, appendSlice, sliceDeepCopy)` is semantically equivalent to the former `mergo.Merge(&dst, src, mergo.WithOverride, mergo.WithTypeCheck)`:

| Scenario | Behaviour |
|----------|-----------|
| Key in `src` only | Added to `dst` |
| Key in `dst` only | Preserved unchanged |
| Both maps | Recursed into |
| `src` value is not a map | Overrides `dst` value |
| `src` is a slice, `dst` is not a slice | Returns type-mismatch error (mirrors `mergo.WithTypeCheck`) |
| `dst` is a slice, `src` is a slice, `appendSlice=true` | Appends `src` to `dst` |
| `dst` is a slice, `src` is a slice, `sliceDeepCopy=true` | Element-wise deep merge |
| Typed map as `src` value | Normalised to `map[string]any` via reflection, then recursed |

#### Integer-Overflow Safety

Size hints passed to `make([]T, n)` combine lengths from two slices/maps.  A `safeAdd(a, b int) int` helper clamps the result to `math.MaxInt` to prevent integer-overflow panics that the GitHub Advanced Security scanner would flag.

#### File Layout

```
pkg/merge/
└── merge_native.go      # deepMergeNative, mergeSlicesNative, appendSlices, safeAdd
```

`merge.go` replaces its import of `dario.cat/mergo` with calls to `deepMergeNative`.

### 2. Terraform Workspace Resilience

#### Helper: `isTerraformCurrentWorkspace`

A new pure function added to `internal/exec/terraform_utils.go`:

```go
// isTerraformCurrentWorkspace reports whether the given workspace name matches
// the workspace recorded in the .terraform/environment file inside componentPath.
func isTerraformCurrentWorkspace(componentPath, workspace string) bool
```

Logic:

1. Resolve the Terraform data directory: use `$TF_DATA_DIR` if set (absolute or relative to `componentPath`), otherwise default to `componentPath/.terraform`.
2. Read `<tfDataDir>/environment`.
3. Return `strings.TrimSpace(content) == workspace`.

#### Call Site Change

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

Only errors with code 1 are suppressed, and only when the environment file confirms the workspace is already correct. All other errors propagate unchanged.

#### Test Cleanup Path Fixes

Two existing test files computed cleanup paths relative to the working directory *after* `t.Chdir`:

- `yaml_func_terraform_state_workspaces_disabled_test.go`
- `yaml_func_utils_test.go`

Both are fixed to compute absolute paths via `filepath.Abs()` *before* `t.Chdir`, ensuring cleanup defers use stable paths regardless of subsequent directory changes.

---

## Functional Requirements

### FR-1: Native Deep Merge

| ID | Requirement |
|----|-------------|
| FR-1.1 | `deepMergeNative` SHALL produce results identical to `mergo.Merge(WithOverride, WithTypeCheck)` for all `map[string]any` inputs |
| FR-1.2 | `deepMergeNative` SHALL handle typed maps (e.g., `map[string]schema.Provider`) by normalising them to `map[string]any` before merging |
| FR-1.3 | `deepMergeNative` SHALL return a descriptive error when a slice is overridden with a non-slice value (type-check parity) |
| FR-1.4 | `deepMergeNative` SHALL support `appendSlice` and `sliceDeepCopy` modes |
| FR-1.5 | `safeAdd` SHALL prevent integer-overflow panics in slice/map size calculations |
| FR-1.6 | `MergeWithOptions` SHALL NOT deep-copy each input before merging (only the first input is seeded into `merged`) |
| FR-1.7 | `BenchmarkMergeWithOptions` SHALL show ≥ 3× speedup over the `mergo`-based baseline |

### FR-2: Terraform Workspace Resilience

| ID | Requirement |
|----|-------------|
| FR-2.1 | `isTerraformCurrentWorkspace` SHALL return `true` when `.terraform/environment` (trimmed) equals the target workspace |
| FR-2.2 | `isTerraformCurrentWorkspace` SHALL return `false` when the environment file is absent or the workspace name does not match |
| FR-2.3 | `isTerraformCurrentWorkspace` SHALL respect the `TF_DATA_DIR` environment variable (absolute and relative forms) |
| FR-2.4 | `ExecuteTerraform` SHALL proceed without error when `workspace new` fails with exit code 1 AND `isTerraformCurrentWorkspace` returns `true` |
| FR-2.5 | `ExecuteTerraform` SHALL return the error unchanged when `workspace new` fails with exit code 1 AND `isTerraformCurrentWorkspace` returns `false` |
| FR-2.6 | `ExecuteTerraform` SHALL return all non-exit-code-1 errors from `workspace new` unchanged |

---

## Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| NFR-1 | Patch test coverage for `pkg/merge/merge_native.go` SHALL be 100% |
| NFR-2 | Patch test coverage for `internal/exec/terraform_utils.go` SHALL be ≥ 95% |
| NFR-3 | All existing `pkg/merge` tests SHALL pass without modification |
| NFR-4 | Race detector (`go test -race`) SHALL report no data races in the merge package |
| NFR-5 | GitHub Advanced Security / CodeQL SHALL report no new alerts |
| NFR-6 | All test paths SHALL use `filepath.Abs()` or `filepath.Join()` (never relative paths computed after `t.Chdir`) |

---

## Testing Strategy

### Merge Package Tests (`pkg/merge/merge_native_test.go`)

Unit tests cover every branch of `deepMergeNative` and `mergeSlicesNative`:

| Test | Description |
|------|-------------|
| `TestDeepMergeNative_NewKeysAddedFromSrc` | Src-only keys appear in dst |
| `TestDeepMergeNative_SrcOverridesDst` | Scalar src overrides scalar dst |
| `TestDeepMergeNative_BothMapsMergedRecursively` | Nested maps are recursed |
| `TestDeepMergeNative_TypeMismatch_SliceVsNonSlice` | Error returned on type mismatch |
| `TestDeepMergeNative_TypedMapMergesWithMapDst` | Typed map normalised and recursed |
| `TestDeepMergeNative_TypedMapOverridesNonMapDst` | Typed map normalised and overrides scalar dst |
| `TestMergeSlicesNative_AppendMode` | Slices appended when `appendSlice=true` |
| `TestMergeSlicesNative_DeepCopyMode` | Element-wise merge when `sliceDeepCopy=true` |
| `TestSafeAdd_NoOverflow` | Normal addition path |
| `TestSafeAdd_Overflow` | Clamp to `math.MaxInt` on overflow |
| `BenchmarkMergeWithOptions` | Benchmark confirming ≥ 3× speedup |

### Terraform Utils Tests (`internal/exec/terraform_utils_test.go`)

| Test | Description |
|------|-------------|
| `TestIsTerraformCurrentWorkspace/matching_workspace` | Returns `true` when file matches |
| `TestIsTerraformCurrentWorkspace/trailing_newline` | Whitespace trimmed before comparison |
| `TestIsTerraformCurrentWorkspace/mismatched_workspace` | Returns `false` on name mismatch |
| `TestIsTerraformCurrentWorkspace/missing_file` | Returns `false` when file absent |
| `TestIsTerraformCurrentWorkspace/missing_directory` | Returns `false` when `.terraform` absent |
| `TestIsTerraformCurrentWorkspace/TF_DATA_DIR_override` | Custom data directory respected |

---

## Implementation Checklist

- [x] `pkg/merge/merge_native.go` — `deepMergeNative`, `mergeSlicesNative`, `appendSlices`, `safeAdd`
- [x] `pkg/merge/merge.go` — replace `mergo.Merge` call with `deepMergeNative`; seed loop from `DeepCopyMap(first)` only
- [x] `pkg/merge/merge_native_test.go` — 100% branch coverage; benchmark
- [x] `internal/exec/terraform_utils.go` — `isTerraformCurrentWorkspace`
- [x] `internal/exec/terraform.go` — conditional workspace-already-active handling
- [x] `internal/exec/terraform_utils_test.go` — 6 sub-tests for `isTerraformCurrentWorkspace`
- [x] `internal/exec/yaml_func_terraform_state_workspaces_disabled_test.go` — absolute path before `t.Chdir`
- [x] `internal/exec/yaml_func_utils_test.go` — absolute path before `t.Chdir`
- [x] `internal/exec/validate_stacks_test.go` — dynamic deduplication threshold
- [x] `website/blog/2026-03-15-faster-deep-merge.mdx` — blog post
- [x] `website/src/data/roadmap.js` — roadmap milestone

---

## References

- Pull Request: [#2201](https://github.com/cloudposse/atmos/pull/2201)
- Blog post: `website/blog/2026-03-15-faster-deep-merge.mdx`
- `mergo` mutation bug: <https://github.com/imdario/mergo/issues/220>
- `mergo` slice deep-copy fix: <https://github.com/imdario/mergo/pull/231>
- `TF_DATA_DIR` documentation: <https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_data_dir>
