# Stack Overflow When Abstract Components Have `metadata.component`

**Date:** 2026-03-16

**Related Issues:**
- User report: `atmos describe stacks -s <stack>` causes `fatal error: stack overflow` on Atmos versions
  above v1.200.0
- The stack overflow occurs when abstract components define `metadata.component` pointing to the same
  Terraform component directory as a real component that inherits from them

**Affected Atmos Versions:** v1.201.0+ (since the metadata inheritance feature in PR #1812)

**Severity:** High — causes `fatal error: stack overflow` (Go runtime crash, not a recoverable error),
blocking all `atmos describe stacks` usage

---

## Issue Description

When an abstract component has `metadata.component` set to the same value as the real component that
inherits from it, `atmos describe stacks` crashes with a stack overflow:

```yaml
components:
  terraform:
    iam-delegated-roles-defaults:
      metadata:
        component: iam-delegated-roles    # <-- Points to the real component's Terraform directory
        type: abstract
      vars:
        namespace: acme

    iam-delegated-roles:
      metadata:
        component: iam-delegated-roles    # <-- Same Terraform component directory
        type: real
        inherits:
          - iam-delegated-roles-defaults
      vars:
        extra_var: value1
```

### Error Output

```text
runtime: goroutine stack exceeds 1000000000-byte limit
fatal error: stack overflow

goroutine 1 [running]:
github.com/cloudposse/atmos/internal/exec.processBaseComponentConfigInternal(...)
    internal/exec/stack_processor_utils.go:1246 +0x18ac
github.com/cloudposse/atmos/internal/exec.processBaseComponentConfigInternal(...)
    internal/exec/stack_processor_utils.go:1298 +0x364
```

The stack trace shows `processBaseComponentConfigInternal` calling itself recursively at two different
call sites (the `metadata.component` follow and the `metadata.inherits` follow), alternating indefinitely
until the Go goroutine stack limit is exhausted.

### Affected Commands

- `atmos describe stacks` — primary trigger
- Any command that internally calls `ExecuteDescribeStacks`

### Workaround

Remove `metadata.component` from abstract component definitions. Since abstract components cannot be
applied/deployed, `metadata.component` on them is functionally a no-op:

```yaml
components:
  terraform:
    iam-delegated-roles-defaults:
      metadata:
        # component: iam-delegated-roles    # Remove this line
        type: abstract
      vars:
        namespace: acme
```

---

## Root Cause Analysis

### Two-Phase Processing Creates a Circular Reference

The issue involves an interaction between two processing phases in `ExecuteDescribeStacks`:

#### Phase 1: Initial Stack Processing (`processComponentsInParallel`)

During initial processing, each component is processed through `processComponentInheritance` →
`processMetadataInheritance`. The result goes through `mergeComponentConfigurations`
(`stack_processor_merge.go:374`), which writes:

```go
// stack_processor_merge.go:372-375
if result.BaseComponentName != "" {
    comp[cfg.ComponentSectionName] = result.BaseComponentName
}
```

For the abstract component `iam-delegated-roles-defaults` with `metadata.component: iam-delegated-roles`,
this sets `comp["component"] = "iam-delegated-roles"` at the **top level** of the processed component map.

After Phase 1, the processed `terraformSection` contains:

```go
// iam-delegated-roles-defaults (processed):
{
    "component": "iam-delegated-roles",   // <-- Added by mergeComponentConfigurations
    "metadata": {"component": "iam-delegated-roles", "type": "abstract"},
    "vars": {"namespace": "acme"},
}

// iam-delegated-roles (processed):
{
    "component": "iam-delegated-roles",   // <-- Added by mergeComponentConfigurations
    "metadata": {"component": "iam-delegated-roles", "type": "real", "inherits": [...]},
    "vars": {"extra_var": "value1"},
}
```

#### Phase 2: Metadata Inheritance in `describe_stacks.go`

The metadata inheritance feature (PR #1812, `describe_stacks.go:243`) re-processes inheritance on the
**already-processed** `terraformSection`. It calls `ProcessBaseComponentConfig` with this enriched data.

The cache key uses `stackFileName` (e.g., `"deploy"`) while Phase 1 used the computed stack name
(e.g., `"tenant1-ue2-dev"`), causing a **cache miss** and triggering fresh recursion.

#### The Recursion Cycle

In `processBaseComponentConfigInternal`, two recursive call sites create the cycle:

**Call Site 1 (line ~1710):** Follows `baseComponentMap["component"]` (top-level component key):

```go
// Line 1710: Check if the base component itself has a "component" reference
if baseComponentOfBaseComponent, exists := baseComponentMap["component"]; exists {
    // Recurse: follow the component reference chain
    processBaseComponentConfigInternal(..., baseComponent, ..., baseComponentOfBaseComponentString, ...)
}
```

**Call Site 2 (line ~1762):** Follows `metadata.inherits`:

```go
// Line 1742: Process metadata.inherits of the base component
if inheritList, exists := componentMetadata[cfg.InheritsSectionName].([]any); exists {
    for _, v := range inheritList {
        processBaseComponentConfigInternal(..., component, ..., baseComponentFromInheritList, ...)
    }
}
```

**Cycle trace:**

1. Processing `iam-delegated-roles` → finds `inherits: [iam-delegated-roles-defaults]`
2. `processBaseComponentConfigInternal(component="iam-delegated-roles", base="iam-delegated-roles-defaults")`
3. In processed data, `iam-delegated-roles-defaults["component"]` = `"iam-delegated-roles"` → **Call Site 1**
4. `processBaseComponentConfigInternal(component="iam-delegated-roles-defaults", base="iam-delegated-roles")`
5. In processed data, `iam-delegated-roles["component"]` = `"iam-delegated-roles"` → **Call Site 1**
6. `processBaseComponentConfigInternal(component="iam-delegated-roles", base="iam-delegated-roles")`
7. `component == baseComponent` → returns nil ✓ (terminates this branch)
8. But then `iam-delegated-roles["metadata"]["inherits"]` = `["iam-delegated-roles-defaults"]` → **Call Site 2**
9. `processBaseComponentConfigInternal(component="iam-delegated-roles", base="iam-delegated-roles-defaults")`
10. **Back to step 3** — infinite recursion!

The simple `component == baseComponent` guard at the function entry (line 1671) is insufficient because
the cycle involves **three different component name pairs**, and the equality check only catches direct
self-references.

### Why This Was Introduced in v1.201.0

The metadata inheritance feature (PR #1812) added the second call to `ProcessBaseComponentConfig` in
`describe_stacks.go`. Before this feature, the inheritance processing only happened once (in Phase 1),
operating on raw component maps that don't have the top-level `"component"` key. The Phase 2 call
operates on processed maps where `mergeComponentConfigurations` has already added the top-level
`"component"` key, enabling the circular reference.

---

## Fix

Two fixes were implemented, with a third considered but deferred:

### Fix 1: Cycle Detection via Visited-Set (Implemented)

Added a `visited map[string]bool` parameter to `processBaseComponentConfigInternal` that tracks
`(component, baseComponent)` pairs during recursion. If a pair is encountered again, the function
returns a `ErrCircularComponentInheritance` error instead of recursing infinitely.

The visited map is created in `ProcessBaseComponentConfig` (the public entry point) and passed through
all recursive calls:

```go
// ProcessBaseComponentConfig (public entry point):
visited := make(map[string]bool)
err := processBaseComponentConfigInternal(..., visited)

// processBaseComponentConfigInternal (recursive implementation):
visitKey := component + ":" + baseComponent
if visited[visitKey] {
    return fmt.Errorf("%w: '%s' -> '%s' in the stack '%s'",
        ErrCircularComponentInheritance, component, baseComponent, stack)
}
visited[visitKey] = true
```

This ensures that any reference cycle — regardless of complexity (2-node, 3-node, or N-node) — is
detected and reported as a clear error instead of crashing with a stack overflow.

A new sentinel error `ErrCircularComponentInheritance` was added to `errors/errors.go`.

### Fix 2: Skip `metadata.component` on Abstract Components (Implemented)

When processing the `metadata.component` reference in `processBaseComponentConfigInternal`, the fix
skips the component chain resolution if the base component is marked as `type: abstract`. Abstract
components can't be deployed, so their `metadata.component` pointer serves no functional purpose in
inheritance resolution.

The implementation uses the existing `isAbstractComponent()` helper from
`describe_affected_deleted.go`:

```go
// Before following the "component" reference chain, check if this is an abstract component.
isAbstract := isAbstractComponent(baseComponentMap)

if !isAbstract {
    if baseComponentOfBaseComponent, exists := baseComponentMap["component"]; exists {
        // ... existing recursion (component chain resolution) ...
    }
}
```

This directly prevents the specific user-reported pattern where an abstract component's promoted
`component` key creates a circular reference back to the real component.

### Fix 3: Cache Key Consistency (Not Implemented)

The `ProcessBaseComponentConfig` cache key uses `stack:component:baseComponent`. Phase 1 uses the
computed stack name (e.g., `"tenant1-ue2-dev"`) while Phase 2 uses `stackFileName` (e.g., `"deploy"`),
causing a cache miss.

This fix was **deferred** because:
- Phase 1 and Phase 2 operate on different data (raw vs. processed component maps passed via
  `allComponentsMap`), so reusing Phase 1's cached results for Phase 2 could produce incorrect
  inheritance resolution
- Fixes 1 and 2 already fully prevent the stack overflow
- The cache mismatch only causes redundant work, not incorrect behavior

---

## Files Modified

| File                                          | Change                                                                                                                                      |
|-----------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------|
| `errors/errors.go`                            | Added `ErrCircularComponentInheritance` sentinel error                                                                                      |
| `internal/exec/stack_processor_utils.go`      | Added visited-set cycle detection to `processBaseComponentConfigInternal`; skip abstract component chain in `metadata.component` resolution |
| `internal/exec/stack_processor_utils_test.go` | Added 10 test cases across 5 test functions for cycle detection, abstract component skip, metadata inheritance, and deep chain validation |

---

## Backward Compatibility

- Abstract components with `metadata.component` will no longer crash — they will either work correctly
  or return a clear error message about circular references
- Real components with `metadata.component` continue to work as before
- The cycle detection adds negligible overhead (map lookup per recursive call)
- No changes to YAML schema or configuration format

---

## Test Coverage

### Unit Tests Added

- **`TestProcessBaseComponentConfig_CycleDetection`** (4 cases):
  - `direct-cycle-via-component-key` — A references B, B references A via top-level `component` key
  - `cycle-via-inherits` — A inherits from B, B inherits from A
  - `three-component-cycle` — A→B→C→A cycle that the simple `component == baseComponent` guard cannot catch
  - `no-cycle-valid-chain` — valid inheritance chain (A←B) works without false positive

- **`TestProcessBaseComponentConfig_AbstractComponentSkip`** (1 case):
  - Reproduces the exact user-reported pattern: abstract component with promoted `component` key that
    would cause infinite recursion without the abstract skip

- **`TestProcessBaseComponentConfig_DeepChainNoFalsePositive`** (1 case):
  - 3-level inheritance chain (level0←level1←level2) works correctly, verifying that the cycle
    detection doesn't produce false positives on deep but valid chains

- **`TestProcessBaseComponentConfig_AbstractMetadataComponentInherited`** (1 case):
  - Verifies that `metadata.component` on an abstract base component IS properly inherited by real
    components through metadata inheritance. Asserts `BaseComponentMetadata["component"]` is populated,
    vars/backend are inherited, and `metadata.type` is NOT inherited.

- **`TestProcessBaseComponentConfig_AbstractMetadataComponentNotInherited_WhenDisabled`** (1 case):
  - Same pattern but with metadata inheritance disabled — confirms `metadata.component` is NOT
    inherited while regular vars still are.

### Existing Tests

All existing `TestProcessBaseComponentConfig` tests (3 cases) and all metadata inheritance tests
(`TestMetadataFieldsInheritance`, `TestAbstractComponentBackendGeneration`, etc.) continue to pass
unchanged.
