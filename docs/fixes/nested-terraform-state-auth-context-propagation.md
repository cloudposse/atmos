# Fix: AuthContext Not Propagating Through Nested !terraform.state Functions

## Problem Statement

When executing Atmos commands with authentication enabled, nested `!terraform.state` YAML functions fail with IMDS timeout errors even though the top-level command has valid authenticated credentials. This occurs when a component's configuration contains `!terraform.state` functions that reference other components which themselves contain `!terraform.state` functions (nested/chained evaluation).

## Symptoms

**Error Message:**
```
failed to read Terraform state for component tgw/hub in stack core-use2-network
in YAML function: !terraform.state tgw/hub core-use2-network transit_gateway_id
failed to get object from S3: operation error S3: GetObject, get identity: get credentials:
failed to refresh cached credentials, no EC2 IMDS role found, operation error ec2imds: GetMetadata,
request canceled, context deadline exceeded
```

**Debug Output Pattern:**
```
DEBU  Authentication chain discovered identity=core-network/terraform
DEBU  Retrieved AWS credentials from keyring alias=core-network/terraform
DEBU  Set AWS auth context profile=core-network/terraform
DEBU  Executing Atmos YAML function function="!terraform.state tgw/attachment transit_gateway_vpc_attachment_id"
DEBU  Found component 'tgw/attachment' in the stack 'core-use2-network'
DEBU  Executing Atmos YAML function function="!terraform.state tgw/hub core-use2-network transit_gateway_id"
DEBU  Using standard AWS SDK credential resolution (no auth context provided)  ← ⚠️ BUG: Lost auth context!
DEBU  Failed to read Terraform state file from the S3 bucket error="...context deadline exceeded"
```

## Reproduction Case

### Stack Configuration

**Component 1: tgw/routes** (Top-level component being deployed)
```yaml
# orgs/ins/core/network/us-east-2/foundation.yaml
components:
  terraform:
    tgw/routes:
      vars:
        transit_gateway_route_tables:
          - transit_gateway_route_table_id: !terraform.state tgw/hub core-use2-network transit_gateway_route_table_id
            routes:
              # This triggers nested evaluation
              - attachment_id: !terraform.state tgw/attachment transit_gateway_vpc_attachment_id
```

**Component 2: tgw/attachment** (Referenced component with nested !terraform.state)
```yaml
# catalog/tgw/attachment/defaults.yaml
components:
  terraform:
    tgw/attachment:
      vars:
        enabled: true
        name: tgw-attachment
        # These nested functions fail because AuthManager is not propagated
        transit_gateway_id: !terraform.state tgw/hub core-use2-network transit_gateway_id
        transit_gateway_route_table_id: !terraform.state tgw/hub core-use2-network transit_gateway_route_table_id
```

**Component 3: tgw/hub** (Final component in the chain)
```yaml
components:
  terraform:
    tgw/hub:
      vars:
        enabled: true
        name: tgw-hub
        # No further !terraform.state functions
```

### Execution Command

```bash
atmos terraform apply tgw/routes -s core-use2-network
```

**Expected Behavior:**
All `!terraform.state` functions should use the authenticated `core-network/terraform` identity credentials.

**Actual Behavior:**
- Level 1: `!terraform.state tgw/hub` ✅ Works (uses AuthContext)
- Level 2: `!terraform.state tgw/attachment` ✅ Works (uses AuthContext)
- Level 3: `!terraform.state tgw/hub` inside `tgw/attachment` config ❌ Fails (no AuthContext)

## Root Cause Analysis

### Execution Flow

```
1. User Command
   └─ atmos terraform apply tgw/routes -s core-use2-network
       └─ internal/exec/terraform.go:ExecuteTerraform()

2. AuthManager Creation (✅ Works)
   └─ auth.CreateAndAuthenticateManager(identityName, authConfig, selectValue)
       ├─ Creates AuthManager
       ├─ Authenticates with core-network/terraform identity
       └─ Populates AuthContext in configAndStacksInfo

3. Stack Processing (✅ Works)
   └─ ProcessStacks(atmosConfig, info, ..., authManager)
       └─ Passes AuthManager through pipeline
       └─ AuthContext available in configAndStacksInfo

4. Level 1: Process tgw/routes Component Config (✅ Works)
   └─ ProcessComponentConfig(..., authManager)
       └─ Populates configAndStacksInfo.AuthContext
       └─ processCustomTagsWithContext(atmosConfig, input, currentStack, resolutionCtx, stackInfo)
           └─ stackInfo contains AuthContext

5. Level 1: Evaluate !terraform.state tgw/attachment (✅ Works)
   └─ processTagTerraformStateWithContext(atmosConfig, input, currentStack, resolutionCtx, stackInfo)
       ├─ Extracts authContext from stackInfo (line 90-93)
       └─ Calls stateGetter.GetState(..., authContext) ✅ AuthContext passed

6. Level 2: GetTerraformState() (❌ BUG HERE)
   └─ internal/exec/terraform_state_utils.go:63-70
       └─ ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
               Component:            "tgw/attachment",
               Stack:                "core-use2-network",
               ProcessTemplates:     true,
               ProcessYamlFunctions: true,
               Skip:                 nil,
               AuthManager:          nil,  ← ⚠️ BUG: Should pass authManager!
           })

7. Level 2: Process tgw/attachment Component Config (❌ No AuthManager)
   └─ ExecuteDescribeComponentWithContext(params)
       └─ params.AuthManager == nil
       └─ configAndStacksInfo.AuthContext NOT populated (line 468-477)

8. Level 3: Evaluate !terraform.state tgw/hub inside tgw/attachment (❌ Fails)
   └─ processTagTerraformStateWithContext(atmosConfig, input, currentStack, nil, nil)
       ├─ stackInfo is nil → authContext is nil
       └─ Calls stateGetter.GetState(..., nil) ❌ No AuthContext

9. Level 3: GetTerraformState() with nil authContext (❌ Fails)
   └─ internal/aws_utils/aws_utils.go:85
       └─ "Using standard AWS SDK credential resolution (no auth context provided)"
       └─ AWS SDK falls back to IMDS → timeout error!
```

### Code Location of Bug

**File:** `internal/exec/terraform_state_utils.go`
**Lines:** 63-70

```go
componentSections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
    Component:            component,
    Stack:                stack,
    ProcessTemplates:     true,
    ProcessYamlFunctions: true,
    Skip:                 nil,
    AuthManager:          nil,  // ❌ BUG: AuthManager not passed
})
```

**Impact:**
When `ExecuteDescribeComponent` is called without an `AuthManager`, the subsequent `ExecuteDescribeComponentWithContext` function (line 468-477 in `describe_component.go`) does NOT populate `configAndStacksInfo.AuthContext`, causing all nested `!terraform.state` evaluations to fail.

### Why This Happens

The `GetTerraformState` function receives an `authContext` parameter but does NOT have access to the `AuthManager`. When it needs to process nested components, it calls `ExecuteDescribeComponent` without an `AuthManager`, breaking the authentication chain.

**The missing link:**
```
authContext (credentials) ← Derived from → AuthManager (authentication manager)
       ↓                                              ↓
  Passed to GetTerraformState              NOT available in GetTerraformState
       ↓                                              ↓
  Cannot be propagated                        Needed for ExecuteDescribeComponent
```

## Proposed Solutions

### Option 1: Add AuthManager to ConfigAndStacksInfo (Recommended)

**Approach:** Store the `AuthManager` reference directly in the `ConfigAndStacksInfo` struct, allowing it to flow naturally through the entire execution pipeline alongside `AuthContext`.

**Advantages:**
- ✅ Clean architectural solution
- ✅ AuthManager available at all levels of execution
- ✅ Consistent with existing pattern (AuthContext is already in ConfigAndStacksInfo)
- ✅ No global state
- ✅ Thread-safe by design
- ✅ Easy to test (pass mock AuthManager in tests)

**Disadvantages:**
- ⚠️ Requires changes to multiple function signatures
- ⚠️ Requires interface changes (TerraformStateGetter)
- ⚠️ More files to modify

**Implementation:**

#### Step 1: Add AuthManager field to ConfigAndStacksInfo

**File:** `pkg/schema/schema.go`

```go
type ConfigAndStacksInfo struct {
    // ... existing fields ...

    // AuthContext holds active authentication credentials for cloud providers.
    // This is the SINGLE SOURCE OF TRUTH for auth credentials.
    // ComponentEnvSection/ComponentEnvList are derived from this context.
    AuthContext *AuthContext

    // AuthManager holds the authentication manager instance used to create AuthContext.
    // This is needed for nested operations (e.g., nested !terraform.state functions)
    // that require re-authentication or access to the original authentication chain.
    // Type is 'any' to avoid circular dependency with pkg/auth.
    AuthManager any

    // ... remaining fields ...
}
```

#### Step 2: Update TerraformStateGetter interface

**File:** `internal/exec/terraform_state_getter.go`

```go
type TerraformStateGetter interface {
    // GetState retrieves terraform state for a component.
    GetState(
        atmosConfig *schema.AtmosConfiguration,
        yamlFunc string,
        stack string,
        component string,
        output string,
        skipCache bool,
        authContext *schema.AuthContext,
        authManager any,  // NEW: Pass AuthManager for nested operations
    ) (any, error)
}

func (d *defaultStateGetter) GetState(
    atmosConfig *schema.AtmosConfiguration,
    yamlFunc string,
    stack string,
    component string,
    output string,
    skipCache bool,
    authContext *schema.AuthContext,
    authManager any,  // NEW parameter
) (any, error) {
    defer perf.Track(atmosConfig, "exec.defaultStateGetter.GetState")()

    return GetTerraformState(atmosConfig, yamlFunc, stack, component, output, skipCache, authContext, authManager)
}
```

#### Step 3: Update GetTerraformState function

**File:** `internal/exec/terraform_state_utils.go`

```go
func GetTerraformState(
    atmosConfig *schema.AtmosConfiguration,
    yamlFunc string,
    stack string,
    component string,
    output string,
    skipCache bool,
    authContext *schema.AuthContext,
    authManager any,  // NEW parameter
) (any, error) {
    defer perf.Track(atmosConfig, "exec.GetTerraformState")()

    stackSlug := fmt.Sprintf("%s-%s", stack, component)

    // Cache lookup (unchanged)
    if !skipCache {
        backend, found := terraformStateCache.Load(stackSlug)
        if found && backend != nil {
            log.Debug("Cache hit", ...)
            result, err := tb.GetTerraformBackendVariable(atmosConfig, backend.(map[string]any), output)
            if err != nil {
                er := fmt.Errorf("%w %s for component `%s` in stack `%s`\nin YAML function: `%s`\n%v",
                    errUtils.ErrEvaluateTerraformBackendVariable, output, component, stack, yamlFunc, err)
                return nil, er
            }
            return result, nil
        }
    }

    // ✅ FIX: Pass AuthManager to ExecuteDescribeComponent
    componentSections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
        Component:            component,
        Stack:                stack,
        ProcessTemplates:     true,
        ProcessYamlFunctions: true,
        Skip:                 nil,
        AuthManager:          authManager,  // ✅ Propagate AuthManager!
    })
    if err != nil {
        er := fmt.Errorf("%w `%s` in stack `%s`\nin YAML function: `%s`\n%v",
            errUtils.ErrDescribeComponent, component, stack, yamlFunc, err)
        return nil, er
    }

    // Rest of function unchanged...
}
```

#### Step 4: Update processTagTerraformStateWithContext

**File:** `internal/exec/yaml_func_terraform_state.go`

```go
func processTagTerraformStateWithContext(
    atmosConfig *schema.AtmosConfiguration,
    input string,
    currentStack string,
    resolutionCtx *ResolutionContext,
    stackInfo *schema.ConfigAndStacksInfo,
) any {
    defer perf.Track(atmosConfig, "exec.processTagTerraformStateWithContext")()

    log.Debug("Executing Atmos YAML function", "function", input)

    // ... existing parsing code ...

    // Extract authContext and authManager from stackInfo if available.
    var authContext *schema.AuthContext
    var authManager any
    if stackInfo != nil {
        authContext = stackInfo.AuthContext
        authManager = stackInfo.AuthManager  // ✅ NEW: Extract AuthManager
    }

    value, err := stateGetter.GetState(atmosConfig, input, stack, component, output, false, authContext, authManager)
    errUtils.CheckErrorPrintAndExit(err, "", "")
    return value
}
```

#### Step 5: Update all callers

**Files to update:**
- `internal/exec/terraform.go` - Set `configAndStacksInfo.AuthManager = authManager` after authentication
- `internal/exec/terraform_output_utils.go` - Similar pattern for `!terraform.output`
- All mock generators and tests

**Example in terraform.go:**

```go
func ExecuteTerraform(info schema.ConfigAndStacksInfo) error {
    // ... existing code ...

    authManager, err := auth.CreateAndAuthenticateManager(
        info.Identity,
        mergedAuthConfig,
        cfg.IdentityFlagSelectValue)
    if err != nil {
        return err
    }

    // ... existing code to store authenticated identity for hooks ...

    // ✅ NEW: Store AuthManager in configAndStacksInfo for nested operations
    info.AuthManager = authManager

    // Process stacks with AuthManager
    info, err = ProcessStacks(&atmosConfig, info, processTemplates, processYamlFunctions, skip, authManager)
    if err != nil {
        return err
    }

    // ... rest of function ...
}
```

### Option 2: Global AuthManager Registry (Alternative)

**Approach:** Create a thread-safe global registry to store the current `AuthManager` keyed by stack+component, allowing `GetTerraformState` to retrieve it without signature changes.

**Advantages:**
- ✅ Minimal code changes
- ✅ No interface signature changes
- ✅ Surgical fix to specific problem

**Disadvantages:**
- ❌ Global state (harder to test)
- ❌ Potential race conditions if not carefully implemented
- ❌ Less explicit (AuthManager availability not clear from signatures)
- ❌ Key collision risk (stack+component may not be unique in all cases)
- ❌ Cleanup required (memory leaks if not cleared properly)

**Implementation:**

#### Step 1: Create AuthManager registry

**File:** `internal/exec/auth_manager_context.go` (NEW)

```go
package exec

import (
    "sync"
)

var (
    authManagerRegistry = &AuthManagerRegistry{
        managers: make(map[string]any),
    }
)

// AuthManagerRegistry provides thread-safe storage for AuthManager instances.
// This allows nested YAML function evaluations to access the AuthManager without
// passing it through all intermediate function signatures.
type AuthManagerRegistry struct {
    mu       sync.RWMutex
    managers map[string]any
}

// SetCurrentAuthManager stores an AuthManager for a given execution context.
func SetCurrentAuthManager(key string, manager any) {
    authManagerRegistry.mu.Lock()
    defer authManagerRegistry.mu.Unlock()
    authManagerRegistry.managers[key] = manager
}

// GetCurrentAuthManager retrieves an AuthManager for a given execution context.
func GetCurrentAuthManager(key string) any {
    authManagerRegistry.mu.RLock()
    defer authManagerRegistry.mu.RUnlock()
    return authManagerRegistry.managers[key]
}

// ClearCurrentAuthManager removes an AuthManager from the registry.
func ClearCurrentAuthManager(key string) {
    authManagerRegistry.mu.Lock()
    defer authManagerRegistry.mu.Unlock()
    delete(authManagerRegistry.managers, key)
}

// ClearAllAuthManagers removes all AuthManagers from the registry.
// Used for cleanup in tests.
func ClearAllAuthManagers() {
    authManagerRegistry.mu.Lock()
    defer authManagerRegistry.mu.Unlock()
    authManagerRegistry.managers = make(map[string]any)
}
```

#### Step 2: Store AuthManager in ExecuteTerraform

**File:** `internal/exec/terraform.go`

```go
func ExecuteTerraform(info schema.ConfigAndStacksInfo) error {
    // ... existing AuthManager creation code ...

    authManager, err := auth.CreateAndAuthenticateManager(
        info.Identity,
        mergedAuthConfig,
        cfg.IdentityFlagSelectValue)
    if err != nil {
        return err
    }

    // ✅ NEW: Store in registry for nested operations
    registryKey := fmt.Sprintf("%s-%s", info.Stack, info.ComponentFromArg)
    SetCurrentAuthManager(registryKey, authManager)
    defer ClearCurrentAuthManager(registryKey)

    // ... rest of function ...
}
```

#### Step 3: Retrieve AuthManager in GetTerraformState

**File:** `internal/exec/terraform_state_utils.go`

```go
func GetTerraformState(
    atmosConfig *schema.AtmosConfiguration,
    yamlFunc string,
    stack string,
    component string,
    output string,
    skipCache bool,
    authContext *schema.AuthContext,
) (any, error) {
    // ... existing code ...

    // ✅ NEW: Try to get AuthManager from global registry
    // Use the original component being processed, not the nested one
    registryKey := fmt.Sprintf("%s-%s", stack, component)
    authManager := GetCurrentAuthManager(registryKey)

    componentSections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
        Component:            component,
        Stack:                stack,
        ProcessTemplates:     true,
        ProcessYamlFunctions: true,
        Skip:                 nil,
        AuthManager:          authManager,  // ✅ Use from registry
    })

    // ... rest of function ...
}
```

**Problem with Option 2:**
The registry key `stack-component` works for the top-level component but fails for nested components. When evaluating `!terraform.state tgw/attachment`, the key becomes `core-use2-network-tgw/attachment`, which doesn't match the stored key `core-use2-network-tgw/routes`.

**Potential Fix:** Use goroutine-local storage or context-based key, but this becomes complex and fragile.

## Recommended Solution

**Option 1: Add AuthManager to ConfigAndStacksInfo**

This is the architecturally correct solution because:

1. **Consistency:** `AuthContext` is already in `ConfigAndStacksInfo`. `AuthManager` (the source of `AuthContext`) should be there too.
2. **Explicitness:** Function signatures clearly show AuthManager is required and available.
3. **Testability:** Easy to pass mock AuthManager in tests.
4. **No Hidden State:** No global registry, no cleanup issues, no race conditions.
5. **Natural Flow:** AuthManager flows through the call stack just like AuthContext does.

## Implementation Plan

### Phase 1: Core Changes
1. Add `AuthManager any` field to `ConfigAndStacksInfo` struct
2. Update `TerraformStateGetter` interface to include `authManager` parameter
3. Update `GetTerraformState` function signature
4. Update `processTagTerraformStateWithContext` to extract and pass `authManager`

### Phase 2: Propagation
5. Update `ExecuteTerraform` to store `authManager` in `configAndStacksInfo`
6. Update all `ExecuteDescribeComponent` calls to use `authManager` from context
7. Update `terraform_output_utils.go` for `!terraform.output` consistency

### Phase 3: Testing
8. Add test for nested `!terraform.state` with AuthContext propagation
9. Update existing mock-based tests to include `authManager` parameter

### Phase 4: Documentation
11. Update `docs/terraform-state-yaml-func-authentication-flow.md`
12. Add example to documentation showing nested function authentication

## Testing Strategy

### Unit Test: Nested !terraform.state with AuthContext

```go
func TestNestedTerraformStateAuthContextPropagation(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockAuthManager := auth.NewMockAuthManager(ctrl)
    mockAuthContext := &schema.AuthContext{
        Profile:         "test-profile",
        CredentialsFile: "/tmp/test-credentials",
        ConfigFile:      "/tmp/test-config",
        Region:          "us-east-2",
    }

    // Setup: Top-level component with AuthManager
    configAndStacksInfo := schema.ConfigAndStacksInfo{
        Stack:          "test-stack",
        Component:      "top-level",
        AuthContext:    mockAuthContext,
        AuthManager:    mockAuthManager,
    }

    // Mock GetState to verify AuthManager is passed through
    mockStateGetter := NewMockTerraformStateGetter(ctrl)
    mockStateGetter.EXPECT().
        GetState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false, mockAuthContext, mockAuthManager).
        Return("expected-value", nil)

    stateGetter = mockStateGetter
    defer func() { stateGetter = &defaultStateGetter{} }()

    // Execute nested evaluation
    result := processTagTerraformStateWithContext(
        &schema.AtmosConfiguration{},
        "!terraform.state nested-component output-name",
        "test-stack",
        nil,
        &configAndStacksInfo,
    )

    assert.Equal(t, "expected-value", result)
}
```

## Success Criteria

1. ✅ Nested `!terraform.state` functions successfully authenticate using parent AuthManager
2. ✅ No IMDS timeout errors when processing nested component configurations
3. ✅ Debug logs show AuthContext is used at all levels of evaluation
4. ✅ All existing tests pass
5. ✅ New tests cover nested authentication scenarios

## Additional Feature: Component-Level Auth Override in Nested Functions

As part of the fix implementation, support for component-level authentication override in nested function evaluations was added. This allows each component in a nested chain to optionally define its own `auth:` configuration, enabling cross-account and cross-permission scenarios.

### Feature Overview

When evaluating nested `!terraform.state` or `!terraform.output` functions, Atmos now checks each component's configuration for an `auth:` section:

1. **Component has `auth:` section** → Merges with global auth and creates component-specific AuthManager
2. **Component has no `auth:` section** → Inherits parent's AuthManager

### Implementation

**File:** `internal/exec/terraform_nested_auth_helper.go` (NEW)

Created `resolveAuthManagerForNestedComponent()` function that:

1. Gets component config WITHOUT processing templates/functions (avoids circular dependency)
2. Checks if component has `auth:` section defined
3. If yes: merges component auth with global auth and creates component-specific AuthManager
4. If no: returns parent AuthManager (inheritance)

**Updated Functions:**
- `GetTerraformState()` in `terraform_state_utils.go` - calls resolver before `ExecuteDescribeComponent()`
- `GetTerraformOutput()` in `terraform_output_utils.go` - same logic for consistency

### Use Case Example

```yaml
# Global auth configuration
auth:
  identities:
    dev-account:
      default: true
      kind: aws/permission-set
      via:
        provider: aws-sso
        account: "111111111111"
        permission_set: "DevAccess"

# Component 1: Uses global auth (dev account)
components:
  terraform:
    app/frontend:
      vars:
        # Reads from app/backend which is in a different account
        backend_url: !terraform.state app/backend api_endpoint

# Component 2: Overrides auth for prod account access
components:
  terraform:
    app/backend:
      auth:
        identities:
          prod-account:
            default: true
            kind: aws/permission-set
            via:
              provider: aws-sso
              account: "222222222222"
              permission_set: "ProdReadOnly"
      vars:
        # This component's state is in prod account
        database_endpoint: !terraform.state database/postgres endpoint
```

**Authentication flow:**
1. `app/frontend` evaluated with dev-account credentials (global default)
2. Encounters `!terraform.state app/backend` → checks for `auth:` section
3. Finds `auth:` section in `app/backend` → creates new AuthManager with prod-account credentials
4. `app/backend` config evaluated with prod-account credentials
5. Nested `!terraform.state database/postgres` inherits prod-account credentials from parent

### Testing

Created comprehensive test suite in `describe_component_auth_override_test.go` with:

- `TestComponentLevelAuthOverride` - Verifies basic auth override functionality
- `TestResolveAuthManagerForNestedComponent` - Tests resolver with/without parent AuthManager
- `TestAuthOverrideInNestedChain` - Documents auth behavior in nested chains
- `TestAuthOverrideErrorHandling` - Verifies graceful error handling

All tests pass successfully (10 tests total for nested auth scenarios).

### Benefits

1. **Cross-Account State Reading** - Components in different AWS accounts can be referenced in nested functions
2. **Granular Permission Control** - Each component can specify required permission level
3. **Inheritance by Default** - Components without `auth:` section automatically inherit parent credentials
4. **No Breaking Changes** - Existing configurations work without modification

### Documentation

Updated authentication flow documentation:
- `docs/terraform-state-yaml-func-authentication-flow.md` - Added "Component-Level Auth Override" section
- `docs/terraform-output-yaml-func-authentication-flow.md` - Added "Component-Level Auth Override in Nested Functions" section

## Related Issues

- PR #1769: Fixed authentication for YAML functions with `--identity` flag
- Issue: Cross-account state access in transit gateway hub-spoke architecture
