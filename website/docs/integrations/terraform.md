---
title: Terraform Integration
sidebar_position: 2
sidebar_label: Terraform
---

Atmos natively supports opinionated workflows for Terraform and [OpenTofu](/integrations/opentofu).
It's compatible with every version of terraform and designed to work with multiple different versions of Terraform
concurrently. Keep in mind that Atmos does not handle the downloading or installation of Terraform; it assumes that any
required commands are already installed on your system.

Atmos provides many settings that are specific to Terraform and OpenTofu.

## CLI Configuration

All of these settings are defined by default in the [Atmos CLI Configuration](/cli/configuration) found in `atmos.yaml`,
but can also be overridden at any level of the [Stack](/core-concepts/stacks/#schema) configuration.

```yaml
components:
  terraform:
    # The executable to be called by `atmos` when running Terraform commands
    command: "/usr/bin/terraform-1"
    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_BASE_PATH' ENV var, or '--terraform-dir' command-line argument
    # Supports both absolute and relative paths
    base_path: "components/terraform"
    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE' ENV var
    apply_auto_approve: false
    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT' ENV var, or '--deploy-run-init' command-line argument
    deploy_run_init: true
    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE' ENV var, or '--init-run-reconfigure' command-line argument
    init_run_reconfigure: true
    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE' ENV var, or '--auto-generate-backend-file' command-line argument
    auto_generate_backend_file: false
```

## Configuration

The settings for terraform can be defined in multiple places and support inheritance. This ensures that projects can
override the behavior.

The defaults for everything are defined in the `atmos.yaml`.

```yaml
components:
  terraform:
    ...
```

The same settings, can be overridden by Stack configurations at any level:

- `terraform`
- `components.terraform`
- `components.terraform._component_`

For example, we can change the terraform command used by a component (useful for legacy components)

```yaml
components:
  terraform:
    vpc:
      command: "/usr/local/bin/terraform-0.13"
```

## Terraform Provider

A Terraform provider (`cloudposse/terraform-provider-utils`) implements a `data` source that can read the YAML Stack
configurations natively from
within terraform.

## Terraform Module

A Terraform module (`cloudposse/terraform-yaml-stack-config`) wraps the data source.

Here's an example of accessing the variables for a given component from within a Terraform module.

```hcl
module "vars" {
  source = "cloudposse/stack-config/yaml//modules/vars"
  # version     = "x.x.x"

  stack_config_local_path = "./stacks"
  stack                   = "my-stack"
  component_type          = "terraform"
  component               = "my-vpc"

  context = module.this.context
}
```

## Example: Provision Terraform Component

To provision a Terraform component using the `atmos` CLI, run the following commands in the container shell:

```console
atmos terraform plan eks --stack=ue2-dev
atmos terraform apply eks --stack=ue2-dev
```

where:

- `eks` is the Terraform component to provision (from the `components/terraform` folder)
- `--stack=ue2-dev` is the stack to provision the component into

Short versions of all command-line arguments can be used:

```console
atmos terraform plan eks -s ue2-dev
atmos terraform apply eks -s ue2-dev
```

The `atmos terraform deploy` command executes `terraform apply -auto-approve` to provision components in stacks without
user interaction:

```console
atmos terraform deploy eks -s ue2-dev
```
