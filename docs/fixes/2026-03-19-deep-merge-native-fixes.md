# Deep-Merge Native & Terraform Workspace Fixes

**Date:** 2026-03-19 (updated 2026-03-23)
**PR:** #2201 (perf: replace mergo with native deep merge)
**Reviewer findings:** CodeRabbit audit + GitHub Advanced Security alerts + independent deep analysis

---

## What the PR Does

Replaces the pre-merge deep-copy loop (which called `mergo.Merge` after copying each input)
with a native Go implementation that deep-copies only the first input and merges subsequent
inputs in-place with leaf-level copying. This reduces N full `DeepCopyMap` calls to 1,
achieving ~3.5× speedup on the ~118k+ merge calls per stack resolution run.

**This is the core of Atmos.** Every stack resolution passes through this code. Any bug
here affects every single `atmos` command that processes stacks.

### Architecture Change

**Before:**
```
for each input:
  copy = DeepCopyMap(input)     // Full deep copy of every input
  mergo.Merge(result, copy)     // mergo merge (uses reflection internally)
```

**After:**
```
result = DeepCopyMap(inputs[0])   // Deep copy only the first input
for each remaining input:
  deepMergeNative(result, input)  // Native merge with leaf-level copying (no reflection)
```

---

## Native Merge Semantics

### Merge Rules (merge_native.go)

| Scenario | Behavior | Correct? |
|---|---|---|
| Both map | Recursive merge | ✅ |
| Src map, dst not map | Src overrides dst | ✅ (matches mergo WithOverride) |
| Src not map, dst map | Src overrides dst | ✅ (matches mergo WithOverride) |
| Src slice, dst map | **Error** — `ErrMergeTypeMismatch` | ⚠️ Asymmetric but intentional |
| Src nil | Override dst with nil | ✅ (matches mergo WithOverride) |
| Src typed map | Normalize to `map[string]any` via reflection | ✅ |
| Src typed slice | Normalize to `[]any` via reflection | ✅ |

### Slice Merge Modes

| Mode | Behavior | Notes |
|---|---|---|
| Default | Src slice replaces dst slice | Standard |
| `appendSlice` | Dst + src elements concatenated | Both deep-copied |
| `sliceDeepCopy` | Element-wise merge, src extends result | Fixed (was truncating) |

### Type Handling

| Type | Deep Copy Method | Correct? |
|---|---|---|
| Primitives (string, int, float, bool) | Pass-through (immutable) | ✅ |
| `map[string]any` | Recursive `deepCopyMap` | ✅ |
| `[]any` | Recursive `deepCopySlice` | ✅ |
| Typed maps (`map[string]string`) | Reflection-based iteration | ✅ |
| Typed slices (`[]string`) | Reflection-based iteration | ✅ |
| Pointers | Pass-through (**aliased**) | ⚠️ Safe for YAML data |
| `nil` | Pass-through | ✅ |

---

## Issues Addressed

### 1. `sliceDeepCopy` truncation — silent data loss (CRITICAL, fixed)

**File:** `pkg/merge/merge_native.go`

**Problem:** When `sliceDeepCopy=true` and src had more elements than dst, extra src elements
were **silently dropped**. This was a data loss bug for users with `list_merge_strategy: deep`
whose overlay stacks add new list elements beyond the base.

**Example:** A base stack with 2 EKS node groups + an overlay adding a 3rd `gpu` group would
silently lose the gpu group:

```yaml
# base: 2 node groups
node_groups:
  - name: general
    instance_type: m5.large
  - name: compute
    instance_type: c5.xlarge

# overlay: adds 3rd group
node_groups:
  - name: general
    instance_type: m5.2xlarge
  - name: compute
    instance_type: c5.2xlarge
  - name: gpu                    # ← SILENTLY DROPPED
    instance_type: g5.xlarge
```

**Fix:** `mergeSlicesNative` now uses `max(len(dst), len(src))` for result length. Extra src
elements are deep-copied and appended, matching mergo's `WithSliceDeepCopy` behavior.

**Tests:** 3 existing tests updated from expecting truncation to expecting extension.
5 new cross-validation tests added for `appendSlice` and `sliceDeepCopy` modes.

### 2. `sliceDeepCopy` vs `appendSlice` precedence flip (behavioral regression, fixed)

**File:** `pkg/merge/merge_native.go`

**Problem:** The new `deepMergeNative` checked `appendSlice` before `sliceDeepCopy`, but the
old mergo code checked `WithSliceDeepCopy` first. When both flags are `true`, the old code
applied element-wise merging, the new code appended.

**Fix:** Reordered: `if sliceDeepCopy { … } else { /* appendSlice */ }`.

### 3. `mergeSlicesNative` aliased dst maps and tail elements (fixed)

**File:** `pkg/merge/merge_native.go`

**Problem (inner maps):** Shallow copy of dstMap values into merged map caused silent
corruption in multi-input merges.

**Fix:** `merged[k] = deepCopyValue(v)` for every dstMap value.

**Problem (tail elements):** `copy(result, dst)` shallow-copied tail positions, creating
aliases that could corrupt the accumulator in subsequent merge passes.

**Fix:** Deep-copy tail positions explicitly.

### 4. Misleading test name (fixed)

**File:** `pkg/merge/merge_compare_mergo_test.go`

**Problem:** Case named `"nil value in src map entry is skipped"` but nil actually overrides.

**Fix:** Renamed to `"nil value in src map entry overrides dst"`.

### 5. Cross-validation test coverage too narrow (fixed)

**File:** `pkg/merge/merge_compare_mergo_test.go`

**Problem:** Only 4 equivalence cases tested against mergo. No coverage for `appendSlice`
or `sliceDeepCopy` modes.

**Fix:** Added 5 new cross-validation tests:
- `appendSlice concatenates slices`
- `appendSlice with nested maps`
- `sliceDeepCopy merges overlapping map elements`
- `sliceDeepCopy src extends beyond dst length`
- `sliceDeepCopy with three inputs extending progressively`

### 6. `isTerraformCurrentWorkspace` default workspace handling (fixed)

**File:** `internal/exec/terraform_utils.go`

**Problem:** Terraform never writes `.terraform/environment` for the `default` workspace.
The helper always returned `false` when the file was absent, so workspace recovery never
triggered for `default`.

**Fix:** Return `true` when file is missing AND workspace is `"default"`. Return `true`
when file is empty AND workspace is `"default"`.

### 7. Workspace recovery log level too low (fixed)

**File:** `internal/exec/terraform_execute_helpers_exec.go`

**Fix:** Upgraded `log.Debug` to `log.Warn` for workspace recovery messages.

### 8. Integer overflow in size computations (fixed)

**File:** `pkg/merge/merge_native.go`

**Fix:** `safeCap(a, b)` clamps to `1<<24` (16M entries) to prevent OOM.

---

## Remaining Items

### Fixed

1. ~~**Document the mergo/native split**~~ ✅ — Added comments to all three remaining
   mergo call sites explaining why they still use mergo:
   - `pkg/merge/merge_yaml_functions.go:177` — YAML function slice merging has different
     semantics (operates on individual elements during `!include`/`!merge`, not full stacks).
   - `pkg/merge/merge_yaml_functions.go:265` — Cross-references the first comment.
   - `pkg/devcontainer/config_loader.go:350` — Devcontainer uses typed structs, not
     `map[string]any`. Not on the hot path.
   - All three have `TODO: migrate to native merge` markers.

### Future TODOs (post-merge)

2. **Run cross-validation in CI** — Add `compare_mergo` tests to a CI job. Currently
   behind `//go:build compare_mergo` build tag and only run manually.
3. **Migrate `merge_yaml_functions.go` to native merge** — Eliminate the dual mergo/native
   split. Requires adapting YAML function slice semantics to the native merge API.
4. **Migrate `devcontainer/config_loader.go` to native merge** — Lower priority since
   devcontainer config merging is not performance-critical and uses typed structs.
5. **Add concurrent-contract test** — Document that `deepMergeNative` is not safe for
   concurrent use on the same dst (callers must synchronize).

### No Action Needed

5. `safeCap` max hint — unlikely to be hit in practice.
6. Pointer aliasing — safe for YAML-parsed data.
7. `TF_DATA_DIR` relative path — `componentPath` is correct (matches Terraform's CWD).
8. Workspace recovery dual guard — correct and well-tested.

---

## Summary of Files Changed

| File | Change |
|------|--------|
| `pkg/merge/merge_native.go` | sliceDeepCopy extension fix; precedence fix; aliasing fixes |
| `pkg/merge/merge_native_test.go` | 3 tests updated for extension; new precedence/aliasing tests |
| `pkg/merge/merge_compare_mergo_test.go` | Fix test name; add 5 cross-validation tests |
| `pkg/merge/merge.go` | Replace mergo pre-copy loop with native merge |
| `internal/exec/terraform_utils.go` | `isTerraformCurrentWorkspace` with default handling |
| `internal/exec/terraform_utils_test.go` | 11 sub-tests for workspace detection |
| `internal/exec/terraform_execute_helpers_exec.go` | Workspace recovery with log.Warn |
| `internal/exec/terraform_execute_helpers_pipeline_test.go` | Recovery path tests |
| `internal/exec/terraform_execute_helpers_workspace_test.go` | Error propagation test |
| `internal/exec/testmain_test.go` | Cross-platform subprocess helper |
| `errors/errors.go` | `ErrMergeNilDst`, `ErrMergeTypeMismatch` sentinels |

## Audit Summary

| Category | Count | Key Items |
|---|---|---|
| **Critical** | 1 (fixed) | sliceDeepCopy truncation — silent data loss |
| **High** | 2 (fixed) | Cross-validation expanded, precedence regression fixed |
| **Medium** | 2 (fixed) | Misleading test name, aliasing in mergeSlicesNative |
| **Low** | 2 | safeCap hint, pointer aliasing (both acceptable) |
| **Positive** | 7 | Sound architecture, thorough aliasing prevention, type handling |

The core merge implementation is well-engineered. All critical and high issues have been
fixed. Cross-validation coverage expanded from 4 to 9 equivalence tests.
