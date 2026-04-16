# Deep-Merge Native: Type-Mismatch Guard Regression

**Date:** 2026-03-24
**Introduced by:** PR #2201 (perf: replace mergo with native deep merge)
**Severity:** Critical — breaks `atmos list stacks` and all stack-processing commands for real customer configs
**Reproducer:** `tests/fixtures/scenarios/merge-type-override`

---

## Symptom

```text
Error: failed to execute describe stacks: merge error: merge key "components":
merge key "terraform": merge key "eks/cluster": merge key "vars":
cannot override two slices with different type

File being processed: catalog/eks/cluster/sandbox.yaml
```

All stack-processing commands (`list stacks`, `describe stacks`, `terraform plan`, etc.) fail
when any stack imports contain a type override — e.g., a list replaced by an empty map `{}`.

---

## Root Cause

PR #2201 added two type-mismatch guards to `deepMergeNative` in `pkg/merge/merge_native.go`
that are **too strict** for real-world Atmos configurations:

### Guard 1: Slice→Map override rejected (lines 78-82)

```go
// Guard: reject map src overriding a dst slice
if _, dstIsSlice := dstVal.([]any); dstIsSlice {
    if isMapValue(srcVal) {
        return errUtils.ErrMergeTypeMismatch
    }
}
```

**What it does:** If dst holds a `[]any` (list) and src is any map type, the merge fails.

**Why it's wrong:** In YAML, `{}` (empty map) is a common idiom for "clear this value".
Users override lists with `{}` to disable inherited behavior. Example:

```yaml
# Vendor defaults (list of maps):
allow_ingress_from_vpc_accounts:
  - tenant: core
    stage: auto
  - tenant: core
    stage: network

# Sandbox override (empty map = "no ingress accounts"):
allow_ingress_from_vpc_accounts: {}
```

The old mergo `WithOverride` allowed this — the src value simply replaced dst regardless
of type. The native merge erroneously rejects it.

### Guard 2: Slice→Non-slice override rejected (lines 133-146)

```go
// Type check: if dst holds a slice but src is not a slice, refuse the override.
if _, dstIsSlice := dstVal.([]any); dstIsSlice {
    if _, srcIsSlice := srcVal.([]any); !srcIsSlice {
        normalized := deepCopyValue(srcVal)
        if _, normalizedIsSlice := normalized.([]any); !normalizedIsSlice {
            return errUtils.ErrMergeTypeMismatch
        }
        dst[k] = normalized
        continue
    }
}
```

**What it does:** If dst holds a `[]any` and src is not a slice (and not normalizable to
a slice), the merge fails.

**Why it's wrong:** Same reason — `WithOverride` semantics mean src always wins. A scalar,
null, or map should be able to replace a list. This is standard YAML override behavior.

### The "WithTypeCheck" misconception

The PR comments cite `mergo.WithTypeCheck` as the source of this behavior. However, Atmos
**never used `mergo.WithTypeCheck`**. The old merge call was:

```go
mergo.Merge(&dst, src, mergo.WithOverride)
// Later added: mergo.WithOverride, mergo.WithSliceDeepCopy (or WithAppendSlice)
// But NEVER WithTypeCheck
```

The native implementation adopted `WithTypeCheck` semantics as a "defined contract", but
this was an incorrect assumption. The actual contract is `WithOverride` — **src always
overrides dst for the same key**, regardless of type differences.

---

## Affected Configurations

Any stack config where an import overrides a key's type:

| Pattern | Example | Status |
|---------|---------|--------|
| List → empty map `{}` | `allow_ingress_from_vpc_accounts: {}` | **Broken** |
| List → scalar | `some_list: "disabled"` | **Broken** |
| List → null | `some_list: null` | Likely broken |
| Map → list | N/A | Was already allowed (lines 92-94) |
| Scalar → list | N/A | Was already allowed |
| Scalar → map | N/A | Was already allowed |

The asymmetry is the problem: map→non-map is allowed (line 92-94), but
slice→non-slice is rejected. Both should allow override.

---

## Fix

Remove both type-mismatch guards. With `WithOverride` semantics, src always overrides
dst at leaf level. The only type-aware behavior should be in the recursive paths:

- Both maps → recurse (deep merge).
- Both slices + `appendSlice` → concatenate.
- Both slices + `sliceDeepCopy` → element-wise merge.
- **Everything else → src overrides dst** (deep copy src value into dst).

The `ErrMergeTypeMismatch` sentinel and the `isMapValue` helper may become unused
after this fix.

---

## Test Fixture

`tests/fixtures/scenarios/merge-type-override` — minimal fixture reproducing all type-override
patterns that must work:

- Base component with list-valued vars (e.g., `allowed_accounts`, `rbac_roles`).
- Overlay stack overriding lists with `{}` (empty map), scalar, and null.
- Multi-level import chain: vendor defaults → catalog → environment stacks.
- EKS-like `node_groups` map-of-maps with nested lists layered across imports.

### Reproducing

```bash
cd tests/fixtures/scenarios/merge-type-override
ATMOS_BASE_PATH=. ATMOS_CLI_CONFIG_PATH=. atmos list stacks
```

---

## Verification

After the fix:

1. `atmos list stacks` must succeed with the `merge-type-override` fixture.
2. All existing merge tests must still pass.
3. The cross-validation tests (`go test -tags compare_mergo ./pkg/merge/... -run CompareMergo`)
   should be updated to verify type-override behavior matches mergo `WithOverride`.
4. New test cases for list→map and list→scalar overrides.

---

## Related

- PR #2201: perf: replace mergo with native deep merge (introduced the regression)
- `docs/fixes/2026-03-19-deep-merge-native-fixes.md`: Original fix doc for PR #2201
- `errors/errors.go`: `ErrMergeTypeMismatch` sentinel (may become unused)
- `pkg/merge/merge_native.go`: Lines 78-82 (guard 1) and 133-146 (guard 2)
