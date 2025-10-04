# Hooks Component Scoping Test Case

This test case verifies that lifecycle hooks are properly scoped to their respective components and not merged globally across all components in a stack.

## Test Scenarios

This test case includes two scenarios:

### Scenario 1: Unique Hook Names
- **3 components**: `vpc`, `rds`, and `lambda`
- Each component is defined in its own catalog file under `stacks/catalog/`
- Each component defines a unique hook name (`vpc-store-outputs`, `rds-store-outputs`, `lambda-store-outputs`)
- All components are imported into a single stack (`acme-dev-test`)

### Scenario 2: DRY Pattern with Shared Hook Name
- **3 components**: `vpc-dry`, `rds-dry`, and `lambda-dry`
- Global `_defaults.yaml` defines the hook structure (events, command, name)
- Each component only defines the `outputs` specific to that component
- All components use the same hook name (`store-outputs`) but with different outputs
- All components are imported into a single stack (`acme-dev-dry`)

## Expected Behavior

### Scenario 1: Unique Hook Names
When describing each component, hooks should be scoped as follows:

- **VPC component**: Should only have `vpc-store-outputs` hook
  - Outputs: `vpc_id`, `vpc_cidr_block`

- **RDS component**: Should only have `rds-store-outputs` hook
  - Outputs: `cluster_endpoint`, `cluster_id`, `database_name`

- **Lambda component**: Should only have `lambda-store-outputs` hook
  - Outputs: `lambda_function_arn`, `lambda_function_name`

### Scenario 2: DRY Pattern (Recommended)
When using the DRY pattern, all components share the same hook name but with component-specific outputs:

- **VPC component**: Has `store-outputs` hook with VPC-specific outputs only
  - Outputs: `vpc_id`, `vpc_cidr_block`
  - Should NOT have RDS or Lambda outputs

- **RDS component**: Has `store-outputs` hook with RDS-specific outputs only
  - Outputs: `cluster_endpoint`, `cluster_id`, `database_name`
  - Should NOT have VPC or Lambda outputs

- **Lambda component**: Has `store-outputs` hook with Lambda-specific outputs only
  - Outputs: `lambda_function_arn`, `lambda_function_name`
  - Should NOT have VPC or RDS outputs

**This DRY pattern is the recommended approach** as it reduces duplication while maintaining proper component scoping.

## Purpose

This test demonstrates that Atmos correctly scopes hooks to their defining components, preventing hook pollution where one component would incorrectly inherit hooks from other components in the same stack.

## Running the Tests

From the repository root:

```bash
# Run both tests
go test -v ./pkg/hooks

# Run specific tests
go test -v ./pkg/hooks -run TestHooksAreComponentScoped
go test -v ./pkg/hooks -run TestHooksWithDRYPattern
```

## Test Structure

```
hooks-component-scoped/
├── atmos.yaml                    # Atmos configuration
├── stacks/
│   ├── catalog/                  # Component catalog definitions
│   │   ├── _defaults.yaml       # Global hook structure for DRY pattern
│   │   ├── vpc.yaml             # VPC component with unique hook name
│   │   ├── rds.yaml             # RDS component with unique hook name
│   │   ├── lambda.yaml          # Lambda component with unique hook name
│   │   ├── vpc-dry.yaml         # VPC using DRY pattern (outputs only)
│   │   ├── rds-dry.yaml         # RDS using DRY pattern (outputs only)
│   │   └── lambda-dry.yaml      # Lambda using DRY pattern (outputs only)
│   └── orgs/acme/
│       ├── dev.yaml             # Stack for unique hook names test
│       └── dev-dry.yaml         # Stack for DRY pattern test
└── components/terraform/         # (empty - no actual terraform components needed)
```

## Related

- Test implementation: `/pkg/hooks/hooks_component_scope_test.go`
- Hooks implementation: `/pkg/hooks/`
- Documentation: https://atmos.tools/core-concepts/stacks/hooks
