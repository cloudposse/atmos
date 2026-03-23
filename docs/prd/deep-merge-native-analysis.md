# Deep Merge Native Implementation — Analysis and Findings

**Date:** 2026-03-23

**Related PR:** #2201 — `perf(merge): replace mergo pre-copy loop with reflection-free native deep merge (3.5× faster)`

**Severity:** This PR touches the core stack configuration merge pipeline. Every stack
resolution in Atmos passes through this code. Any bug here affects every single `atmos`
command that processes stacks.

---

## What the PR Does

Replaces the pre-merge deep-copy loop (which called `mergo.Merge` after copying each input)
with a native Go implementation that deep-copies only the first input and merges subsequent
inputs in-place with leaf-level copying. This reduces N full `DeepCopyMap` calls to 1,
achieving ~3.5× speedup on the ~118k+ merge calls per stack resolution run.

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

### Files Changed

| Area | Files | Risk |
|------|-------|------|
| **Core merge** | `pkg/merge/merge.go`, `pkg/merge/merge_native.go` | **Critical** — affects all stack resolution |
| **Merge tests** | 5 test files in `pkg/merge/` | Coverage |
| **Terraform workspace** | `terraform_execute_helpers_exec.go`, `terraform_utils.go` | **Medium** — separate concern bundled in PR |
| **Test infrastructure** | `testmain_test.go`, `preconditions.go`, 6 test files | Low |
| **Docs/blog** | fix doc, blog post, roadmap | None |

---

## Core Merge Analysis

### Native Merge Semantics (merge_native.go)

The `deepMergeNative(dst, src)` function implements these rules:

| Scenario | Behavior | Correct? |
|---|---|---|
| Both map | Recursive merge | ✅ |
| Src map, dst not map | Src overrides dst | ✅ (matches mergo WithOverride) |
| Src not map, dst map | Src overrides dst | ✅ (matches mergo WithOverride) |
| Src slice, dst map | **Error** — `ErrMergeTypeMismatch` | ⚠️ Asymmetric |
| Src nil | Override dst with nil | ✅ (matches mergo WithOverride) |
| Src typed map | Normalize to `map[string]any` via reflection | ✅ |
| Src typed slice | Normalize to `[]any` via reflection | ✅ |

### Slice Merge Modes

| Mode | Behavior | Notes |
|---|---|---|
| Default | Src slice replaces dst slice | Standard |
| `appendSlice` | Dst + src elements concatenated | Both deep-copied |
| `sliceDeepCopy` | Element-wise merge for overlapping indices | **Known divergence from mergo** |

### Known Divergence: `sliceDeepCopy` Truncation

When `sliceDeepCopy=true` and src has more elements than dst, the native implementation
**drops extra src elements**. Mergo's `WithSliceDeepCopy` would **extend** the slice.

This is explicitly documented in the code (line 184-188 of merge_native.go) as intentional,
claiming it "matches the semantics previously relied on by callers."

**Risk assessment:** If any stack config relies on extending slices via the merge strategy
(e.g., appending new list items from an overlay), this would be a behavioral change. The
`sliceDeepCopy` mode is only used when `list_merge_strategy: deep` is configured, which
is rare.

### Aliasing Prevention

The implementation correctly prevents aliasing at every insertion point:
- `deepCopyValue(srcVal)` before storing into dst
- `deepCopySlice(dst[i])` before recursing in `mergeSlicesNative`
- `deepCopyMap` for the first input in the `Merge` function
- Tested via `TestDeepMergeNative_SrcDoesNotMutateSrcData`

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

## Findings

### HIGH — Cross-validation test coverage is narrow

The `merge_compare_mergo_test.go` file only tests **4 equivalence cases** and **1 divergence
case** against mergo. These tests are behind a build tag (`compare_mergo`) and are **not run
in CI**.

Missing cross-validation scenarios:
- `appendSlice` mode comparison with mergo
- `sliceDeepCopy` mode with varying src/dst lengths
- Multiple inputs (3+) with nested maps
- Typed slices (`[]string`) replacing existing `[]any` values
- Maps with deeply nested mix of slices-of-maps-of-slices

**Recommendation:** Expand cross-validation coverage and run in CI (even if gated behind
a slower test suite like `make testacc`).

### HIGH — `mergo` still used in YAML function merging

`pkg/merge/merge_yaml_functions.go` (lines 177 and 263) still uses `dmergo.Merge()` with
`mergo.WithOverride`. This creates a split where the main merge pipeline uses native merge
but YAML function merging uses mergo. If these code paths interact (and they do — YAML
functions produce maps that are later merged via the native path), there could be subtle
differences in how typed values are normalized.

**Recommendation:** Document this split explicitly. Plan eventual migration of
`merge_yaml_functions.go` to native merge.

### MEDIUM — Misleading test case name

In `merge_compare_mergo_test.go` line 60, the case is named
`"nil value in src map entry is skipped"` but both mergo and native actually DO override
with nil (the assertion verifies the value IS nil in the result). The name should be
`"nil value in src map entry overrides dst"`.

### MEDIUM — No concurrent test for `deepMergeNative`

While `DeepCopyMap` has a concurrent test, `deepMergeNative` itself does not. Since it
mutates `dst` in place, concurrent calls on the same dst would race. This is expected
behavior (caller must synchronize), but there is no test documenting this contract.

### LOW — `safeCap` max capacity hint

`safeCap` uses `maxCapHint` of `1 << 24` (16M entries). For `make([]any, 0, safeCap(a, b))`,
this could allocate ~128MB of pointer-sized memory. This is a capacity hint only (not actual
length) and the overflow protection is correct. Unlikely to be hit in practice.

### LOW — Pointer aliasing in `deepCopyTypedValue`

Pointers are not dereferenced — the function returns the original pointer. For YAML-parsed
data (which contains no Go pointers), this is fine. If any code path produces maps containing
pointer values, mutation through the alias could corrupt data.

---

## Terraform Workspace Recovery (Separate Concern)

The PR bundles a Terraform workspace recovery fix:

When `terraform workspace new` fails with exit code 1 AND the `.terraform/environment`
file already names the target workspace, the error is downgraded to a warning and execution
continues. This handles a real edge case where the environment file names the workspace but
the state directory was deleted.

### New Function: `isTerraformCurrentWorkspace`

Reads `.terraform/environment` (or `TF_DATA_DIR`-relative equivalent) and compares to the
requested workspace. Handles `TF_DATA_DIR` absolute/relative, missing file (returns `true`
for "default" workspace), empty file, trailing whitespace.

**Risk:** Medium-low. The dual guard (exit code 1 AND file match) makes false positives
unlikely. Well-tested with 11 sub-tests.

---

## Recommendations

### Must Fix Before Merge

1. **Fix misleading test name** — `"nil value in src map entry is skipped"` → `"nil value
   in src map entry overrides dst"`.

### Should Fix Before Merge

2. **Expand cross-validation** — Add `appendSlice` and `sliceDeepCopy` mode comparisons
   with mergo to `merge_compare_mergo_test.go`.
3. **Document the mergo/native split** — Add a comment in `merge_yaml_functions.go`
   explaining that it still uses mergo and why.

### Should Fix After Merge

4. **Run cross-validation in CI** — Add `compare_mergo` tests to a CI job (can be slow).
5. **Migrate `merge_yaml_functions.go`** — Eventually use native merge instead of mergo.
6. **Add concurrent-contract test** — Document that `deepMergeNative` is not safe for
   concurrent use on the same dst.

### No Action Needed

7. The `sliceDeepCopy` truncation behavior is intentionally documented and matches
   caller expectations.
8. The workspace recovery logic is correct with appropriate guards.
9. The `safeCap` max hint is unlikely to be hit.
10. Pointer aliasing is safe for YAML data.

---

## Summary

| Category | Count | Key Items |
|---|---|---|
| **Critical** | 0 | No data-corruption bugs found |
| **High** | 2 | Narrow cross-validation, mergo/native split |
| **Medium** | 2 | Misleading test name, no concurrent test |
| **Low** | 2 | safeCap hint, pointer aliasing |
| **Positive** | 7 | Sound architecture, thorough aliasing prevention, type handling |

The core merge implementation is well-engineered and correct for the tested scenarios.
The biggest risk is the narrow scope of cross-validation testing against mergo — for the
deepest merge pipeline in Atmos, broader equivalence testing would provide stronger
confidence.
