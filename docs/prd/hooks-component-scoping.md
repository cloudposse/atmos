# PRD: Lifecycle Hooks Component Scoping

## Purpose

Define how lifecycle hooks are scoped and inherited in Atmos to enable automated actions at specific points in a component's lifecycle while maintaining clear boundaries between components.

## Background

Lifecycle hooks allow users to execute automated actions (e.g., storing outputs to parameter stores, sending notifications) at specific lifecycle events such as `after-terraform-apply` or `before-terraform-plan`. These hooks need to support:

- Reusability across multiple components through inheritance
- Component-specific customization without duplication
- Clear, predictable scoping rules

## Design

### Scoping Model

Hooks follow the same inheritance and scoping rules as all other Atmos configuration:

1. **Hooks are defined within the configuration hierarchy** and inherit through the same chain as variables, settings, and other component properties
2. **Each component receives only the hooks applicable to it** after the configuration is fully merged
3. **Hooks defined at a component level are scoped to that component** and do not leak to other components

### Configuration Hierarchy

Hooks can be defined at any level of the Atmos configuration hierarchy:

```
Global defaults (_defaults.yaml)
  ↓
Terraform section defaults
  ↓
Base component configuration
  ↓
Component-specific configuration
  ↓
Component overrides
```

Each level inherits and can override hooks from levels above it.

### Merging Behavior

When configurations are merged:

1. **Hooks merge by name** - A hook with the same name at different levels merges its properties
2. **Maps deep-merge** - Nested maps (like `outputs`) merge by key
3. **Arrays concatenate** - Arrays (like `events`) combine from all levels
4. **Later values override earlier ones** - Component-specific values take precedence over defaults

This allows for the DRY principle: define structure once, customize per component.

## Configuration Patterns

### Pattern 1: Component-Specific Hooks

Define complete hook configuration at the component level.

**When to use**: Components with unique hook requirements that aren't shared.

```yaml
# stacks/catalog/vpc.yaml
components:
  terraform:
    vpc:
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

**Benefits**:
- Self-contained and explicit
- No dependencies on other configuration
- Easy to understand in isolation

### Pattern 2: Shared Structure with Component Customization (Recommended)

Define hook structure globally, customize outputs per component.

**When to use**: Multiple components share the same hook behavior but need different outputs.

```yaml
# stacks/catalog/_defaults.yaml
hooks:
  store-outputs:
    events:
      - after-terraform-apply
    command: store
    name: prod/ssm

# stacks/catalog/vpc.yaml
components:
  terraform:
    vpc:
      hooks:
        store-outputs:
          outputs:
            vpc_id: .vpc_id
            vpc_cidr_block: .vpc_cidr_block

# stacks/catalog/rds.yaml
components:
  terraform:
    rds:
      hooks:
        store-outputs:
          outputs:
            cluster_endpoint: .cluster_endpoint
            cluster_id: .cluster_id
```

**How it works**:
- VPC component inherits `events`, `command`, and `name` from defaults
- VPC adds its specific `outputs`
- RDS component inherits the same base structure
- RDS adds its own different `outputs`
- Each component gets the same hook behavior with its own outputs

**Benefits**:
- Reduces duplication of hook configuration
- Easy to update hook behavior globally
- Maintains clear component boundaries for outputs
- Scales well to many components

### Pattern 3: Conditional Hooks

Use Atmos templates to conditionally define hooks.

```yaml
# stacks/catalog/defaults.yaml
{{ if eq .stage "prod" }}
hooks:
  store-outputs:
    events:
      - after-terraform-apply
    command: store
    name: {{ .stage }}/ssm
{{ end }}

# Component inherits conditional hook only in prod
components:
  terraform:
    app:
      hooks:
        store-outputs:
          outputs:
            app_url: .url
```

## Implementation

### Stack Processing

During stack processing (`internal/exec/stack_processor_utils.go`):

1. Load all imported stack files in dependency order
2. Build component configuration through hierarchical merging
3. Merge hooks at each level using the same algorithm as other properties
4. Result: Each component has fully resolved hooks applicable only to it

### Hook Merging Code

```go
finalComponentHooks, err := m.Merge(
    atmosConfig,
    []map[string]any{
        globalAndTerraformHooks,    // From _defaults.yaml and terraform section
        baseComponentHooks,          // From base component
        componentHooks,              // From component definition
        componentOverridesHooks,     // From overrides
    })
```

This ensures hooks follow the same inheritance chain as all other component configuration.

## Expected Behavior

### Example: Two Components with Shared Hook

Given this configuration:

```yaml
# _defaults.yaml
hooks:
  store-outputs:
    events: [after-terraform-apply]
    command: store
    name: prod/ssm

# vpc.yaml
components:
  terraform:
    vpc:
      hooks:
        store-outputs:
          outputs:
            vpc_id: .vpc_id

# rds.yaml
components:
  terraform:
    rds:
      hooks:
        store-outputs:
          outputs:
            db_endpoint: .endpoint
```

**Expected result when describing VPC**:
```yaml
hooks:
  store-outputs:
    events: [after-terraform-apply]
    command: store
    name: prod/ssm
    outputs:
      vpc_id: .vpc_id
```

**Expected result when describing RDS**:
```yaml
hooks:
  store-outputs:
    events: [after-terraform-apply]
    command: store
    name: prod/ssm
    outputs:
      db_endpoint: .endpoint
```

Each component inherits the structure but has only its own outputs.

## Test Coverage

### Test Location

- **Implementation**: `pkg/hooks/hooks_component_scope_test.go`
- **Test Cases**: `tests/test-cases/hooks-component-scoped/`

### Test 1: Component-Specific Hooks

**Verifies**: Components with unique hook names remain isolated.

**Setup**:
- 3 components: vpc, rds, lambda
- Each defines complete hook with unique name
- All imported into one stack

**Validates**:
- ✅ VPC has only `vpc-store-outputs` hook
- ✅ RDS has only `rds-store-outputs` hook
- ✅ Lambda has only `lambda-store-outputs` hook

### Test 2: Shared Structure Pattern

**Verifies**: DRY pattern maintains proper output scoping.

**Setup**:
- Global `_defaults.yaml` defines hook structure
- 3 components: vpc-dry, rds-dry, lambda-dry
- Each defines only component-specific outputs
- All imported into one stack

**Validates**:
- ✅ All components have `store-outputs` hook with correct structure
- ✅ VPC has only VPC outputs (vpc_id, vpc_cidr_block)
- ✅ RDS has only RDS outputs (cluster_endpoint, cluster_id, database_name)
- ✅ Lambda has only Lambda outputs (lambda_function_arn, lambda_function_name)

### Running Tests

```bash
# Run both scoping tests
go test -v ./pkg/hooks -run TestHooks

# Run specific test
go test -v ./pkg/hooks -run TestHooksAreComponentScoped
go test -v ./pkg/hooks -run TestHooksWithDRYPattern
```

## Usage Guidelines

### Recommended: Use Shared Structure Pattern

For most use cases, define hook structure globally and customize per component:

```yaml
# Define structure once in _defaults.yaml
hooks:
  store-outputs:
    events: [after-terraform-apply]
    command: store
    name: {{ .stage }}/ssm

# Customize per component
components:
  terraform:
    vpc:
      hooks:
        store-outputs:
          outputs:
            vpc_id: .vpc_id
```

### When to Use Component-Specific Hooks

Use complete component-specific hooks when:
- Component has unique hook requirements not shared with others
- Hook structure differs significantly between components
- Explicit self-contained configuration is preferred

### Template Functions

Hooks support all Atmos template functions:

```yaml
hooks:
  notify:
    events: [after-terraform-apply]
    command: exec
    args:
      - echo
      - "Deployed {{ .atmos_component }} to {{ .stage }}"
```

## Debugging

### View Component Hooks

```bash
# See all hooks for a component
atmos describe component <component> -s <stack>

# See just the hooks section
atmos describe component <component> -s <stack> | yq '.hooks'
```

### Verify Inheritance Chain

```bash
# View component configuration sources
atmos describe component <component> -s <stack> | yq '.imports'
```

### Compare Against Reference

Working examples available in `tests/test-cases/hooks-component-scoped/`

## Anti-Patterns to Avoid

While the system works correctly, certain configurations can be confusing or error-prone:

### ❌ Anti-Pattern 1: Defining All Outputs in Defaults

**Don't do this**:
```yaml
# _defaults.yaml - Confusing
hooks:
  store-outputs:
    outputs:
      vpc_id: .vpc_id          # Will apply to all components
      cluster_id: .cluster_id  # Will apply to all components
```

**Why**: All components would inherit all outputs, even if they don't produce them.

**Do this instead**:
```yaml
# _defaults.yaml - Clear
hooks:
  store-outputs:
    events: [after-terraform-apply]
    command: store
    # No outputs - defined per component
```

### ❌ Anti-Pattern 2: Hooks at Top-Level of Stack File

**Don't do this**:
```yaml
# stack.yaml - Ambiguous
hooks:
  some-hook:
    events: [after-terraform-apply]

components:
  terraform:
    vpc: {}
    rds: {}
```

**Why**: Unclear which components the hook applies to.

**Do this instead**:
```yaml
# stack.yaml - Clear
components:
  terraform:
    vpc:
      hooks:
        vpc-hook:
          events: [after-terraform-apply]
```

### ❌ Anti-Pattern 3: Duplicating Hook Structure

**Don't do this**:
```yaml
# Duplicated structure in each component
components:
  terraform:
    vpc:
      hooks:
        store:
          events: [after-terraform-apply]
          command: store
          name: prod/ssm
          outputs: {vpc_id: .id}
    rds:
      hooks:
        store:
          events: [after-terraform-apply]
          command: store
          name: prod/ssm
          outputs: {cluster_id: .id}
```

**Do this instead**:
```yaml
# _defaults.yaml
hooks:
  store:
    events: [after-terraform-apply]
    command: store
    name: prod/ssm

# Components just add outputs
components:
  terraform:
    vpc:
      hooks:
        store:
          outputs: {vpc_id: .id}
```

## Related Documentation

- [Lifecycle Hooks](https://atmos.tools/core-concepts/stacks/hooks)
- [Stack Configuration](https://atmos.tools/core-concepts/stacks)
- [Component Inheritance](https://atmos.tools/core-concepts/components)

## Reference Implementation

Complete working examples: `tests/test-cases/hooks-component-scoped/`

## Future Enhancements

Potential future capabilities:

1. **Hook dependencies** - Specify execution order across components
2. **Conditional execution** - Run hooks based on component state
3. **Hook templates** - Parameterized hook definitions
4. **Validation schemas** - Validate hook configurations against schemas
