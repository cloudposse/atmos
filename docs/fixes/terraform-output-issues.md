# Fixing User-Reported Issues

This document tracks analysis and fixes for user-reported issues in Atmos.

## Issue #1030: Missing Component Results in Silent Failure

**GitHub Issue:** https://github.com/cloudposse/atmos/issues/1030

**Reporter:** joshAtRula

**Status:** Complete

### Problem Statement

When using template functions like `atmos.Component()` or `!terraform.output` to reference outputs from other components, if the **referenced component is removed from the Atmos configuration** (not just the terraform state), Atmos exits with code 1 but produces **confusing or no error message**, even with `ATMOS_LOGS_LEVEL=Trace/Debug` enabled.

### Important Distinction

There are **two different scenarios** that must be handled differently:

1. **Component exists in config, but output doesn't exist** (e.g., terraform output not defined)
   - Should return `nil` (backward compatible)
   - This is the existing behavior and is correct

2. **Component removed from Atmos configuration** (referenced component not in stack manifest)
   - Should return a **clear error** explaining the component is not found
   - This was broken: error was swallowed due to component type fallback logic

### Reproduction Steps

1. Configure Component A to pull outputs from Component B using template functions
2. Verify normal operation (plan/apply works correctly)
3. Remove or comment out Component B references in the manifest (leaving state/resources intact)
4. Attempt to run plan/apply/output on Component A
5. Observe silent/confusing failure with exit code 1

### Root Cause Analysis

The actual root cause was in **component type detection fallback logic**, not in the YAML functions themselves:

#### Error Chain Contamination

When `!terraform.output` or `atmos.Component()` fails because a referenced component is not found:

1. `DescribeComponent` returns `ErrInvalidComponent` error
2. This error was wrapped with `%w`, preserving the error chain
3. In `detectComponentType()`, when checking if the main component exists:
   ```go
   if !errors.Is(err, errUtils.ErrInvalidComponent) {
       return result, err
   }
   // Try Helmfile... then Packer...
   ```
4. Since the error from YAML function processing **also chained to `ErrInvalidComponent`**, the fallback was triggered
5. The system then tried to find the main component as Helmfile, then Packer
6. The **final error** was about the main component not being found as Helmfile/Packer, **not** about the missing referenced component

#### The Chain Problem

```
Original error: ErrInvalidComponent: Could not find component-B in stack...
  ↓ (wrapped with %w)
Executor error: failed to describe component-B: <ErrInvalidComponent chain preserved>
  ↓ (wrapped with %w)
YAML function error: failed to get terraform output...: <ErrInvalidComponent chain preserved>
  ↓
detectComponentType sees ErrInvalidComponent → triggers fallback → confusing error
```

### Solution Design

#### Breaking the Error Chain

The fix is to **break the `ErrInvalidComponent` chain** when wrapping errors from `DescribeComponent` in YAML functions and template functions. This ensures that:

1. Errors about the main component not being found → triggers type fallback (correct)
2. Errors about referenced components not being found → returned immediately with clear message (fixed)

#### Affected Functions

The fix applies to all three functions that can reference other components:

| Function | Location | Status |
|----------|----------|--------|
| `!terraform.output` | `pkg/terraform/output/executor.go` | ✅ Fixed - added `wrapDescribeError()` |
| `!terraform.state` | `internal/exec/terraform_state_utils.go` | ✅ Already correct - uses `%v` instead of `%w` |
| `atmos.Component()` | `internal/exec/template_funcs_component.go` | ✅ Fixed - added `wrapComponentFuncError()` |

#### Implementation Details

**File: `pkg/terraform/output/executor.go`** (for `!terraform.output`)

Added `wrapDescribeError()` helper that breaks the `ErrInvalidComponent` chain:

```go
func wrapDescribeError(component, stack string, err error) error {
    if errors.Is(err, errUtils.ErrInvalidComponent) {
        // Break the ErrInvalidComponent chain by using ErrDescribeComponent as the base.
        // This ensures that errors from YAML function processing don't trigger
        // fallback to try other component types.
        return fmt.Errorf("%w: component '%s' in stack '%s': %s",
            errUtils.ErrDescribeComponent, component, stack, err.Error())
    }
    // For other errors, preserve the full chain.
    return fmt.Errorf("failed to describe component %s in stack %s: %w", component, stack, err)
}
```

**File: `internal/exec/template_funcs_component.go`** (for `atmos.Component()`)

Added `wrapComponentFuncError()` helper with the same pattern for `atmos.Component()`.

**File: `internal/exec/terraform_state_utils.go`** (for `!terraform.state`)

Already correctly breaks the chain using `%v` instead of `%w` for the underlying error (no changes needed):

```go
er := fmt.Errorf("%w `%s` in stack `%s`\nin YAML function: `%s`\n%v",
    errUtils.ErrDescribeComponent, component, stack, yamlFunc, err)
//                                                            ^^-- uses %v, not %w
```

The key difference is `%v` formats the error as a string, breaking the error chain, while `%w` would preserve the chain and allow `errors.Is()` to match the wrapped error.

**File: `internal/exec/yaml_func_utils.go`**

Added debug logging (was already in place from earlier fixes):

```go
case string:
    result, err := processCustomTagsWithContext(...)
    if err != nil {
        log.Debug("Error processing YAML function",
            "value", v,
            "stack", currentStack,
            "error", err.Error(),
        )
        firstErr = err
        return v
    }
    return result
```

### Files Modified

| File | Change |
|------|--------|
| `pkg/terraform/output/executor.go` | Added `wrapDescribeError()` helper, updated `GetOutput` and `fetchAndCacheOutputs` |
| `internal/exec/template_funcs_component.go` | Added `wrapComponentFuncError()` helper, updated `componentFunc` |
| `pkg/terraform/output/executor_test.go` | Added `TestWrapDescribeError_BreaksErrInvalidComponentChain` |
| `internal/exec/template_funcs_component_test.go` | Added `TestWrapComponentFuncError_BreaksErrInvalidComponentChain` |

### Testing

1. **Unit tests:** Added tests verifying `ErrInvalidComponent` chain is broken
2. **Error chain verification:** Tests use `assert.NotErrorIs(t, result, errUtils.ErrInvalidComponent)`
3. **Backward compatibility:**
   - Missing outputs still return `nil` (backward compatible)
   - Other errors (network, auth) preserve full error chain

### Key Insight

The issue was NOT about silent nil returns or error swallowing in YAML functions. Those behaviors are **correct**:
- Missing output with no default → return `nil` (backward compatible)
- Errors properly propagate up

The issue was that the **component type fallback logic** was incorrectly triggered when a **referenced** component was not found, because the error chain included `ErrInvalidComponent`.

---

## Issue #1921: Panic in !terraform.output with Authentication

**GitHub Issue:** https://github.com/cloudposse/atmos/issues/1921

**Reporter:** leoagueci

**Status:** Complete

### Problem Statement

When using `!terraform.output` YAML function with authentication configured (e.g., AWS SSO provider), Atmos panics with the error:

```
panic: authContextWrapper.GetChain should not be called
```

This occurs when processing nested component references that require authentication context propagation.

### Reproduction Steps

1. Configure Atmos authentication (e.g., AWS SSO provider in `atmos.yaml`)
2. Create a component that uses `!terraform.output` to reference another component
3. Run `atmos terraform plan` on the component
4. Observe the panic exception

### Root Cause Analysis

The `authContextWrapper` is a minimal `AuthManager` implementation used to propagate authentication context through nested component processing. It's designed to only provide `GetStackInfo()` for passing `AuthContext` to `ExecuteDescribeComponent`.

When a nested component has its own auth configuration with a `default: true` identity, the `resolveAuthManagerForNestedComponent()` function tries to inherit the identity from the parent:

```go
// In createComponentAuthManager():
if parentAuthManager != nil {
    chain := parentAuthManager.GetChain()  // ← PANIC here
    if len(chain) > 0 {
        identityName = chain[len(chain)-1]
    }
}
```

The `authContextWrapper.GetChain()` method was implemented as a panic stub because it was originally assumed this method would never be called on the wrapper.

### Call Chain Leading to Panic

```
!terraform.output processing
  ↓
componentFunc() creates authContextWrapper from AuthContext
  ↓
ExecuteDescribeComponent(authManager: wrapper)
  ↓
Component has auth config with default identity
  ↓
resolveAuthManagerForNestedComponent(parentAuthManager: wrapper)
  ↓
createComponentAuthManager(parentAuthManager: wrapper)
  ↓
parentAuthManager.GetChain() → PANIC!
```

### Solution Design

Changed `GetChain()` in `authContextWrapper` to return an empty slice instead of panicking:

```go
func (a *authContextWrapper) GetChain() []string {
    defer perf.Track(nil, "exec.authContextWrapper.GetChain")()

    // Return empty slice instead of panicking.
    // This wrapper doesn't track the authentication chain; it only propagates auth context.
    // When used in resolveAuthManagerForNestedComponent, an empty chain means
    // no inherited identity, so the component will use its own defaults.
    return []string{}
}
```

An empty chain means no inherited identity from the wrapper, so the nested component will use its own default identity configuration. This is the correct behavior.

### Files Modified

| File | Change |
|------|--------|
| `internal/exec/terraform_output_utils.go` | Changed `GetChain()` to return empty slice instead of panic |
| `internal/exec/terraform_output_utils_test.go` | Updated test to expect empty slice; added regression test |

### Testing

1. **Regression test:** Added `TestAuthContextWrapper_GetChain_NoLongerPanics` to verify no panic
2. **Behavior verification:** Updated `TestAuthContextWrapper_PanicMethods` to expect empty slice for `GetChain()`
3. **Unit test:** Verifies `GetChain()` returns empty slice

### Key Insight

The `authContextWrapper` was originally designed with panic stubs for all methods except `GetStackInfo()`. However, the auth system evolved to call `GetChain()` on any `AuthManager` passed to nested component resolution. The fix ensures backward compatibility while supporting the auth chain inheritance logic.

---

## Template for Additional Issues

### Issue #XXXX: [Title]

**GitHub Issue:** https://github.com/cloudposse/atmos/issues/XXXX

**Reporter:** [username]

**Status:** [Not Started | In Progress | Complete]

#### Problem Statement

[Description of the issue]

#### Root Cause Analysis

[Technical analysis of what's causing the issue]

#### Solution Design

[Proposed fix approach]

#### Files to Modify

[List of files and changes]

#### Testing

[How the fix will be tested]
