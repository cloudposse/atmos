---
sidebar_position: 8
title: Terraform Integration
---

# Terraform Integration

Atmos natively supports opinionated workflows for terraform.

There are a few settings exposed that specific to terraform.

## Settings

```
# The executable to be called by `atmos` when running terraform commands.
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

The settings for terraform can be defined in multiple places and support inheritance. This ensures that projects can override the behavior.

The defaults everything are defined in the `atmos.yaml`.

```
components:
  terraform:
    ...
```

The same settings, can be overridden by Stack configurations at any level:

- `terraform`
- `components.terraform`
- `components.terraform._component_`

For example, we can change the terraform command used by a component (useful for legacy components)

```
components:
  terraform:
    vpc:
      command: "/usr/local/bin/terraform-0.13"
```

## Terraform Provider

There is a Terraform provider ([`cloudposse/terraform-provider-utils`](https://github.com/cloudposse/terraform-provider-utils)) that implements a `data` source thjat can read the YAML Stack configurations natively from within terraform.

## Terraform Module

There is a Terraform module ([`cloudposse/terraform-yaml-stack-config`](https://github.com/cloudposse/terraform-yaml-stack-config)) that wraps the data source.

Here's an example of how the variables for a component could be accessed from within a Terraform module.

```
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

