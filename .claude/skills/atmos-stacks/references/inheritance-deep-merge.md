# Inheritance and Deep-Merge Reference

This reference covers Atmos deep-merge algorithm, component inheritance via metadata fields, multiple inheritance, and the override precedence order.

## Deep-Merge Algorithm

Atmos uses recursive deep-merging to combine configuration from multiple sources (imports, inheritance, inline definitions). The merge rules are:

### Scalar Values (strings, numbers, booleans)

Later values replace earlier values. The higher-priority source wins.

```yaml
# From imported defaults
vars:
  instance_type: t3.micro
  enabled: false

# From component definition (higher priority)
vars:
  instance_type: t3.large    # Replaces t3.micro
  enabled: true               # Replaces false
```

### Maps / Objects

Maps are recursively merged. Keys from both sources are combined. When both sources define the same key, the higher-priority value wins for that key, but other keys from the lower-priority source are preserved.

```yaml
# From base component
vars:
  tags:
    ManagedBy: Atmos
    Team: Platform

# From derived component (higher priority)
vars:
  tags:
    Environment: Production
    Team: SRE               # Overrides "Platform"

# Result (deep-merged)
vars:
  tags:
    ManagedBy: Atmos        # Preserved from base
    Team: SRE               # Overridden by derived
    Environment: Production # Added by derived
```

### Lists / Arrays

Lists are NOT merged -- the higher-priority list entirely replaces the lower-priority list. This is a critical distinction from map merge behavior.

```yaml
# From base component
vars:
  availability_zones:
    - us-east-1a
    - us-east-1b
    - us-east-1c

# From derived component (higher priority)
vars:
  availability_zones:
    - us-west-2a
    - us-west-2b

# Result (replaced, NOT appended)
vars:
  availability_zones:
    - us-west-2a
    - us-west-2b
```

## Override Precedence Order

When computing the final configuration for a component, Atmos merges from multiple sources in a defined order. Lower-numbered sources have lower priority; higher-numbered sources override them:

1. **Global scope** -- `vars:`, `env:`, `settings:`, `hooks:` at the top level of any imported file
2. **Component-type scope** -- `terraform.vars:`, `terraform.env:`, etc.
3. **Inherited base components** -- Configuration from `metadata.inherits` list, processed in list order (later entries override earlier entries)
4. **Component inline definition** -- `components.terraform.<name>.vars:` etc.
5. **Overrides** -- `overrides:`, `terraform.overrides:`, `helmfile.overrides:` (highest priority for the sections they affect)

Within each level, imports are processed in the order they appear in the `import` list. Later imports override earlier imports.

### Precedence Example

```yaml
# stacks/catalog/vpc/_defaults.yaml (imported first)
vars:
  tags:
    Team: Platform              # Priority 1: global scope from import

terraform:
  vars:
    terraform_version: "1.5"    # Priority 2: component-type scope

components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
      vars:
        enabled: true           # Priority 3: base component
        max_subnets: 3

# stacks/orgs/acme/plat/prod/us-east-1.yaml (top-level stack)
import:
  - catalog/vpc/_defaults

vars:
  stage: prod                   # Priority 1: global scope (merged with import)

components:
  terraform:
    vpc:
      metadata:
        inherits:
          - vpc/defaults        # Priority 3: inherited
      vars:
        max_subnets: 6          # Priority 4: inline (overrides inherited value)
        vpc_cidr: "10.0.0.0/16"
```

## Component Inheritance

Component inheritance allows one Atmos component to inherit configuration from another using `metadata.inherits`.

### Single Inheritance

```yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
        component: vpc
      vars:
        enabled: true
        nat_gateway_enabled: true
        max_subnet_count: 3

    vpc:
      metadata:
        inherits:
          - vpc/defaults
      vars:
        max_subnet_count: 2     # Overrides the inherited value
```

The `vpc` component receives all configuration from `vpc/defaults`, then its own inline values are deep-merged on top. Only `max_subnet_count` differs.

### metadata.component

The `metadata.component` field specifies which Terraform root module the Atmos component maps to. This allows the Atmos component name to differ from the Terraform module directory name:

```yaml
components:
  terraform:
    vpc-prod:
      metadata:
        component: vpc          # Points to components/terraform/vpc/
      vars:
        environment: prod

    vpc-staging:
      metadata:
        component: vpc          # Same Terraform module, different config
      vars:
        environment: staging
```

Both `vpc-prod` and `vpc-staging` use `components/terraform/vpc/` as their Terraform root module but maintain separate state and configurations.

### metadata.inherits

The `metadata.inherits` field is a list of component names from which to inherit configuration. The base components must be defined in the same stack (either inline or via imports).

```yaml
components:
  terraform:
    vpc:
      metadata:
        inherits:
          - vpc/defaults
```

Inherited sections include `vars`, `env`, `settings`, `hooks`, `backend`, `backend_type`, `providers`, and `command`. The `metadata` section itself is NOT inherited (except `metadata.custom` when configured).

## Multiple Inheritance

A component can inherit from multiple base components. The `inherits` list is processed in order, with later entries overriding earlier entries:

```yaml
components:
  terraform:
    # Abstract trait: defaults
    base/defaults:
      metadata:
        type: abstract
      vars:
        enabled: true
        tags:
          managed_by: atmos

    # Abstract trait: logging
    base/logging:
      metadata:
        type: abstract
      vars:
        logging_enabled: true
        log_retention_days: 30

    # Abstract trait: production settings
    base/production:
      metadata:
        type: abstract
      vars:
        multi_az: true
        deletion_protection: true
        tags:
          environment: production

    # Concrete component inheriting from all three
    rds:
      metadata:
        component: rds
        inherits:
          - base/defaults       # Applied first
          - base/logging        # Applied second
          - base/production     # Applied third (highest precedence among bases)
      vars:
        name: my-database       # Inline values have highest precedence
```

The merge order for this component:

1. `base/defaults` vars
2. `base/logging` vars deep-merged on top
3. `base/production` vars deep-merged on top
4. Inline `rds` vars deep-merged on top (highest priority)

Result:

```yaml
vars:
  enabled: true                 # from base/defaults
  tags:
    managed_by: atmos           # from base/defaults (preserved)
    environment: production     # from base/production (deep-merged)
  logging_enabled: true         # from base/logging
  log_retention_days: 30        # from base/logging
  multi_az: true                # from base/production
  deletion_protection: true     # from base/production
  name: my-database             # from inline
```

## Abstract vs Real Components

### Abstract Components

Marked with `metadata.type: abstract`. They serve as blueprints and cannot be deployed:

```yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
      vars:
        enabled: true
```

Attempting to deploy an abstract component produces an error:

```
abstract component 'vpc/defaults' cannot be provisioned since it's explicitly
prohibited from being deployed by 'metadata.type: abstract' attribute
```

### Real Components (Default)

If `metadata.type` is not specified, the component defaults to `real` and can be deployed with `atmos terraform apply`.

## The Overrides Section

The `overrides` section is a special mechanism that applies configuration changes only to components defined in the current manifest and its imports, not to all components in the top-level stack.

This is critical for team-based organization where different teams manage different sets of components:

```yaml
# stacks/teams/testing.yaml
import:
  - catalog/terraform/test-component
  - catalog/terraform/test-component-override

overrides:
  env:
    TEST_ENV_VAR1: "overridden-value"
  vars:
    custom_tag: override-value

terraform:
  overrides:
    settings:
      spacelift:
        autodeploy: true
    command: tofu
```

The `overrides` in `testing.yaml` affect only components from `test-component` and `test-component-override`. Components from other team manifests imported into the same top-level stack are unaffected.

### Overrides Scope Rules

- `overrides:` at global scope affects all component types in the current manifest and its imports.
- `terraform.overrides:` affects only Terraform components in the current manifest. Deep-merged with global overrides, with `terraform.overrides` taking higher priority.
- `helmfile.overrides:` affects only Helmfile components, similarly deep-merged with global overrides.
- Overrides defined inline in a manifest take precedence over imported overrides.
- The order of imports matters: overrides from an imported manifest only affect components imported AFTER the overrides manifest in the import list. However, overrides defined inline in the manifest affect ALL components, including those imported before the overrides.

## Debugging Inheritance and Merge Results

Use `atmos describe component` to see the fully resolved configuration including inheritance chain:

```bash
atmos describe component vpc -s plat-ue2-prod
```

The output includes the `overrides` section (showing what overrides were applied), the final merged `vars`, `env`, `settings`, and the `metadata` showing the inheritance chain.

Use `atmos describe stacks` with filters to compare configurations across stacks:

```bash
atmos describe stacks --components vpc --sections vars,metadata
```
