# AuthManager Propagation Test Fixture

This test fixture is specifically designed to test the authentication context propagation from AuthManager to ExecuteDescribeComponent.

## Purpose

Tests the fix for the bug where:
- Commands created `AuthManager` with credentials (via `--identity` flag)
- `ExecuteDescribeComponent` accepted `AuthManager` parameter but didn't extract `AuthContext`
- `AuthContext` was nil in `configAndStacksInfo`
- YAML template functions (`!terraform.state`, `!terraform.output`) failed with timeout errors

## What It Tests

This fixture is used by `describe_component_authmanager_propagation_test.go` to verify:

1. **AuthContext Extraction**: When `ExecuteDescribeComponent` receives an `AuthManager`, it correctly extracts the `AuthContext` from `AuthManager.GetStackInfo()` and populates `configAndStacksInfo.AuthContext`.

2. **Nil Handling**: Graceful handling of:
   - Nil `AuthManager` (commands without `--identity` flag)
   - Nil `stackInfo` from `AuthManager.GetStackInfo()`
   - Nil `AuthContext` in `stackInfo`

3. **Integration**: End-to-end component description with authentication context.

## Structure

```
authmanager-propagation/
├── atmos.yaml              # Minimal Atmos configuration
├── stacks/
│   └── test.yaml          # Test stack with simple component
└── components/terraform/
    └── test-component/
        └── main.tf        # Minimal Terraform component
```

## Usage

This fixture is intentionally minimal - it contains only what's necessary to test authentication context propagation, without additional complexity from:
- Abstract components
- Component inheritance
- YAML template functions
- Complex variable structures

The tests focus on verifying the specific code path in `describe_component.go` lines 464-473:

```go
if params.AuthManager != nil {
    managerStackInfo := params.AuthManager.GetStackInfo()
    if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
        configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
        log.Debug("Populated AuthContext from AuthManager for template functions")
    }
}
```

## Related Files

- Test: `internal/exec/describe_component_authmanager_propagation_test.go`
- Implementation: `internal/exec/describe_component.go` (lines 464-473)
- PRD: `docs/prd/terraform-template-functions-auth-context.md`
