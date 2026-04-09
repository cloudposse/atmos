# Nested AuthManager Propagation and Component-Level Auth Override Test Fixture

This test fixture verifies that AuthManager and AuthContext propagate correctly through multiple levels of nested
`!terraform.state` and `!terraform.output` YAML functions, including component-level authentication overrides.

## Purpose

This fixture tests authentication propagation in nested YAML function evaluations where:

- **AuthManager propagates** from parent to nested component evaluations
- **Component-level auth overrides** allow components to define their own authentication
- **Auth inheritance** enables components without auth config to inherit from parent
- **Mixed scenarios** support some components overriding auth while others inherit
- **Deep nesting** validates authentication works correctly at 4+ nesting levels

## Test Coverage

The fixture includes test cases for:

1. **Basic Nested Propagation** - AuthManager flows through multiple levels without overrides
2. **Component-Level Auth Override** - Components can define their own `auth:` section with default identity
3. **Auth Inheritance** - Components without `auth:` section inherit parent's AuthManager
4. **Mixed Auth Scenarios** - Some components override, others inherit in same chain
5. **Deep Nesting** - Authentication works at 4+ nesting levels with overrides at multiple levels

## Structure

```
authmanager-nested-propagation/
├── atmos.yaml              # Minimal Atmos configuration
├── stacks/
│   └── test.yaml          # Test stack with 18 components across 5 scenarios
└── README.md              # This file
```

All components use **local backend** for testing (no AWS credentials required).

## Related Files

- **Tests**:
  - `internal/exec/describe_component_nested_authmanager_test.go`
  - `internal/exec/describe_component_auth_override_test.go`
- **Implementation**:
  - `internal/exec/terraform_nested_auth_helper.go`
  - `internal/exec/terraform_state_utils.go`
  - `internal/exec/terraform_output_utils.go`
  - `pkg/schema/schema.go`
- **Documentation**:
  - `docs/terraform-yaml-functions-authentication-flow.md`
  - `docs/fixes/nested-terraform-state-auth-context-propagation.md`
