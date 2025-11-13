# Nested AuthManager Propagation Test Fixture

This test fixture verifies that AuthManager and AuthContext propagate correctly through multiple levels of nested `!terraform.state` and `!terraform.output` YAML functions.

## Purpose

Tests the fix for nested YAML function authentication where:
- Level 1 component has `!terraform.state level2-component ...`
- Level 2 component has `!terraform.state level3-component ...`
- Level 3 component is the base (no nested functions)
- AuthManager must propagate from Level 1 → Level 2 → Level 3

Without the fix, Level 2 and Level 3 would fail with IMDS timeout errors because AuthManager was not passed to `ExecuteDescribeComponent` when evaluating nested functions.

## What It Tests

This fixture verifies:

1. **Multi-Level Propagation**: AuthManager flows through 3 levels of nesting
2. **AuthContext Availability**: Each nested level has access to AuthContext for state reads
3. **No Re-Authentication**: Single authentication session used across all levels
4. **Error-Free Execution**: No IMDS timeout errors at any nesting level

## Nesting Structure

```
Level 1: level1-component
  ├─ vars.level2_vpc_id: !terraform.state level2-component test vpc_id
  └─ Triggers evaluation of level2-component
      ↓
Level 2: level2-component
  ├─ vars.level3_subnet_id: !terraform.state level3-component test subnet_id
  └─ Triggers evaluation of level3-component
      ↓
Level 3: level3-component
  └─ vars: base component with no nested functions
```

## Structure

```
authmanager-nested-propagation/
├── atmos.yaml              # Minimal Atmos configuration
├── stacks/
│   └── test.yaml          # Test stack with 3 nested components (local backend)
└── README.md              # This file
```

## Backend Configuration

All components use **local backend** for testing:
- No AWS credentials required
- No external dependencies
- Faster test execution
- Tests run in isolation

## Before the Fix

**Problem:**
- `GetTerraformState()` called `ExecuteDescribeComponent(AuthManager: nil)`
- Nested components had no AuthContext
- AWS SDK fell back to IMDS, causing timeout errors on non-EC2 instances

**Error Message:**
```
dial tcp 169.254.169.254:80: i/o timeout
```

## After the Fix

**Solution:**
- `GetTerraformState()` receives `authManager` parameter
- Casts to `auth.AuthManager` and passes to `ExecuteDescribeComponent()`
- Nested components receive AuthContext from AuthManager
- All levels use same authenticated session

**Implementation:**
1. Added `AuthManager any` field to `ConfigAndStacksInfo` struct (schema.go:607)
2. Updated `GetTerraformState()` signature to accept authManager (terraform_state_utils.go:41)
3. Modified YAML function processors to extract and pass authManager (yaml_func_terraform_state.go:89-97)
4. Updated `ExecuteTerraform()` to store AuthManager (terraform.go:118-121)

## Usage

This fixture is used by tests to verify:

```go
// Test nested propagation with mock AuthManager
authManager := createMockAuthManager(t, authContext)
result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
    Component:   "level1-component",
    Stack:       "test",
    AuthManager: authManager,
})
// Verify nested !terraform.state functions work without IMDS errors
```

## Related Files

- Test: `internal/exec/describe_component_nested_authmanager_test.go` (to be created)
- Implementation:
  - `internal/exec/terraform_state_utils.go` (lines 66-82)
  - `internal/exec/yaml_func_terraform_state.go` (lines 89-97)
  - `pkg/schema/schema.go` (lines 598-607)
- Documentation:
  - `docs/terraform-state-yaml-func-authentication-flow.md`
  - `docs/fixes/nested-terraform-state-auth-context-propagation.md`
