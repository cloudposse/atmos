# Spacelift Integration Reference

## Overview

Atmos natively supports Spacelift for Terraform/OpenTofu automation. The integration uses a
Terraform module (`terraform-spacelift-cloud-infrastructure-automation`) that reads YAML stack
configurations and provisions Spacelift resources automatically.

Cloud Posse provides two Terraform components for Spacelift:
- **Worker Pool component** -- Provisions a Spacelift Worker Pool (`spacelift/worker-pool`)
- **Admin Stack component** -- Provisions Spacelift Stacks (`spacelift/admin-stack`)

## Stack Configuration: `settings.spacelift`

All Spacelift-specific settings are configured in the `settings.spacelift` section of component
configurations. These settings control how Spacelift manages each component/stack pair.

### Complete Settings Reference

```yaml
components:
  terraform:
    my-component:
      settings:
        spacelift:
          # Whether this component is managed in Spacelift
          workspace_enabled: true

          # Whether this is an administrative stack (manages other stacks)
          administrative: false

          # Automatically apply changes on merge (no manual confirmation)
          autodeploy: true

          # Commands to run before terraform init
          before_init: []

          # Path to the Terraform component directory
          component_root: components/terraform/my-component

          # Human-readable description
          description: "My component"

          # Whether to automatically destroy resources if the stack is deleted
          stack_destructor_enabled: false

          # Name of the Spacelift worker pool to use (null = default pool)
          worker_pool_name: null

          # List of policy names to enable
          policies_enabled: []

          # Whether to enable administrative trigger policy
          administrative_trigger_policy_enabled: false

          # List of policy IDs to attach
          policies_by_id_enabled:
            - my-policy-id

          # Terraform or OpenTofu workflow tool
          terraform_workflow_tool: TERRAFORM  # or OPEN_TOFU

          # Custom stack name (overrides the auto-generated name)
          # stack_name: custom-name

          # Labels for organizing and filtering stacks
          # labels:
          #   - env:production
          #   - team:platform
```

### Key Settings Explained

#### `workspace_enabled`

Controls whether a Spacelift stack is created for this component/stack pair. Set to `true` to
manage the component in Spacelift, `false` to skip it.

```yaml
settings:
  spacelift:
    workspace_enabled: true
```

#### `administrative`

Marks a stack as administrative. Administrative stacks can manage other Spacelift stacks and
typically have elevated permissions. Admin stacks are used to bootstrap the Spacelift environment.

```yaml
settings:
  spacelift:
    administrative: true
    # Admin stacks typically don't use normal child policies
    policies_enabled: []
    administrative_trigger_policy_enabled: false
    policies_by_id_enabled:
      - trigger-administrative-policy
```

#### `autodeploy`

When `true`, Spacelift automatically applies changes after a successful plan, without requiring
manual confirmation. Typically enabled for non-production environments and disabled for production.

```yaml
# Dev environment
settings:
  spacelift:
    autodeploy: true

# Prod environment
settings:
  spacelift:
    autodeploy: false
```

#### `component_root`

Specifies the path to the Terraform component directory relative to the repository root. This
tells Spacelift where to find the Terraform code for this component.

```yaml
settings:
  spacelift:
    component_root: components/terraform/vpc
```

#### `terraform_workflow_tool`

Selects whether to use Terraform or OpenTofu for the workflow.

```yaml
# Per-component
settings:
  spacelift:
    terraform_workflow_tool: OPEN_TOFU

# Or globally in a top-level manifest
settings:
  spacelift:
    terraform_workflow_tool: OPEN_TOFU
```

#### `before_init`

A list of shell commands to run before `terraform init`. Useful for setting up authentication
or downloading dependencies.

```yaml
settings:
  spacelift:
    before_init:
      - "aws configure set region us-east-1"
      - "echo 'Initializing...'"
```

## Stack Dependencies with `settings.depends_on`

Atmos supports Spacelift Stack Dependencies through the `settings.depends_on` section. This
defines which other components must be provisioned before the current component.

### Schema

Each dependency is a map entry with:
- `component` (required) -- The Atmos component name
- `namespace` (optional) -- The namespace context
- `tenant` (optional) -- The tenant context
- `environment` (optional) -- The environment context
- `stage` (optional) -- The stage context

### Examples

```yaml
components:
  terraform:
    eks/cluster:
      settings:
        depends_on:
          # Same stack dependency
          1:
            component: "vpc"
          # Cross-stack dependency (same tenant/environment, different stage)
          2:
            component: "dns-zone"
            stage: "prod"
          # Fully qualified cross-stack dependency
          3:
            component: "iam-roles"
            tenant: "core"
            environment: "gbl"
            stage: "prod"
```

Context variables in `depends_on` map to Atmos stack name segments. If only `component` is
provided, the dependency is in the same stack. Adding context variables references components
in other stacks.

## Global vs. Per-Component Configuration

Spacelift settings follow Atmos inheritance. Define defaults at the global level and override
per-component.

### Global Defaults

```yaml
# stacks/orgs/acme/_defaults.yaml
settings:
  spacelift:
    workspace_enabled: true
    autodeploy: false
    terraform_workflow_tool: TERRAFORM
    stack_destructor_enabled: false
```

### Environment Overrides

```yaml
# stacks/orgs/acme/plat/dev/_defaults.yaml
settings:
  spacelift:
    autodeploy: true  # Auto-deploy in dev
```

### Component Overrides

```yaml
# stacks/catalog/vpc/defaults.yaml
components:
  terraform:
    vpc:
      settings:
        spacelift:
          workspace_enabled: true
          component_root: components/terraform/vpc
          description: "VPC networking"
```

## Spacelift Policies

Spacelift supports several policy types that can be attached to stacks:

- **Plan policies** -- Control whether a plan is allowed
- **Push policies** -- Control which Git pushes trigger runs
- **Trigger policies** -- Define dependencies between stacks
- **Approval policies** -- Require approvals before apply
- **Notification policies** -- Send notifications on events

Attach policies by ID:

```yaml
settings:
  spacelift:
    policies_by_id_enabled:
      - plan-policy-restrict-regions
      - push-policy-main-branch-only
      - approval-policy-prod
```

Or enable named policies:

```yaml
settings:
  spacelift:
    policies_enabled:
      - plan-default
      - push-default
```

## Labels

Spacelift labels can be used for organizing, filtering, and applying policies to stacks:

```yaml
settings:
  spacelift:
    labels:
      - "env:production"
      - "team:platform"
      - "cost-center:infrastructure"
      - "folder:networking"
```

Labels are commonly used with Spacelift's policy system to apply rules to groups of stacks
based on their labels.

## Integration with `atmos describe dependents`

Use `atmos describe dependents` to understand the dependency graph between components. This
is especially useful when configuring Spacelift stack dependencies, as it shows which
components depend on a given component and would need to be updated when that component changes.
