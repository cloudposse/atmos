# Component Type Aliases Example

This example shows how to expose Terraform semantics through another component type name.

`atmos.yaml` declares `opentofu` as an alias for the canonical `terraform` component type:

```yaml
components:
  terraform:
    aliases: opentofu
    base_path: components/terraform
    command: tofu
```

Stack manifests can then use the alias envelope:

```yaml
components:
  opentofu:
    vpc:
      vars:
        cidr_block: 10.10.0.0/16
```

Atmos resolves `opentofu` to Terraform internally for provider lookup, path resolution,
validation, dependencies, and command execution. Describe output preserves the original
stack envelope, so `atmos describe stacks -s dev` renders the component under
`components.opentofu`.

## Usage

```bash
# Run through the alias command
atmos opentofu plan vpc -s dev

# The canonical command still works against the same aliased stack component
atmos terraform plan vpc -s dev

# Describe output preserves components.opentofu
atmos describe stacks -s dev
atmos describe component vpc -s dev
```

The alias does not change the executable. Use `components.terraform.command: tofu`
when you want the Terraform provider integration to run OpenTofu.
