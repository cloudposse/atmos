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

Two issues need to be addressed:

### Issue 1: Cycle Detection in `processBaseComponentConfigInternal`

Add a visited-set parameter to `processBaseComponentConfigInternal` to detect cycles and return an error
instead of recursing infinitely. The visited set tracks `(component, baseComponent)` pairs:

```go
func processBaseComponentConfigInternal(
    // ... existing params ...
    visited map[string]bool,  // NEW: cycle detection
) error {
    if component == baseComponent {
        return nil
    }

    // Cycle detection: check if we've already processed this (component, baseComponent) pair.
    visitKey := component + ":" + baseComponent
    if visited[visitKey] {
        return fmt.Errorf("circular component reference detected: %s -> %s", component, baseComponent)
    }
    visited[visitKey] = true
    defer delete(visited, visitKey)  // Clean up after processing.

    // ... rest of function ...
}
```

This ensures that any reference cycle — regardless of complexity — is detected and reported as a clear
error instead of crashing with a stack overflow.

### Issue 2: Skip `metadata.component` on Abstract Components

When processing the `metadata.component` reference in `processBaseComponentConfigInternal`, skip it if
the component is marked as `type: abstract`. Abstract components can't be deployed, so their
`metadata.component` pointer serves no functional purpose in inheritance resolution:

```go
// Line 1710: Skip component reference for abstract components.
if baseComponentOfBaseComponent, exists := baseComponentMap["component"]; exists {
    // Check if this is an abstract component — skip component chain for abstract types.
    if componentMetadata, ok := baseComponentMap["metadata"].(map[string]any); ok {
        if compType, ok := componentMetadata["type"].(string); ok && compType == "abstract" {
            // Abstract components don't need component chain resolution.
            goto skipComponentChain
        }
    }
    // ... existing recursion ...
}
skipComponentChain:
```

Alternatively, the `describe_stacks.go` metadata inheritance code could check for abstract components
and skip re-processing them entirely.

### Issue 3: Cache Key Consistency

The `ProcessBaseComponentConfig` cache key uses `stack:component:baseComponent`. Phase 1 uses the
computed stack name while Phase 2 uses `stackFileName`. Normalizing the cache key (e.g., always using
the computed stack name) would allow Phase 2 to reuse Phase 1's cached results, avoiding the
re-processing entirely.

---

## Files to Modify

| File                                                              | Change                                                                                                 |
|-------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------|
| `internal/exec/stack_processor_utils.go`                          | Add visited-set cycle detection to `processBaseComponentConfigInternal`; skip abstract component chain |
| `internal/exec/describe_stacks.go`                                | Consider normalizing cache key or skipping abstract components in metadata inheritance                 |
| `internal/exec/stack_processor_utils_test.go`                     | Tests for cycle detection, abstract component handling                                                 |
| `internal/exec/abstract_component_describe_stacks_test.go`        | Integration test reproducing the stack overflow                                                        |
| `tests/fixtures/scenarios/abstract-component-metadata-component/` | Test fixture with abstract + real component pair                                                       |

---

## Backward Compatibility

- Abstract components with `metadata.component` will no longer crash — they will either work correctly
  or return a clear error message about circular references
- Real components with `metadata.component` continue to work as before
- The cycle detection adds negligible overhead (map lookup per recursive call)
- No changes to YAML schema or configuration format

---

## Test Plan

### Unit Tests

- `TestProcessBaseComponentConfig_CycleDetection` — verify circular references are detected and reported
- `TestProcessBaseComponentConfig_AbstractComponentSkip` — verify abstract components skip component chain
- `TestProcessBaseComponentConfig_DeepInheritanceChain` — verify non-circular deep chains still work

### Integration Tests

- `TestAbstractComponentMetadataComponent_DescribeStacks` — reproduce the exact user-reported config pattern
- `TestAbstractComponentMetadataComponent_DescribeComponent` — verify `describe component` works with
  abstract components that have `metadata.component`
- Verify all existing inheritance tests continue to pass
