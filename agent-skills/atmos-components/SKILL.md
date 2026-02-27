---
name: atmos-components
description: "Component architecture: Terraform root modules, abstract components, component inheritance, versioning, mixins, catalog patterns"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Component Architecture

Components are the building blocks of infrastructure in Atmos. Each component is an opinionated, reusable unit of infrastructure-as-code -- typically a Terraform root module -- that solves a specific problem. Atmos separates the component implementation (code) from its configuration (stack manifests), enabling one implementation to be deployed many times with different settings.

## What Components Are

In Atmos, a component consists of two parts:

1. **Implementation** -- The infrastructure code itself (a Terraform root module, Helmfile, or Packer template) stored in the `components/` directory.
2. **Configuration** -- The settings that customize how the component is deployed, defined in stack manifests under the `components` section.

This separation is fundamental: you write the Terraform module once, then configure it differently for each environment, region, and account through stack YAML files.

## Component Types

Atmos natively supports three component types:

| Type | Implementation Location | Purpose |
|------|------------------------|---------|
| Terraform / OpenTofu | `components/terraform/<name>/` | Provision cloud infrastructure resources |
| Helmfile | `components/helmfile/<name>/` | Deploy Helm charts to Kubernetes clusters |
| Packer | `components/packer/<name>/` | Build machine images (AMIs, VM images) |

Terraform is by far the most common type. Custom commands can extend Atmos to support any tooling.

## Directory Structure

Components are stored in your project's `components/` directory, organized by type:

```
components/
  terraform/
    vpc/
      main.tf
      variables.tf
      outputs.tf
      versions.tf
    eks/
      cluster/
        main.tf
        variables.tf
        outputs.tf
    s3-bucket/
      main.tf
      variables.tf
      outputs.tf
    iam-role/
      main.tf
      variables.tf
      outputs.tf
  helmfile/
    nginx-ingress/
      helmfile.yaml
    cert-manager/
      helmfile.yaml
  packer/
    ubuntu-base/
      template.pkr.hcl
```

The base path for Terraform components is configured in `atmos.yaml`:

```yaml
components:
  terraform:
    base_path: "components/terraform"
```

Nested directories are supported. A component at `components/terraform/eks/cluster/` is referenced as `eks/cluster` in stack configurations.

## Component Configuration in Stacks

Components are configured in the `components` section of stack manifests:

```yaml
components:
  terraform:
    vpc:
      metadata:
        component: vpc           # Points to components/terraform/vpc/
      vars:
        cidr_block: "10.0.0.0/16"
        availability_zones:
          - us-east-1a
          - us-east-1b
      settings:
        spacelift:
          workspace_enabled: true

    eks-cluster:
      metadata:
        component: eks/cluster   # Points to components/terraform/eks/cluster/
      vars:
        cluster_name: prod-eks
        kubernetes_version: "1.28"
```

Each component configuration can include these sections:

| Section | Purpose |
|---------|---------|
| `metadata` | Component location, inheritance, type, and Atmos behavior |
| `vars` | Input variables passed to Terraform/Helmfile/Packer |
| `env` | Environment variables set during execution |
| `settings` | Integration metadata (Spacelift, validation, depends_on) |
| `hooks` | Lifecycle event handlers |
| `backend` / `backend_type` | Terraform state backend configuration |
| `providers` | Terraform provider configuration |
| `command` | Override the executable (e.g., `tofu` instead of `terraform`) |
| `auth` | Authentication identity reference |

## Abstract vs Real Components

### Abstract Components

Mark a component as `metadata.type: abstract` to create a blueprint that cannot be deployed directly:

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
        vpc_flow_logs_enabled: true
```

Abstract components:
- Cannot be provisioned with `atmos terraform apply` (Atmos returns an error).
- Do not appear in `atmos describe stacks` output by default.
- Serve as base configurations for real components to inherit from.

### Real Components (Default)

If `metadata.type` is not specified, the component is `real` and can be deployed:

```yaml
components:
  terraform:
    vpc:
      metadata:
        inherits:
          - vpc/defaults
      vars:
        vpc_cidr: "10.0.0.0/16"
```

## Component Inheritance

### Single Inheritance

Use `metadata.inherits` to inherit configuration from a base component:

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

    vpc:
      metadata:
        inherits:
          - vpc/defaults
      vars:
        nat_gateway_enabled: false   # Override inherited value
```

The derived component receives all `vars`, `env`, `settings`, `hooks`, `backend`, `providers`, and `command` from the base, then its own values are deep-merged on top.

### Multiple Inheritance

A component can inherit from multiple bases. Entries are processed in order with later entries having higher precedence:

```yaml
components:
  terraform:
    rds:
      metadata:
        component: rds
        inherits:
          - base/defaults       # Applied first
          - base/logging        # Applied second
          - base/production     # Applied last (highest base precedence)
      vars:
        name: my-database       # Inline has highest precedence
```

This enables composing "traits" -- reusable abstract components that represent independent configuration concerns (logging, security, sizing, environment settings).

### metadata.component

The `metadata.component` field maps an Atmos component name to its Terraform root module directory:

```yaml
components:
  terraform:
    vpc-main:
      metadata:
        component: vpc          # Uses components/terraform/vpc/
      vars:
        name: main-vpc

    vpc-isolated:
      metadata:
        component: vpc          # Same Terraform module
      vars:
        name: isolated-vpc
        enable_internet_gateway: false
```

Both components use the same Terraform code but maintain separate state files and configurations. This is the **multiple component instances** pattern.

## Multiple Component Instances

Deploy the same Terraform module multiple times in the same stack by giving each instance a unique Atmos component name:

```yaml
components:
  terraform:
    vpc/1:
      metadata:
        component: vpc
        inherits:
          - vpc/defaults
      vars:
        name: vpc-1
        ipv4_primary_cidr_block: 10.9.0.0/18

    vpc/2:
      metadata:
        component: vpc
        inherits:
          - vpc/defaults
      vars:
        name: vpc-2
        ipv4_primary_cidr_block: 10.10.0.0/18
```

Each instance has its own Terraform state and is independently deployable:

```bash
atmos terraform apply vpc/1 -s plat-ue2-prod
atmos terraform apply vpc/2 -s plat-ue2-prod
```

## metadata Section Fields

All fields available in the `metadata` section:

| Field | Type | Description |
|-------|------|-------------|
| `component` | string | Path to Terraform root module relative to components base path |
| `inherits` | list | List of component names to inherit configuration from |
| `type` | string | `abstract` (non-deployable) or `real` (default, deployable) |
| `name` | string | Stable logical identity for workspace key prefix |
| `enabled` | boolean | Enable or disable the component (default: true) |
| `locked` | boolean | Prevent modifications to the component |
| `terraform_workspace` | string | Explicit workspace name override |
| `terraform_workspace_pattern` | string | Workspace name pattern with tokens |
| `custom` | map | User-defined metadata (preserved, not interpreted by Atmos) |

## Catalog Patterns

The `stacks/catalog/` directory is the conventional location for reusable component configurations:

```
stacks/
  catalog/
    vpc/
      _defaults.yaml          # Abstract base for all VPC instances
    eks/
      _defaults.yaml
      cluster.yaml
    s3-bucket/
      _defaults.yaml
    iam-role/
      _defaults.yaml
```

Catalog files define abstract components with sensible defaults. Top-level stacks import from the catalog and override only what differs:

```yaml
# stacks/catalog/vpc/_defaults.yaml
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
```

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
import:
  - catalog/vpc/_defaults

components:
  terraform:
    vpc:
      metadata:
        inherits:
          - vpc/defaults
      vars:
        vpc_cidr: "10.0.0.0/16"
```

## Mixins for Reusable Configuration

Mixins are small, focused configuration snippets that alter component behavior. They are typically stored in `stacks/mixins/` and imported into stacks:

```
stacks/
  mixins/
    region/
      us-east-1.yaml
      us-east-2.yaml
      us-west-2.yaml
    stage/
      dev.yaml
      staging.yaml
      prod.yaml
    tenant/
      plat.yaml
```

```yaml
# stacks/mixins/region/us-east-2.yaml
vars:
  region: us-east-2
  environment: ue2
```

```yaml
# stacks/orgs/acme/plat/prod/us-east-2.yaml
import:
  - mixins/region/us-east-2
  - mixins/stage/prod
  - catalog/vpc/_defaults
```

## Remote State Access Between Components

Components can access outputs from other components using the `remote-state` module or YAML functions:

```yaml
# Using YAML function
components:
  terraform:
    eks-cluster:
      vars:
        vpc_id: !terraform.output vpc/vpc_id
        subnet_ids: !terraform.output vpc/private_subnet_ids
```

For the Terraform-side approach, use the `remote-state` module:

```hcl
# In components/terraform/eks/cluster/remote-state.tf
module "vpc" {
  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.5.0"

  component = "vpc"
}
```

This reads the VPC component's Terraform outputs from the same or a different stack.

## Component Versioning Patterns

### Folder-Based Versioning

Maintain multiple versions of a component side by side:

```
components/
  terraform/
    vpc/
      v1/
        main.tf
      v2/
        main.tf
```

```yaml
components:
  terraform:
    vpc:
      metadata:
        name: vpc           # Stable workspace key prefix
        component: vpc/v2   # Physical version path
```

### Vendor-Based Versioning

Use `atmos vendor pull` to pin specific upstream versions. See the atmos-vendoring skill for details.

## Best Practices

1. **One concern per component**: Each component should provision a single logical piece of infrastructure (VPC, EKS cluster, database). Do not combine resources with different lifecycles.

2. **Use abstract base components**: Define catalog defaults as `abstract` components and inherit from them.

3. **Keep inheritance chains shallow**: Limit to 2-3 levels for readability and debuggability.

4. **Use metadata.component for instances**: When deploying the same module multiple times, use `metadata.component` to share the implementation.

5. **Use metadata.name for versioning**: Set `metadata.name` to maintain stable Terraform workspace key prefixes across version upgrades.

6. **Design for reuse**: Components should accept configuration through variables, not hard-coded values. Use the catalog pattern to define sensible defaults.

7. **Use `atmos describe component`**: Always verify the resolved configuration before applying changes.

```bash
atmos describe component vpc -s plat-ue2-prod
```

## References

- [references/component-types.md](references/component-types.md) -- Detailed reference on component types, metadata fields, abstract components, inheritance chains
- [references/examples.md](references/examples.md) -- Concrete configuration examples for VPC, EKS, S3, IAM patterns
