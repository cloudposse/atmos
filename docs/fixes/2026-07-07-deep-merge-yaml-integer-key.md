# Deep-Merge Native: Unquoted-Integer YAML Map Key Replaces Instead of Merging

**Date:** 2026-07-07
**Introduced by:** PR #2201 (perf: replace mergo with native deep merge)
**Severity:** High — silently drops catalog config wherever a stack overrides a subset of keys
under a map whose parent key is an unquoted integer
**Issue:** [cloudposse/atmos#2376](https://github.com/cloudposse/atmos/issues/2376)
**Reproducer:** `pkg/merge/merge_yaml_integer_key_test.go`

---

## Symptom

Given a catalog that defines a map keyed by an unquoted integer:

```yaml
# catalog/nss.yaml
components:
  terraform:
    ec2-custom-nss-web:
      vars:
        eni:
          1:                             # unquoted integer key
            description: NSS system interface
            sg_rules:
              ip4_cidr_blocks:
                egress,tcp,443: [0.0.0.0/0]
```

And a stack that adds/overrides a subset of keys under that same `eni.1` entry:

```yaml
# stack/ue1-pci-it.yaml
components:
  terraform:
    ec2-custom-nss-web:
      vars:
        eni:
          1:
            security_groups:
              - sg-07dd44f6e72142c44
            sg_rules:
              ip4_cidr_blocks:
                egress,tcp,8601: [10.107.0.0/17]
```

`atmos describe component ec2-custom-nss-web -s ue1-pci-it` should deep-merge `eni.1`, but
instead the stack's `eni.1` **wholesale replaces** the catalog's `eni.1` — `description` and
`egress,tcp,443` are silently dropped:

```yaml
eni:
  1:
    security_groups:
      - sg-07dd44f6e72142c44
    sg_rules:
      ip4_cidr_blocks:
        egress,tcp,8601: [10.107.0.0/17]      # catalog's egress,tcp,443 entry is gone
```

**Workaround:** quoting the key (`"1":` instead of `1:`) in both files forces a string key and
the native merge handles it correctly — but this requires touching every catalog/stack file that
uses a numeric key, which is impractical at scale.

The regression only affects the specific mapping level whose key is an unquoted non-string
scalar; sibling maps with all-string keys merge correctly, which is why the bug is easy to miss
in small configs and only surfaces on integer-keyed maps like NIC/ENI indices, port numbers, or
numeric identifiers used as map keys.

---

## Root Cause

`gopkg.in/yaml.v3` decodes a YAML mapping into `map[string]interface{}` **only when every key in
that mapping resolves to the `!!str` tag** (see `isStringMap` in `decode.go`, used by the
`mapping()` decoder at `gopkg.in/yaml.v3@v3.0.1/decode.go:789-795`). An unquoted `1` resolves to
tag `!!int`, so the entire mapping containing that key — here, the value of `eni` — decodes as
`map[interface{}]interface{}` instead, with the key stored as `int(1)`. This is a per-mapping-node
decision: only the specific mapping that contains the non-string key is affected, everything
above and below it in the document decodes normally.

`deepMergeNative` in `pkg/merge/merge_native.go` only fast-recurses when **both** sides of a key
are exactly `map[string]any` (line 56). Anything else falls through to `deepCopyValue` →
`normalizeValueReflect` → `normalizeMapReflect` in `pkg/merge/merge.go`, which — before this fix —
branched only on whether the map's key kind was `reflect.String`:

```go
// Non-string keys: copy to same type, ensuring value type matches Elem().
if keyKind != reflect.String {
    return copyNonStringKeyMap(rv, iter)
}
```

This conflated two unrelated cases:

- **Genuinely-typed Go maps with a concrete non-string key type**, e.g. `map[int]schema.Provider`
  — these have no `map[string]any` shape to merge into, so preserving them as opaque typed leaves
  (via `copyNonStringKeyMap`) is correct. Covered by `TestDeepCopyMap_TypedMaps` in
  `pkg/merge/merge_test.go`.
- **`map[interface{}]interface{}`** produced by yaml.v3 for a mapping with a dynamic (non-string)
  key — here `Key().Kind()` is `reflect.Interface`, not a concrete type. This is exactly the shape
  produced by an unquoted integer YAML key and needs to be **stringified**, not preserved, to
  match the merge's `map[string]any` contract.

Because both cases hit `copyNonStringKeyMap` identically, a YAML-decoded
`map[interface{}]interface{}` kept its original shape (`int` keys, `map[interface{}]interface{}`
container) instead of becoming `map[string]any`. When `deepMergeNative` later compared `dstVal`
and `srcVal` for that key, the `map[string]any` type assertions (merge_native.go:56-57, 70-71)
failed for whichever side hadn't independently been normalized, so the code fell to the
"type mismatch: src overrides dst" branch — the same override path used for legitimate type
changes like list→scalar — and replaced the whole integer-keyed submap instead of recursing into
it.

The codebase already has precedent for stringifying `map[interface{}]interface{}` keys:
`pkg/list/list_instances.go`'s `sanitizeForJSON` converts them via `fmt.Sprintf("%v", k)` for JSON
output. The merge package's normalization path just hadn't been taught to do the same.

---

## Fix

`normalizeMapReflect` (`pkg/merge/merge.go`) now treats `keyKind == reflect.Interface` the same
as `reflect.String`: every key is stringified — via `.String()` for concrete string keys, via
`fmt.Sprintf("%v", ...)` for dynamic interface keys — and the result is a `map[string]any`, with
values recursively normalized through the existing `deepCopyValue` call. Only concrete,
non-string, non-interface key kinds (e.g. `reflect.Int` on a genuinely typed `map[int]T`) still go
through `copyNonStringKeyMap` and keep their original type.

```go
// String keys, or interface{} keys (e.g. yaml.v3 decodes a mapping with an unquoted
// non-string key like `1:` as map[interface{}]interface{}, not map[string]interface{}):
// stringify every key so this collapses onto the same map[string]any shape as an
// all-string-keyed sibling map, letting deepMergeNative's map[string]any fast path recurse
// into it instead of treating it as an opaque leaf that gets replaced wholesale.
result := make(map[string]any, rv.Len())
for iter.Next() {
    key := iter.Key()
    var keyStr string
    if key.Kind() == reflect.String {
        keyStr = key.String()
    } else {
        keyStr = fmt.Sprintf("%v", key.Interface())
    }
    normalizedVal := deepCopyValue(iter.Value().Interface())
    if existing, collided := result[keyStr]; collided {
        if existingMap, ok := existing.(map[string]any); ok {
            if newMap, ok := normalizedVal.(map[string]any); ok {
                _ = deepMergeNative(existingMap, newMap, false, false)
                continue
            }
        }
    }
    result[keyStr] = normalizedVal
}
return result
```

No other call sites needed changes: `deepCopyValue` (merge.go:54) and `deepMergeNative`
(merge_native.go:41) already route any value that isn't a fast-path `map[string]any`/`[]any`/
primitive through this function, on both the "key only in src" insertion path and the "both sides
are maps after normalization" recursion path.

### Follow-up: collision on stringified keys (PR #2700 review)

Stringifying interface{} keys introduces a new (narrow) risk that didn't exist before this fix:
two *distinct* original keys can stringify to the same string — e.g. YAML `1` (int) and `1.0`
(float) both format to `"1"` via `fmt.Sprintf("%v", ...)`. Before this fix, `map[interface{}]interface{}`
kept its native keys, so such a collision was impossible; a naive stringify-and-overwrite would
silently drop one entry, and since Go map iteration order is unspecified, which entry survived
would vary run to run.

The fix above detects the collision (`result[keyStr]` already set) and, when both the existing and
new values are `map[string]any`, merges them via `deepMergeNative` instead of overwriting — so no
data is lost. Non-map collisions (rare in practice) still fall back to overwrite, which is no worse
than not having the case at all. See
`TestNormalizeMapReflect_CollidingStringifiedKeysAreMerged` for the regression test.

---

## Tests

`pkg/merge/merge_yaml_integer_key_test.go` (new):

- `TestDeepMergeNative_YAMLIntegerMapKey` — reproduces the bug end-to-end through real
  `gopkg.in/yaml.v3` unmarshaling of a catalog/stack pair sharing an unquoted-integer map key,
  confirming the merge preserves the catalog-only key, the stack-only key, and applies the
  stack's override to the shared key. Verified failing before the fix (catalog-only key dropped)
  and passing after.
- `TestNormalizeMapReflect_InterfaceKeyedMap` — unit-tests the normalization contract directly on
  a `map[interface{}]interface{}` with mixed int/string dynamic keys, asserting it becomes
  `map[string]any` with stringified keys — distinct from `TestDeepCopyMap_TypedMaps`, which covers
  `map[int]T` and must keep its concrete type.

---

## Verification

1. `go build ./... && go test ./pkg/merge/...` — new tests pass, no regressions.
2. `go test ./pkg/config/... ./pkg/stack/... ./pkg/list/...` — no regressions in dependent
   packages.
3. `TestDeepCopyMap_TypedMaps` and `pkg/merge/merge_type_override_test.go` continue to pass
   unchanged, confirming concrete-typed non-string-key maps (e.g. `map[int]string`) still
   preserve their type.

---

## Related

- PR #2201: perf: replace mergo with native deep merge (introduced the regression).
- PR #2248 / `docs/fixes/2026-03-24-deep-merge-type-mismatch-regression.md`: prior partial
  regression fix — addressed list↔map type-mismatch guards, but not this map-key normalization
  gap.
- `docs/fixes/2026-03-19-deep-merge-native-fixes.md`: original fix doc for PR #2201.
- `pkg/merge/merge.go`: `normalizeMapReflect`, `copyNonStringKeyMap`, `deepCopyValue`.
- `pkg/merge/merge_native.go`: `deepMergeNative`.
- `pkg/list/list_instances.go`: `sanitizeForJSON` — prior art for stringifying
  `map[interface{}]interface{}` keys.
