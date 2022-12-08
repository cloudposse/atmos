---
title: Component Remote State
sidebar_position: 8
sidebar_label: Remote State
id: remote-state
---

Component Remote State is used when we need to get the outputs of an [Atmos component](/core-concepts/components),
provisioned in the same or a different [Atmos stack](/core-concepts/stacks), and use the outputs as inputs to another Atmos component.

:::info

In Atmos, Remote State is implemented by using these modules:

- [terraform-provider-utils](https://github.com/cloudposse/terraform-provider-utils) - The Cloud Posse Terraform Provider for various utilities,
  including stack configuration management

- [remote-state](https://github.com/cloudposse/terraform-yaml-stack-config/tree/main/modules/remote-state) - Terraform module that loads and processes
  stack configurations from YAML sources and returns remote state outputs for Terraform components

:::

<br/>

[terraform-provider-utils](https://github.com/cloudposse/terraform-provider-utils) is implemented in [Go](https://go.dev/) and uses Atmos `Go`
modules to work with [Atmos CLI config](/cli/configuration) and [Atmos stacks](/core-concepts/stacks). The provider processes stack
configurations to get the final configuration for a component in a given stack. The final component config is then used by
the [remote-state](https://github.com/cloudposse/terraform-yaml-stack-config/tree/main/modules/remote-state) Terraform module to return the remote
state for the component in the stack.

Here is an example.

Suppose that we need to provision two Terraform components:

- [vpc-flow-logs-bucket](https://github.com/cloudposse/atmos/tree/master/examples/complete/components/terraform/infra/vpc-flow-logs-bucket)
- [vpc](https://github.com/cloudposse/atmos/tree/master/examples/complete/components/terraform/infra/vpc)

The `vpc` component needs the outputs from the `vpc-flow-logs-bucket` component to
configure [VPC Flow Logs](https://docs.aws.amazon.com/vpc/latest/userguide/flow-logs.html)
and store them in the S3 bucket.

We will provision the Terraform components in the `ue2-dev` Atmos stack (in the `dev` AWS account by setting `stage = "dev"` and in the `us-east-2`
region by setting `environment = "ue2"`).

## Configure and Provision `vpc-flow-logs-bucket` Atmos Component

In the `stacks/catalog/vpc-flow-logs-bucket.yaml` file, add the following default configuration for the `vpc-flow-logs-bucket` component:

```yaml title="stacks/catalog/vpc-flow-logs-bucket.yaml"
components:
  terraform:
    vpc-flow-logs-bucket-defaults:
      metadata:
        # Setting `metadata.type: abstract` makes the component `abstract`,
        # explicitly prohibiting the component from being deployed.
        # `atmos terraform apply` will fail with an error.
        # If `metadata.type` attribute is not specified, it defaults to `real`.
        # `real` components can be provisioned by `atmos` and CI/CD like Spacelift and Atlantis.
        type: abstract
      # Default variables, which will be inherited and can be overriden in the derived components
      vars:
        force_destroy: false
        lifecycle_rule_enabled: false
        traffic_type: "ALL"
```

<br/>

In the `stacks/ue2-dev.yaml` stack config file, add the following config for the `vpc-flow-logs-bucket` component in the `ue2-dev` Atmos stack:

```yaml title="stacks/ue2-dev.yaml"
# Import the base Atmos component configuration from the `catalog`.
# `import` supports POSIX-style Globs for file names/paths (double-star `**` is supported).
# File extensions are optional (if not specified, `.yaml` is used by default).
import:
  - catalog/vpc-flow-logs-bucket

components:
  terraform:
    vpc-flow-logs-bucket:
      metadata:
        # Point to the Terraform component in `components/terraform` folder
        component: infra/vpc-flow-logs-bucket
        inherits:
          # Inherit all settings and variables from the 
          # `vpc-flow-logs-bucket-defaults` base Atmos component
          - vpc-flow-logs-bucket-defaults
      vars:
        # Define variables that are specific for this component
        # and are not set in the base component
        name: vpc-flow-logs-bucket-1
        # Override the default variables from the base component
        traffic_type: "REJECT"
```

<br/>

Having the stacks configured as shown above, we can now provision the `vpc-flow-logs-bucket` component into the `ue2-dev` stack by executing the
following Atmos commands:

```shell
atmos terraform plan vpc-flow-logs-bucket -s ue2-dev
atmos terraform apply vpc-flow-logs-bucket -s ue2-dev
```

<br/>

## Configure and Provision `vpc` Atmos Component

Having the `vpc-flow-logs-bucket` Atmos component provisioned into the `ue2-dev` Atmos stack, we can now configure the `vpc` Atmos component to obtain
the outputs from the remote state of the `vpc-flow-logs-bucket` component and provision it into the `ue2-dev` stack.

In the `components/terraform/infra/vpc/remote-state.tf` file, we first configure the
[remote-state](https://github.com/cloudposse/terraform-yaml-stack-config/tree/main/modules/remote-state) Terraform module to obtain the remote state
for the `vpc-flow-logs-bucket` component:

```hcl title="components/terraform/infra/vpc/remote-state.tf"
module "vpc_flow_logs_bucket" {
  count = var.vpc_flow_logs_enabled ? 1 : 0

  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.3.1"

  # Specify the Atmos component name (defined in YAML stack config files) 
  # for which to get the remote state outputs
  component = var.vpc_flow_logs_bucket_component_name

  # Override the context variables to point to a different Atmos stack if the 
  # `vpc-flow-logs-bucket` component is provisioned in another AWS account, OU or region
  stage       = try(coalesce(var.vpc_flow_logs_bucket_stage_name, module.this.stage), null)
  environment = try(coalesce(var.vpc_flow_logs_bucket_environment_name, module.this.environment), null)
  tenant      = try(coalesce(var.vpc_flow_logs_bucket_tenant_name, module.this.tenant), null)

  # `context` input is a way to provide the information about the stack (using the context
  # variables `namespace`, `tenant`, `environment`, `stage` defined in the stack config)
  context = module.this.context
}
```

In the `stacks/catalog/vpc.yaml` file, add the following config for the VPC component:

```yaml title="stacks/catalog/vpc.yaml"
components:
  terraform:
    vpc-defaults:
      metadata:
        # Setting `metadata.type: abstract` makes the component `abstract`,
        # explicitly prohibiting the component from being deployed.
        # `atmos terraform apply` will fail with an error.
        # If `metadata.type` attribute is not specified, it defaults to `real`.
        # `real` components can be provisioned by `atmos` and CI/CD like Spacelift and Atlantis.
        type: abstract
      # Default variables, which will be inherited and can be overriden in the derived components
      vars:
        public_subnets_enabled: false
        nat_gateway_enabled: false
        nat_instance_enabled: false
        max_subnet_count: 3
        vpc_flow_logs_enabled: false
```

<br/>

In the `stacks/ue2-dev.yaml` stack config file, add the following config for the derived VPC components in the `ue2-dev` stack:

```yaml title="stacks/ue2-dev.yaml"
# Import the base component configuration from the `catalog`.
# `import` supports POSIX-style Globs for file names/paths (double-star `**` is supported).
# File extensions are optional (if not specified, `.yaml` is used by default).
import:
  - catalog/vpc

components:
  terraform:
    vpc:
      metadata:
        # Point to the Terraform component in `components/terraform` folder
        component: infra/vpc
        inherits:
          # Inherit all settings and variables from the `vpc-defaults` base Atmos component
          - vpc-defaults
      vars:
        # Define variables that are specific for this component
        # and are not set in the base component
        name: vpc-1
        # Override the default variables from the base component
        vpc_flow_logs_enabled: true
        # Specify the name of the Atmos component that provides configuration
        # for the `infra/vpc-flow-logs-bucket` Terraform component
        vpc_flow_logs_bucket_component_name: vpc-flow-logs-bucket
```

<br/>

Having the stacks configured as shown above, we can now provision the `vpc` Atmos component into the `ue2-dev` Atmos stack by
executing the following Atmos commands:

```shell
atmos terraform plan vpc -s ue2-dev
atmos terraform apply vpc -s ue2-dev
```

## Summary

- Remote State for an Atmos component in an Atmos stack is obtained by using
  the [remote-state](https://github.com/cloudposse/terraform-yaml-stack-config/tree/main/modules/remote-state) Terraform module

- The module calls the [terraform-provider-utils](https://github.com/cloudposse/terraform-provider-utils) Terraform provider which processes the stack
  configs and returns the configuration for the Atmos component in the stack.
  The [terraform-provider-utils](https://github.com/cloudposse/terraform-provider-utils) Terraform provider utilizes Atmos `Go` modules to parse and
  process stack configurations

- The module accepts the `component` input as the Atmos component name for which to get the remote state outputs

- The module accepts the `context` input as a way to provide the information about the stack (using the context
  variables `namespace`, `tenant`, `environment`, `stage` defined in the stack config)

- If the Atmos component (for which we want to get the remote state outputs) is provisioned in a different Atmos stack (in different AWS account, or
  different AWS region), we can override the context variables `tenant`, `stage` and `environment` to point the module to the correct stack. For
  example, if the component is provisioned in a different AWS region (let's say `us-west-2`), we can set `environment = "uw2"`, and
  the [remote-state](https://github.com/cloudposse/terraform-yaml-stack-config/tree/main/modules/remote-state) module will get the remote state
  outputs for the Atmos component provisioned in that region

<br/>

:::caution


:::
