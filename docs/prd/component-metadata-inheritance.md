# Component Configuration Inheritance

## Overview

This document describes Atmos's configuration inheritance system, which enables Component-Oriented Programming (COP)
principles through deep merging of configurations at multiple scopes. The system provides a predictable precedence order
where more specific configurations override less specific ones.

## Problem Statement

### Current State

Atmos supports a rich inheritance system that allows configurations to be inherited and overridden at multiple scopes:

1. Global CLI configuration (`atmos.yaml`)
2. Global stack-level configuration
3. Component-type level (terraform/helmfile/packer)
4. Base component inheritance via `metadata.inherits`
5. Component-level configuration
6. Component overrides (highest precedence)

### Challenges

1. **Documentation gaps** - The inheritance system is powerful but complex; users need clear guidance
2. **Merge behavior confusion** - Lists replace (don't append), maps deep-merge - this surprises users
3. **Inheritance order complexity** - Multiple inheritance uses C3 linearization, which isn't intuitive
4. **metadata.component vs inheritance** - Users confuse physical component path with inheritance chain

## Architecture

### Configuration Hierarchy

```
+-------------------------------------------------------------+
|                    CONFIGURATION SCOPES                     |
|              (from LOWEST to HIGHEST precedence)            |
+-------------------------------------------------------------+

+-------------------------------------------------------------+
| 1. Global vars: (stack root level)                          |
|    - Defined at root of stack manifest files                |
|    - Applies to ALL components in the stack                 |
+-------------------------------------------------------------+
                             |
                             v
+-------------------------------------------------------------+
| 2. Component-Type vars (terraform.vars:, helmfile.vars:)    |
|    - Defined under terraform:, helmfile:, or packer:        |
|    - Applies to all components of that type                 |
+-------------------------------------------------------------+
                             |
                             v
+-------------------------------------------------------------+
| 3. Base Component Inheritance Chain                         |
|    - Processed via metadata.inherits list                   |
|    - Uses C3 linearization (left-to-right, later wins)      |
|    - Recursive: base components can inherit too             |
+-------------------------------------------------------------+
                             |
                             v
+-------------------------------------------------------------+
| 4. Component-Level vars                                     |
|    - Defined directly on the component                      |
|    - Overrides all inherited values                         |
+-------------------------------------------------------------+
                             |
                             v
+-------------------------------------------------------------+
| 5. Component overrides.vars (HIGHEST PRECEDENCE)            |
|    - Defined in overrides: section                          |
|    - Final say - overrides everything else                  |
+-------------------------------------------------------------+
```

### Scope Definitions

#### 1. Global CLI Configuration (`atmos.yaml`)

The root configuration file defines project-wide settings:

```yaml
# atmos.yaml
base_path: "."
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  name_pattern: "{tenant}-{environment}-{stage}"
```

This configuration affects:

- Component discovery paths
- Stack file locations
- Naming conventions
- Backend defaults
- Global CLI behavior

#### 2. Global Stack Level

Variables defined at the root of stack manifest files apply to ALL components:

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
vars:
  environment: prod
  region: us-east-1
  tags:
    Environment: Production
    ManagedBy: Atmos

settings:
  spacelift:
    workspace_enabled: true

env:
  AWS_PROFILE: acme-prod
```

**Sections that support global scope:**

- `vars` - Variables passed to components
- `settings` - Atmos-specific settings (spacelift, atlantis, etc.)
- `env` - Environment variables
- `auth` - Authentication configuration

#### 3. Component-Type Level

Variables under `terraform:`, `helmfile:`, or `packer:` apply to all components of that type:

```yaml
# stacks/orgs/acme/plat/prod/_defaults.yaml
terraform:
  vars:
    terraform_version: "1.5"
  settings:
    spacelift:
      runner_image: "public.ecr.aws/spacelift/runner-terraform"
  backend_type: s3
  backend:
    bucket: "acme-terraform-state"
    region: "us-east-1"
    dynamodb_table: "terraform-locks"
  providers:
    aws:
      region: us-east-1

helmfile:
  vars:
    helm_timeout: 300
```

**Terraform-specific sections:**

- `vars` - Terraform variables
- `settings` - Terraform-specific settings
- `env` - Environment variables for terraform commands
- `command` - Override terraform binary (e.g., `tofu`)
- `backend_type` - Backend type (s3, gcs, azurerm, etc.)
- `backend` - Backend configuration
- `remote_state_backend_type` - For reading remote state
- `remote_state_backend` - Remote state backend config
- `providers` - Provider configurations
- `hooks` - Terraform lifecycle hooks

#### 4. Base Component Inheritance

Components can inherit from one or more base components using `metadata.inherits`:

```yaml
# stacks/catalog/vpc.yaml
components:
  terraform:
    vpc-defaults:
      metadata:
        type: abstract  # Cannot be deployed directly
      vars:
        public_subnets_enabled: false
        nat_gateway_enabled: false
        nat_instance_enabled: false
        max_subnet_count: 3
        vpc_flow_logs_enabled: true
```

**Key concepts:**

- **Abstract components** (`metadata.type: abstract`) - Templates that cannot be deployed
- **Derived components** - Inherit from base components via `metadata.inherits`
- **Inheritance chain** - Base components can themselves inherit from other bases

#### 5. Component-Level Configuration

Variables defined directly on a component override all inherited values:

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
components:
  terraform:
    vpc:
      metadata:
        component: infra/vpc  # Physical terraform component
        inherits:
          - vpc-defaults      # Inherit from base
      vars:
        vpc_cidr: "10.0.0.0/16"
        public_subnets_enabled: true  # Override inherited value
```

#### 6. Component Overrides (Highest Precedence)

The `overrides:` section has the final say:

```yaml
components:
  terraform:
    vpc:
      vars:
        vpc_cidr: "10.0.0.0/16"
      overrides:
        vars:
          vpc_cidr: "172.16.0.0/16"  # This wins!
        env:
          TF_LOG: DEBUG
```

## Inheritance Types

### Single Inheritance

A component derives from one base component:

```
    +----------------+
    |  vpc-defaults  |
    +----------------+
           |
     +-----+-----+
     |           |
     v           v
+------------+ +---------------+
| vpc-prod   | | vpc-dev       |
+------------+ +---------------+
```

**Example:**

```yaml
# Base component (catalog)
vpc-defaults:
  metadata:
    type: abstract
  vars:
    public_subnets_enabled: false
    nat_gateway_enabled: false

# Derived component (stack)
vpc:
  metadata:
    inherits:
      - vpc-defaults
  vars:
    public_subnets_enabled: true  # Override
```

### Multiple Inheritance

A component inherits from multiple base components. Later items in the list override earlier ones:

```
+------------------+     +-------------------+
| base-networking  |     | base-monitoring   |
+------------------+     +-------------------+
         |                        |
         +----------+-------------+
                    |
                    v
           +----------------+
           | production-vpc |
           +----------------+
```

**Processing order:**

```yaml
metadata:
  inherits:
    - componentA  # Processed first
    - componentB  # Processed second, overrides componentA
```

1. Process `componentA`'s full inheritance chain recursively
2. Process `componentB`'s full inheritance chain recursively
3. Merge: `componentB` overrides `componentA`
4. Current component overrides everything

### Multilevel Inheritance

Inheritance chains where A -> B -> C:

```
+----------------+     +----------------------+     +----------------+
| base-component | --> | intermediate-component| --> | final-component|
+----------------+     +----------------------+     +----------------+
```

### Hierarchical Inheritance (C3 Linearization)

Complex inheritance combining multiple and multilevel inheritance. Uses **C3 linearization** algorithm (similar to
Python's Method Resolution Order):

```
+------------------+
| base-component-1 |
+------------------+
         |
         v
+--------------------+     +------------------+
| derived-component-1| <-- | base-component-2 |
+--------------------+     +------------------+
         |                        |
         +----------+-------------+
                    |
                    v
           +---------------------+
           | derived-component-2 |
           +---------------------+
```

**Processing order for `derived-component-2`:**

1. `base-component-2` processed first (first in inherits list)
2. `base-component-1` processed (base of `derived-component-1`)
3. `derived-component-1` processed, overrides previous
4. `derived-component-2` overrides all

**Console output:**

```
Inheritance: derived-component-2 -> derived-component-1 -> base-component-1 -> base-component-2
```

## Merge Behavior

### Deep Merge Rules

| Type               | Behavior                                        | Example                                 |
|--------------------|-------------------------------------------------|-----------------------------------------|
| **Maps/Objects**   | Recursively merged (nested keys combined)       | `tags.Team` + `tags.Env` = both present |
| **Scalars**        | Later value wins (strings, numbers, bools)      | `"prod"` overwrites `"dev"`             |
| **Lists/Arrays**   | Later list **REPLACES** entirely (NOT appended) | `[a,b]` overwrites `[c,d]` = `[a,b]`    |
| **YAML Functions** | Deferred evaluation via `MergeWithDeferred()`   | Functions evaluated after merge         |

### Map Merge Example

```yaml
# catalog/_defaults.yaml
vars:
  tags:
    Team: Platform

# prod/_defaults.yaml
vars:
  tags:
    Environment: Production

# component level
vars:
  vpc_cidr: "10.0.0.0/16"
  tags:
    Name: prod-vpc

# RESULT:
vars:
  vpc_cidr: "10.0.0.0/16"
  tags:
    Team: Platform           # From catalog (preserved)
    Environment: Production  # From prod (preserved)
    Name: prod-vpc          # From component (added)
```

### List Replace Example

```yaml
# base component
vars:
  availability_zones:
    - us-east-1a
    - us-east-1b
    - us-east-1c

# derived component
vars:
  availability_zones:
    - us-west-2a
    - us-west-2b

# RESULT (list replaced, NOT merged):
vars:
  availability_zones:
    - us-west-2a
    - us-west-2b
```

## Key Distinction: metadata.component vs metadata.inherits

| Attribute               | Purpose                                          | Inheritance Behavior                          |
|-------------------------|--------------------------------------------------|-----------------------------------------------|
| `metadata.component`    | Physical Terraform component directory path      | **NOT inherited** - must be set per component |
| `metadata.inherits`     | List of components to inherit configuration from | Specifies inheritance chain                   |
| `component` (top-level) | Legacy single inheritance                        | **Inherited** - passed through chain          |

### Example

```yaml
components:
  terraform:
    # Base component
    vpc-defaults:
      metadata:
        type: abstract
        component: infra/vpc  # Points to components/terraform/infra/vpc
      vars:
        max_subnets: 3

    # Derived component 1
    vpc/primary:
      metadata:
        component: infra/vpc  # MUST specify - not inherited from vpc-defaults
        inherits:
          - vpc-defaults
      vars:
        name: primary-vpc

    # Derived component 2 - uses same terraform code
    vpc/secondary:
      metadata:
        component: infra/vpc  # MUST specify - not inherited
        inherits:
          - vpc-defaults
      vars:
        name: secondary-vpc
```

## Implementation Details

### Key Code Locations

| Function                         | File                                                                        | Purpose                                           |
|----------------------------------|-----------------------------------------------------------------------------|---------------------------------------------------|
| `ProcessStackConfig()`           | `internal/exec/stack_processor_process_stacks.go:27`                        | Main entry point for stack processing             |
| `mergeComponentConfigurations()` | `internal/exec/stack_processor_merge.go:16-258`                             | Merges all configuration levels                   |
| `processComponentInheritance()`  | `internal/exec/stack_processor_process_stacks_helpers_inheritance.go:14-48` | Handles inheritance chain resolution              |
| `ProcessBaseComponentConfig()`   | `internal/exec/stack_processor_utils.go:1446-1492`                          | Recursive base component processing               |
| `extractComponentSections()`     | `internal/exec/stack_processor_process_stacks_helpers_extraction.go:14-150` | Extracts component configuration sections         |
| `processComponentOverrides()`    | `internal/exec/stack_processor_process_stacks_helpers_overrides.go:14-107`  | Handles overrides (highest precedence)            |
| `MergeWithDeferred()`            | `pkg/merge/merge.go`                                                        | Deep merge with deferred YAML function evaluation |

### Merge Implementation

From `internal/exec/stack_processor_merge.go:20-27`:

```go
// Merge order for vars (same pattern for settings, env, auth, etc.)
m.MergeWithDeferred(
atmosConfig,
[]map[string]any{
opts.GlobalVars,          // 1. Global stack-level vars
result.BaseComponentVars, // 2. Inherited from base components
result.ComponentVars, // 3. Component-level vars
result.ComponentOverridesVars, // 4. Overrides (highest precedence)
})
```

### Caching

Base component configurations are cached to avoid redundant processing:

- **Cache key:** `"stack:component:baseComponent"`
- **Location:** `internal/exec/stack_processor_utils.go:41-51`
- **Cache disabled when:** Provenance tracking is enabled

## Component Metadata Reference

The `metadata` section is a special configuration block that controls how Atmos interprets and manages a component.
Unlike other sections (`vars`, `settings`, `env`), the `metadata` section is **only valid inside component definitions**
and has unique inheritance behavior.

### Metadata Scope

```yaml
# VALID: Inside component definition
components:
  terraform:
    vpc:
      metadata:
        component: vpc/network
        inherits:
          - vpc/defaults

# INVALID: Cannot be used at global level or under terraform: defaults
terraform:
  metadata: # This is NOT supported
    type: abstract
```

### Supported Metadata Attributes

#### `component`

**Type:** `string`
**Inherited:** NO
**Default:** Same as the Atmos component name

Specifies the path to the physical Terraform/Helmfile/Packer component directory, relative to your components base path.

```yaml
components:
  terraform:
    # Atmos component name: "vpc-prod"
    # Physical terraform component: "components/terraform/vpc"
    vpc-prod:
      metadata:
        component: vpc  # Points to components/terraform/vpc
      vars:
        environment: prod

    # Different Atmos component using the same physical terraform code
    vpc-staging:
      metadata:
        component: vpc  # Same physical component
      vars:
        environment: staging
```

**Important:** This attribute is NOT inherited from base components. Each derived component must explicitly set
`metadata.component` if it differs from the Atmos component name.

---

#### `inherits`

**Type:** `list[string]`
**Inherited:** N/A (defines inheritance)
**Default:** `[]` (no inheritance)

Defines a list of component names from which this component inherits configuration. Components are processed in order,
with later items overriding earlier ones.

```yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
      vars:
        enable_dns_hostnames: true

    monitoring/defaults:
      metadata:
        type: abstract
      vars:
        enable_flow_logs: true

    vpc-prod:
      metadata:
        inherits:
          - vpc/defaults        # Processed first
          - monitoring/defaults # Processed second, overrides vpc/defaults
      vars:
        vpc_cidr: "10.0.0.0/16"
```

**Processing Order:** For `inherits: [A, B, C]`:

1. Recursively process A's inheritance chain
2. Recursively process B's inheritance chain (overrides A)
3. Recursively process C's inheritance chain (overrides A and B)
4. Current component's values override everything

---

#### `type`

**Type:** `string` (`"real"` | `"abstract"`)
**Inherited:** YES
**Default:** `"real"`

Marks a component as deployable (`real`) or as a template (`abstract`).

```yaml
components:
  terraform:
    # Abstract: template only, cannot be deployed
    vpc/base:
      metadata:
        type: abstract
      vars:
        enable_dns_hostnames: true

    # Real: can be deployed (default)
    vpc:
      metadata:
        inherits:
          - vpc/base
        # type: real  # Implicit default
      vars:
        vpc_cidr: "10.0.0.0/16"
```

**Abstract components:**

- Cannot be deployed with `atmos terraform apply`
- Do not appear in `atmos describe stacks` output by default
- Serve as templates for other components to inherit from
- Useful for DRY configuration patterns

---

#### `enabled`

**Type:** `boolean`
**Inherited:** YES
**Default:** `true`

Controls whether a component is active in the stack.

```yaml
components:
  terraform:
    expensive-monitoring:
      metadata:
        enabled: false  # Disabled in this stack
      vars:
        retention_days: 90
```

**When `enabled: false`:**

- Component is skipped during `atmos terraform apply`
- Component does not appear in active stack listings
- Useful for conditionally disabling components per environment

---

#### `locked`

**Type:** `boolean`
**Inherited:** YES
**Default:** `false`

Prevents modifications to a component.

```yaml
components:
  terraform:
    core-network:
      metadata:
        locked: true  # Prevent changes
      vars:
        vpc_cidr: "10.0.0.0/8"
```

**When `locked: true`:**

- Atmos will warn or prevent changes to the component
- Useful for protecting critical infrastructure components
- Guards against accidental modifications in production

---

#### `terraform_workspace`

**Type:** `string`
**Inherited:** YES
**Default:** Auto-calculated from stack name

Overrides the Terraform workspace name with a literal string value.

```yaml
components:
  terraform:
    vpc:
      metadata:
        terraform_workspace: "custom-workspace-name"
```

By default, Atmos calculates workspace names automatically based on the stack naming pattern. Use this field when you
need explicit control over the workspace name.

---

#### `terraform_workspace_pattern`

**Type:** `string`
**Inherited:** YES
**Default:** Uses `atmos.yaml` workspace pattern

Overrides the Terraform workspace name using a pattern with context tokens.

```yaml
components:
  terraform:
    vpc:
      metadata:
        terraform_workspace_pattern: "{tenant}-{environment}-{stage}"
```

**Supported tokens:**

| Token              | Description                                         |
|--------------------|-----------------------------------------------------|
| `{namespace}`      | The namespace from context variables                |
| `{tenant}`         | The tenant from context variables                   |
| `{environment}`    | The environment from context variables              |
| `{region}`         | The region from context variables                   |
| `{stage}`          | The stage from context variables                    |
| `{attributes}`     | The attributes from context variables               |
| `{component}`      | The Atmos component name                            |
| `{base-component}` | The base component name (from `metadata.component`) |

**Precedence:** `terraform_workspace` (literal) > `terraform_workspace_pattern` (pattern) > default calculation

---

#### `custom`

**Type:** `map[string]any`
**Inherited:** NO
**Default:** `{}`

A user extension point for storing arbitrary metadata that Atmos preserves but does not interpret.

```yaml
components:
  terraform:
    vpc:
      metadata:
        custom:
          owner: platform-team
          cost_center: "12345"
          tier: critical
          pagerduty_service_id: "PXXXXXX"
          last_reviewed: "2025-01-15"
```

**Use cases:**

- Storing metadata for external tooling (CI/CD pipelines, dashboards)
- Adding labels or annotations readable via `atmos describe stacks`
- Custom categorization that doesn't affect Atmos behavior
- Integration with governance and compliance tools

**Important:** The `custom` section is **NOT inherited** by derived components. Each component must define its own
`custom` values.

---

### Metadata Inheritance Summary

| Attribute                     | Inherited | Description                                      |
|-------------------------------|-----------|--------------------------------------------------|
| `component`                   | **NO**    | Physical component path - must be set explicitly |
| `inherits`                    | N/A       | Defines the inheritance chain                    |
| `type`                        | YES       | Abstract or real component type                  |
| `enabled`                     | YES       | Whether component is active                      |
| `locked`                      | YES       | Whether component is protected                   |
| `terraform_workspace`         | YES       | Literal workspace name override                  |
| `terraform_workspace_pattern` | YES       | Pattern-based workspace name                     |
| `custom`                      | **NO**    | User-defined metadata - not inherited            |

### Metadata Processing Order

When Atmos processes a component's metadata:

1. **Extract metadata** from component definition
2. **Process inheritance** via `metadata.inherits` (if present)
3. **Merge inherited metadata** (type, enabled, locked, workspace settings)
4. **Override with component's own metadata** values
5. **Resolve `metadata.component`** - always from current component, never inherited
6. **Preserve `metadata.custom`** - never merged from inheritance

### Code Reference

Key implementation locations:

- `internal/exec/stack_processor_process_stacks_helpers_extraction.go:91-98` - Metadata extraction
- `internal/exec/stack_processor_process_stacks_helpers_inheritance.go:84-147` - Inheritance processing
- `internal/exec/stack_utils.go:52-58` - Workspace pattern resolution

---

## Configuration Sections

The following sections support the full inheritance hierarchy:

### All Component Types

| Section    | Description                    |
|------------|--------------------------------|
| `vars`     | Variables passed to component  |
| `settings` | Atmos-specific settings        |
| `env`      | Environment variables          |
| `auth`     | Authentication configuration   |
| `metadata` | Component metadata (see above) |

### Terraform-Specific

| Section                     | Description                  |
|-----------------------------|------------------------------|
| `command`                   | Override terraform binary    |
| `backend_type`              | Backend type (s3, gcs, etc.) |
| `backend`                   | Backend configuration        |
| `remote_state_backend_type` | For reading remote state     |
| `remote_state_backend`      | Remote state backend config  |
| `providers`                 | Provider configurations      |
| `hooks`                     | Terraform lifecycle hooks    |

## Best Practices

### 1. Use Abstract Base Components

```yaml
# Good: Abstract component as template
vpc-defaults:
  metadata:
    type: abstract  # Cannot be deployed
  vars:
    public_subnets_enabled: false
```

### 2. Keep Inheritance Chains Shallow

```yaml
# Good: 1-2 levels of inheritance
vpc:
  metadata:
    inherits:
      - vpc-defaults

# Avoid: Deep inheritance chains (hard to debug)
vpc:
  metadata:
    inherits:
      - vpc-level-3  # inherits from vpc-level-2
        # which inherits from vpc-level-1
      # which inherits from vpc-base
```

### 3. Use Imports for Organization

```yaml
import:
  - catalog/vpc/_defaults      # Base component definitions
  - orgs/acme/plat/_defaults   # Organization defaults
  - mixins/monitoring          # Cross-cutting concerns
```

### 4. Document Inheritance in Comments

```yaml
components:
  terraform:
    vpc:
      metadata:
        # Inherits: public_subnets_enabled=false, nat_gateway_enabled=false
        # Overrides: public_subnets_enabled=true
        inherits:
          - vpc-defaults
```

### 5. Use Overrides Sparingly

```yaml
# Overrides are for exceptional cases, not normal configuration
vpc:
  vars:
    normal_setting: "value"  # Normal configuration
  overrides:
    vars:
      emergency_override: "critical"  # Use only when necessary
```

## Debugging Inheritance

### View Resolved Configuration

```bash
# Show fully resolved component configuration
atmos describe component vpc -s prod-us-east-1

# Show inheritance chain in terraform output
atmos terraform plan vpc -s prod-us-east-1
# Output includes: Inheritance: vpc -> vpc-defaults -> ...
```

### Common Issues

| Issue                  | Cause                                     | Solution                        |
|------------------------|-------------------------------------------|---------------------------------|
| Variable not inherited | `metadata.inherits` missing or misspelled | Check inherits list             |
| Wrong value inherited  | Multiple inheritance order                | Reorder inherits list           |
| List values merged     | Expected append                           | Lists replace - use map instead |
| Component not found    | `metadata.component` not set              | Set physical component path     |

## Future Improvements

### Planned Enhancements

1. **Inheritance visualization** - `atmos describe inheritance vpc -s stack`
2. **Merge conflict detection** - Warn on conflicting values
3. **Inheritance linting** - Detect circular dependencies
4. **Provenance tracking** - Show which file each value came from

## References

- [Atmos Inheritance Documentation](https://atmos.tools/howto/inheritance/)
- [Stack Configuration](https://atmos.tools/stacks/)
- [Component-Oriented Programming](https://en.wikipedia.org/wiki/Component-based_software_engineering)
- [C3 Linearization](https://en.wikipedia.org/wiki/C3_linearization)
- [Python Method Resolution Order](https://www.python.org/download/releases/2.3/mro/)

## Changelog

| Version | Date       | Changes                                    |
|---------|------------|--------------------------------------------|
| 1.0     | 2025-12-05 | Initial PRD documenting inheritance system |
