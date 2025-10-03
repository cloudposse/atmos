# PRD: Lifecycle Hooks Component Scoping

## Executive Summary

Lifecycle hooks in Atmos are designed to execute automated actions at specific points in a component's lifecycle (e.g., after `terraform apply`). This document defines the design, implementation, and expected behavior of hook scoping to ensure hooks are properly isolated to their defining components and do not leak across component boundaries.

## Problem Statement

When multiple components in a stack define lifecycle hooks, there is potential for:

1. **Hook pollution**: Hooks defined in one component appearing in other components
2. **Output accumulation**: Outputs from multiple components merging together incorrectly
3. **Unexpected behavior**: Components executing hooks not intended for them
4. **Configuration complexity**: Difficulty understanding which hooks apply to which components

## Design Goals

1. **Component isolation**: Hooks defined for a component should only apply to that component
2. **DRY principle support**: Allow shared hook structure while maintaining output isolation
3. **Predictable behavior**: Users should be able to easily understand hook scope
4. **Template flexibility**: Support both simple and advanced hook configuration patterns

## Design Principles

### Scoping Rules

1. **Hooks are component-scoped by default**: A hook defined in a component's configuration applies only to that component
2. **Hook merging follows component inheritance**: Hooks merge through the same inheritance chain as other component properties
3. **Output maps are component-specific**: Even when hook names are shared, output definitions remain isolated per component
4. **Global hooks require explicit structure**: Shared hook configurations must be defined in defaults and overridden per component

## Architecture

### Hook Configuration Hierarchy

Hooks can be defined at multiple levels in the Atmos configuration hierarchy:

```
Global (_defaults.yaml)
  ↓
Terraform Section (terraform.hooks)
  ↓
Component (components.terraform.<component>.hooks)
  ↓
Component Overrides (components.terraform.<component>.overrides.hooks)
```

### Merging Behavior

When hooks are merged through the inheritance chain:

1. **Hook names are keys**: Hooks with the same name merge their properties
2. **Properties are deep-merged**: Arrays and maps merge by default
3. **Outputs are component-specific**: Each component's outputs override, not accumulate

### Implementation

Hook scoping is implemented in `internal/exec/stack_processor_utils.go` at line 1303-1313:

```go
finalComponentHooks, err := m.Merge(
    atmosConfig,
    []map[string]any{
        globalAndTerraformHooks,
        baseComponentHooks,
        componentHooks,
        componentOverridesHooks,
    })
```

This ensures hooks are merged per-component during stack processing, maintaining isolation.

## Configuration Patterns

### Pattern 1: Unique Hook Names (Simple)

Each component defines its complete hook configuration with a unique name.

**Use Case**: Small number of components, each with distinct hook requirements.

```yaml
# stacks/catalog/vpc.yaml
components:
  terraform:
    vpc:
      metadata:
        component: vpc
      hooks:
        vpc-store-outputs:
          events:
            - after-terraform-apply
          command: store
          name: prod/ssm
          outputs:
            vpc_id: .vpc_id
            vpc_cidr_block: .vpc_cidr_block
```

**Pros**:
- Simple and explicit
- No risk of naming conflicts
- Easy to understand

**Cons**:
- Duplicates hook structure across components
- More verbose

### Pattern 2: DRY Pattern (Recommended)

Global defaults define hook structure, components override outputs only.

**Use Case**: Many components with similar hook structure but different outputs.

```yaml
# stacks/catalog/_defaults.yaml
hooks:
  store-outputs:
    events:
      - after-terraform-apply
    command: store
    name: prod/ssm
    # No outputs - defined per component

# stacks/catalog/vpc.yaml
components:
  terraform:
    vpc:
      metadata:
        component: vpc
      hooks:
        store-outputs:
          outputs:
            vpc_id: .vpc_id
            vpc_cidr_block: .vpc_cidr_block

# stacks/catalog/rds.yaml
components:
  terraform:
    rds:
      metadata:
        component: rds
      hooks:
        store-outputs:
          outputs:
            cluster_endpoint: .cluster_endpoint
            cluster_id: .cluster_id
```

**Pros**:
- Reduces duplication
- Easy to update hook behavior globally
- Maintains output isolation per component
- Scalable to many components

**Cons**:
- Requires understanding of inheritance
- Global changes affect all components

**Why This Works**: The merging algorithm combines the global hook structure with component-specific outputs. Each component gets:
- The `events`, `command`, and `name` from the global definition
- Only the `outputs` it explicitly defines

## Test Coverage

### Test Location

- **Implementation**: `pkg/hooks/hooks_component_scope_test.go`
- **Test Cases**: `tests/test-cases/hooks-component-scoped/`

### Test Scenarios

#### Test 1: `TestHooksAreComponentScoped`

Verifies that components with unique hook names remain isolated.

**Setup**:
- 3 components: vpc, rds, lambda
- Each with uniquely named hooks
- All imported into one stack

**Assertions**:
- ✅ VPC has only `vpc-store-outputs`
- ✅ RDS has only `rds-store-outputs`
- ✅ Lambda has only `lambda-store-outputs`
- ✅ No cross-component hook pollution

#### Test 2: `TestHooksWithDRYPattern`

Verifies the DRY pattern maintains output scoping.

**Setup**:
- Global `_defaults.yaml` with hook structure
- 3 components: vpc-dry, rds-dry, lambda-dry
- Each defines only outputs
- All imported into one stack

**Assertions**:
- ✅ All components have `store-outputs` hook
- ✅ VPC has only VPC outputs
- ✅ RDS has only RDS outputs
- ✅ Lambda has only Lambda outputs
- ✅ No output accumulation across components

### Running Tests

```bash
# Run both scoping tests
go test -v ./pkg/hooks -run TestHooks

# Run specific tests
go test -v ./pkg/hooks -run TestHooksAreComponentScoped
go test -v ./pkg/hooks -run TestHooksWithDRYPattern

# Run all hooks tests
go test -v ./pkg/hooks
```

## Expected Behavior

### What Should Happen

1. **Component A** with `store-outputs` hook defining outputs `[a1, a2]`
2. **Component B** with `store-outputs` hook defining outputs `[b1, b2]`
3. When describing Component A: should see hook with outputs `[a1, a2]` only
4. When describing Component B: should see hook with outputs `[b1, b2]` only

### What Should NOT Happen

1. ❌ Component A should not see outputs `[b1, b2]`
2. ❌ Component B should not see outputs `[a1, a2]`
3. ❌ Either component should not see accumulated outputs `[a1, a2, b1, b2]`

## Debugging Hook Scope Issues

If users report hooks appearing globally:

### Step 1: Verify Hook Definition Location

Check where hooks are defined:

```bash
# View component configuration
atmos describe component <component> -s <stack>

# Check the hooks section
atmos describe component <component> -s <stack> | yq '.hooks'
```

### Step 2: Check Import Chain

Verify import hierarchy:

```bash
# View all imports
atmos describe component <component> -s <stack> | yq '.imports'

# View dependency chain
atmos describe component <component> -s <stack> | yq '.deps'
```

### Step 3: Compare Against Test Cases

Compare user configuration against working test cases:
- `tests/test-cases/hooks-component-scoped/stacks/catalog/_defaults.yaml`
- `tests/test-cases/hooks-component-scoped/stacks/catalog/*-dry.yaml`

### Step 4: Verify Hook Name Uniqueness

If using DRY pattern:
- ✅ Same hook name across components is OK
- ✅ Each component defines only its outputs
- ❌ Do not define all outputs in _defaults.yaml

## Common Pitfalls

### Pitfall 1: Defining All Outputs Globally

**Incorrect**:
```yaml
# _defaults.yaml - WRONG
hooks:
  store-outputs:
    outputs:
      vpc_id: .vpc_id          # ❌ Will apply to ALL components
      cluster_id: .cluster_id  # ❌ Will apply to ALL components
```

**Correct**:
```yaml
# _defaults.yaml - CORRECT
hooks:
  store-outputs:
    events: [after-terraform-apply]
    command: store
    name: prod/ssm
    # ✅ No outputs here

# vpc.yaml - CORRECT
hooks:
  store-outputs:
    outputs:
      vpc_id: .vpc_id  # ✅ Only for VPC
```

### Pitfall 2: Using Global Scope Incorrectly

**Incorrect**:
```yaml
# At top-level of stack file - WRONG
hooks:
  store-outputs:
    outputs:
      vpc_id: .vpc_id  # ❌ May apply to all components in file
```

**Correct**:
```yaml
# Scoped to component - CORRECT
components:
  terraform:
    vpc:
      hooks:
        store-outputs:
          outputs:
            vpc_id: .vpc_id  # ✅ Only for VPC
```

## Implementation Details

### Stack Processing

When processing a stack, Atmos:

1. Loads all imported files in order
2. Builds component configuration through merging
3. Merges hooks using the same algorithm as other properties
4. Ensures each component gets only its own hook definitions

### Merge Algorithm

The merge algorithm (`pkg/merge/`) handles hook merging:

1. **Maps merge by key**: Hooks with same name merge
2. **Arrays concatenate**: Events can accumulate
3. **Last write wins for scalars**: Later values override earlier ones
4. **Deep merge for nested structures**: Outputs merge at the component level

## Best Practices

### DO

✅ **Use the DRY pattern** for multiple components with similar hooks
✅ **Define hook structure globally** (events, command, name)
✅ **Define outputs per component** in component-specific files
✅ **Use unique hook names** if components need different hook structures
✅ **Test with `describe component`** to verify hook configuration

### DON'T

❌ **Don't define all outputs in _defaults.yaml**
❌ **Don't expect hooks to automatically scope without proper structure**
❌ **Don't define hooks at top-level of stack files** (use component scope)
❌ **Don't skip testing** - always verify with `describe component`

## Reference Implementation

Complete working examples available in:
- `tests/test-cases/hooks-component-scoped/`

## Related Documentation

- [Lifecycle Hooks Overview](https://atmos.tools/core-concepts/stacks/hooks)
- [Stack Processing](/docs/prd/stack-processing.md)
- [Configuration Merging](/docs/prd/configuration-merging.md)

## Version History

- **v1.0** (2025-01): Initial PRD with test coverage for component scoping
- Added DRY pattern as recommended approach
- Comprehensive test coverage for both patterns

## Future Considerations

1. **Conditional hooks**: Execute hooks based on component state
2. **Hook dependencies**: Order hook execution across components
3. **Hook templates**: Parameterized hook definitions
4. **Validation**: Schema validation for hook configurations
