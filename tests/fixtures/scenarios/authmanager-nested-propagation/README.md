# Nested AuthManager Propagation and Component-Level Auth Override Test Fixture

This test fixture verifies that AuthManager and AuthContext propagate correctly through multiple levels of nested `!terraform.state` and `!terraform.output` YAML functions, including component-level authentication overrides.

## Purpose

Tests the complete authentication propagation system for nested YAML functions where:
- AuthManager propagates from parent to nested component evaluations
- Components can optionally override authentication at any nesting level
- Components without auth config inherit parent's AuthManager
- Multiple auth overrides can coexist in the same evaluation chain
- Deep nesting (4+ levels) works correctly with mixed auth configurations

## What It Tests

This fixture verifies:

1. **Basic Nested Propagation** - AuthManager flows through multiple levels without overrides
2. **Component-Level Auth Override** - Components can define their own `auth:` section
3. **Auth Inheritance** - Components without `auth:` section inherit parent's AuthManager
4. **Mixed Auth Scenarios** - Some components override, others inherit in same chain
5. **Deep Nesting** - Authentication works correctly at 4+ nesting levels
6. **Error-Free Execution** - No IMDS timeout errors at any nesting level

## Test Scenarios

### Scenario 1: Basic Nested Propagation (No Auth Overrides)

Tests that AuthManager propagates through nested levels when no components override auth.

```
Level 1: level1-component (no auth override)
  └─ !terraform.state level2-component ... (inherits parent auth)
      ↓
Level 2: level2-component (no auth override)
  └─ !terraform.state level3-component ... (inherits parent auth)
      ↓
Level 3: level3-component (base, no nested functions)
```

**Expected Behavior:**
- All levels use the same AuthManager from top-level command
- Single authentication session for entire evaluation chain
- No re-authentication at any level

### Scenario 2: Auth Override at Middle Level

Tests that a component in the middle of a chain can override authentication.

```
Level 1: auth-override-level1 (no auth override)
  └─ !terraform.state auth-override-level2 ... (uses parent auth for this call)
      ↓
Level 2: auth-override-level2 (HAS auth override → account 222222222222)
  └─ !terraform.state auth-override-level3 ... (uses level2's auth)
      ↓
Level 3: auth-override-level3 (no auth override, inherits from level2)
```

**Expected Behavior:**
- Level 1 uses parent/global AuthManager
- Level 2 detects `auth:` section and creates new AuthManager with account 222222222222
- Level 3 inherits Level 2's AuthManager (account 222222222222)

**Auth Configuration:**
```yaml
auth-override-level2:
  auth:
    identities:
      test-level2-identity:
        default: true
        kind: aws/permission-set
        via:
          provider: aws-sso
          account: "222222222222"
          permission_set: "Level2Access"
```

### Scenario 3: Multiple Auth Overrides in Chain

Tests that multiple components in a chain can each have their own auth overrides.

```
Level 1: multi-auth-level1 (auth override → account 444444444444)
  └─ !terraform.state multi-auth-level2 ... (uses level1's auth for this call)
      ↓
Level 2: multi-auth-level2 (auth override → account 333333333333)
  └─ !terraform.state multi-auth-level3 ... (uses level2's auth)
      ↓
Level 3: multi-auth-level3 (no auth override, inherits from level2)
```

**Expected Behavior:**
- Level 1 uses account 444444444444 (AccountCAccess)
- Level 2 uses account 333333333333 (AccountBAccess) - its own override
- Level 3 uses account 333333333333 (inherited from Level 2)

**Use Case:** Cross-account state reading where:
- Account C component reads state from Account B component
- Account B component reads state from Account A component
- Each account requires different permissions/credentials

### Scenario 4: Selective Auth Override (Mixed Inheritance)

Tests that in the same top-level component, some nested components can override auth while others inherit.

```
mixed-top-level (no auth override, uses parent auth)
  ├─ !terraform.state mixed-inherit-component ... (inherits parent auth)
  └─ !terraform.state mixed-override-component ... (uses its own auth)
      └─ !terraform.state mixed-inherit-component ... (inherits mixed-override's auth)
```

**Expected Behavior:**
- `mixed-top-level` uses parent AuthManager
- `mixed-inherit-component` inherits parent AuthManager when called directly
- `mixed-override-component` uses account 555555555555 (its own auth)
- When `mixed-override-component` calls `mixed-inherit-component`, it inherits account 555555555555

**Auth Configuration:**
```yaml
mixed-override-component:
  auth:
    identities:
      mixed-override-identity:
        default: true
        kind: aws/permission-set
        via:
          provider: aws-sso
          account: "555555555555"
          permission_set: "MixedAccess"
```

### Scenario 5: Deep Nesting with Auth Override at Multiple Levels

Tests authentication in 4-level deep nesting with auth overrides at non-adjacent levels.

```
Level 1: deep-level1 (auth override → account 777777777777)
  └─ !terraform.state deep-level2 ... (uses level1's auth)
      ↓
Level 2: deep-level2 (no auth override, inherits from level1)
  └─ !terraform.state deep-level3 ... (uses level1's auth)
      ↓
Level 3: deep-level3 (auth override → account 666666666666)
  └─ !terraform.state deep-level4 ... (uses level3's auth)
      ↓
Level 4: deep-level4 (no auth override, inherits from level3)
```

**Expected Behavior:**
- Level 1 uses account 777777777777 (Level1Access)
- Level 2 inherits account 777777777777 from Level 1
- Level 3 switches to account 666666666666 (Level3Access) - its own override
- Level 4 inherits account 666666666666 from Level 3

**Auth Flow:**
```
Account 777777777777 → Level 1
     ↓ (inherited)
Account 777777777777 → Level 2
     ↓ (override)
Account 666666666666 → Level 3
     ↓ (inherited)
Account 666666666666 → Level 4
```

## Structure

```
authmanager-nested-propagation/
├── atmos.yaml              # Minimal Atmos configuration
├── stacks/
│   └── test.yaml          # Test stack with 18 components across 5 scenarios
└── README.md              # This file
```

## Backend Configuration

All components use **local backend** for testing:
- No AWS credentials required for actual state operations
- No external dependencies
- Faster test execution
- Tests run in isolation
- Focus is on authentication resolution, not actual state reads

## Component Naming Convention

Components are named to indicate their role in the test:

- `level1-component`, `level2-component`, `level3-component` - Basic propagation (Scenario 1)
- `auth-override-level1/2/3` - Auth override at middle level (Scenario 2)
- `multi-auth-level1/2/3` - Multiple auth overrides (Scenario 3)
- `mixed-*` - Mixed inheritance patterns (Scenario 4)
- `deep-level1/2/3/4` - Deep nesting (Scenario 5)

## How the Fix Works

### Before the Fix

**Problem:**
- `GetTerraformState()` called `ExecuteDescribeComponent(AuthManager: nil)`
- Nested components had no AuthContext
- AWS SDK fell back to IMDS, causing timeout errors on non-EC2 instances

**Error Message:**
```
dial tcp 169.254.169.254:80: i/o timeout
```

### After the Fix

**Solution:**
1. Added `AuthManager any` field to `ConfigAndStacksInfo` struct
2. `GetTerraformState()` receives `authManager` parameter and passes it to `ExecuteDescribeComponent()`
3. Created `resolveAuthManagerForNestedComponent()` to handle auth resolution:
   - Gets component config WITHOUT processing templates/functions (avoids circular dependency)
   - Checks if component has `auth:` section
   - If yes: merges with global auth and creates component-specific AuthManager
   - If no: returns parent AuthManager (inheritance)
4. Both `!terraform.state` and `!terraform.output` use same resolution logic

**Implementation Files:**
- `internal/exec/terraform_nested_auth_helper.go` - Auth resolution helper
- `internal/exec/terraform_state_utils.go` - Updated GetTerraformState()
- `internal/exec/terraform_output_utils.go` - Updated GetTerraformOutput()
- `pkg/schema/schema.go` - Added AuthManager field to ConfigAndStacksInfo

## Component-Level Auth Resolution Algorithm

When evaluating a nested component, Atmos:

1. **Extract parent AuthManager** from the calling context
2. **Get component config** without processing templates/functions
3. **Check for `auth:` section** in component configuration
4. **If `auth:` exists:**
   - Merge component auth with global auth config
   - Create new AuthManager with merged config
   - Authenticate and populate AuthContext
   - Use component-specific AuthManager for this evaluation
5. **If no `auth:` section:**
   - Inherit parent's AuthManager
   - Use parent's AuthContext
   - No re-authentication needed

## Usage in Tests

### Testing Basic Propagation

```go
func TestNestedAuthManagerPropagation(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockAuthManager := types.NewMockAuthManager(ctrl)
    mockAuthManager.EXPECT().
        GetStackInfo().
        Return(authStackInfo).
        AnyTimes()

    workDir := "../../tests/fixtures/scenarios/authmanager-nested-propagation"
    t.Chdir(workDir)

    componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
        Component:            "level1-component",
        Stack:                "test",
        ProcessTemplates:     true,
        ProcessYamlFunctions: true,
        AuthManager:          mockAuthManager,
    })

    require.NoError(t, err)
    // Verify nested functions resolved without IMDS errors
}
```

### Testing Auth Override

```go
func TestAuthOverrideAtMiddleLevel(t *testing.T) {
    // Test that auth-override-level2 creates its own AuthManager
    // while auth-override-level3 inherits it

    componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
        Component:            "auth-override-level1",
        Stack:                "test",
        ProcessTemplates:     true,
        ProcessYamlFunctions: true,
        AuthManager:          mockAuthManager,
    })

    require.NoError(t, err)

    // Verify that:
    // 1. level1 used parent auth
    // 2. level2 created new auth (account 222222222222)
    // 3. level3 inherited level2's auth
}
```

### Testing Mixed Scenarios

```go
func TestMixedAuthInheritanceAndOverride(t *testing.T) {
    // Test that in same evaluation:
    // - some components override auth
    // - others inherit auth

    componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
        Component:            "mixed-top-level",
        Stack:                "test",
        ProcessTemplates:     true,
        ProcessYamlFunctions: true,
        AuthManager:          mockAuthManager,
    })

    require.NoError(t, err)

    // Verify:
    // - mixed-inherit-component used parent auth
    // - mixed-override-component used account 555555555555
}
```

## Key Behaviors Verified

### 1. AuthManager Propagation
- ✅ AuthManager flows from top-level command through all nested evaluations
- ✅ No IMDS timeout errors at any nesting level
- ✅ Single authentication session when no overrides exist

### 2. Component-Level Auth Override
- ✅ Components can define `auth:` section to override authentication
- ✅ Auth override creates new AuthManager with merged config
- ✅ New AuthManager is used for that component and its nested calls

### 3. Auth Inheritance
- ✅ Components without `auth:` section inherit parent's AuthManager
- ✅ No re-authentication when inheriting
- ✅ Inherited AuthManager works correctly for nested calls

### 4. Mixed Scenarios
- ✅ Multiple components in same chain can each have different auth
- ✅ Auth override affects only that component and its descendants
- ✅ Sibling components can have different auth configurations

### 5. Deep Nesting
- ✅ Authentication works correctly at 4+ nesting levels
- ✅ Auth overrides can occur at any level in the chain
- ✅ Non-adjacent auth overrides work correctly (e.g., Level 1 and 3 override, Level 2 and 4 inherit)

## Performance Characteristics

- **No Redundant Authentication**: Components inherit auth instead of re-authenticating
- **Component-Specific Auth**: Creates new AuthManager only when component has `auth:` section
- **Cache-Friendly**: Authentication results are reused within evaluation chain
- **Efficient Resolution**: Auth resolution happens without processing templates/functions

## Debug Output

Enable debug logging to see auth resolution in action:

```bash
ATMOS_LOGS_LEVEL=Debug atmos describe component level1-component -s test
```

Look for log messages:
```
Component has no auth config, inheriting parent AuthManager
Component has auth config, creating component-specific AuthManager
Created component-specific AuthManager identityChain=[account-b-identity]
```

## Related Files

- **Test**: `internal/exec/describe_component_nested_authmanager_test.go`
- **Test**: `internal/exec/describe_component_auth_override_test.go`
- **Implementation**:
  - `internal/exec/terraform_nested_auth_helper.go` - Auth resolution logic
  - `internal/exec/terraform_state_utils.go` - GetTerraformState() with auth propagation
  - `internal/exec/terraform_output_utils.go` - GetTerraformOutput() with auth propagation
  - `pkg/schema/schema.go` - ConfigAndStacksInfo.AuthManager field
- **Documentation**:
  - `docs/terraform-state-yaml-func-authentication-flow.md`
  - `docs/terraform-output-yaml-func-authentication-flow.md`
  - `docs/fixes/nested-terraform-state-auth-context-propagation.md`

## Test Execution

Run tests that use this fixture:

```bash
# Run all nested AuthManager tests
go test ./internal/exec -run "TestNested.*AuthManager" -v

# Run component auth override tests
go test ./internal/exec -run "TestComponent.*AuthOverride" -v

# Run specific scenario tests
go test ./internal/exec -run "TestAuthOverrideInNestedChain" -v
```

## Success Criteria

- ✅ All nested `!terraform.state` functions resolve without IMDS errors
- ✅ Component-level auth overrides create new AuthManagers
- ✅ Components without auth config inherit parent AuthManager
- ✅ Mixed auth scenarios work correctly in same evaluation chain
- ✅ Deep nesting (4+ levels) works with auth overrides at multiple levels
- ✅ Debug logging shows correct auth resolution at each level
- ✅ All test cases pass

## Real-World Use Cases

This fixture models common real-world scenarios:

### Cross-Account State Reading
```yaml
# Account C component reading from Account B component
# which reads from Account A component
multi-auth-level1:  # Account C (444444444444)
  vars:
    vpc_ref: !terraform.state multi-auth-level2 vpc_id

multi-auth-level2:  # Account B (333333333333)
  auth:
    identities:
      account-b-identity:
        account: "333333333333"
  vars:
    shared: !terraform.state multi-auth-level3 shared_resource_id

multi-auth-level3:  # Account A (default/inherited)
  vars:
    shared_resource_id: "shared-12345"
```

### Hub-Spoke Architecture
```yaml
# Transit Gateway hub component (elevated permissions)
tgw-hub:
  auth:
    identities:
      network-admin:
        permission_set: "NetworkAdministrator"

# Spoke components (standard permissions) reading hub state
tgw-spoke:
  vars:
    tgw_id: !terraform.state tgw-hub transit_gateway_id
```

### Environment-Specific Permissions
```yaml
# Production components require elevated permissions
prod-database:
  auth:
    identities:
      prod-dba:
        permission_set: "DatabaseAdmin"
  vars:
    backup_arn: !terraform.state prod-s3-backups bucket_arn

# Development components use standard permissions
dev-database:
  vars:  # No auth override, uses default dev credentials
    config: "standard-dev-config"
```
