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

- [terraform-provider-utils](https://github.com/cloudposse/terraform-provider-utils) - The Cloud Posse Terraform Provider for various utilities (e.g.
  deep merging, stack configuration management)

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

## Configure and Provision `vpc-flow-logs-bucket` Component

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
# Import the base component configuration from the `catalog`.
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
          - # Inherit all settings and variables from the `vpc-flow-logs-bucket-defaults` base Atmos component
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

## Configure Remote State for `vpc-flow-logs-bucket` Component

## Configure and Provision `vpc` Component

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
        vpc_flow_logs_enabled: true
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

    vpc-1:
      metadata:
        component: infra/vpc # Point to the Terraform component in `components/terraform` folder
        inherits:
          - vpc-defaults # Inherit all settings and variables from the `vpc-defaults` base component
      vars:
        # Define variables that are specific for this component
        # and are not set in the base component
        name: vpc-1
        # Override the default variables from the base component
        public_subnets_enabled: true
        nat_gateway_enabled: true
        vpc_flow_logs_enabled: false

    vpc-2:
      metadata:
        component: infra/vpc # Point to the same Terraform component in `components/terraform` folder
        inherits:
          - vpc-defaults # Inherit all settings and variables from the `vpc-defaults` base component
      vars:
        # Define variables that are specific for this component
        # and are not set in the base component
        name: vpc-2
        # Override the default variables from the base component
        max_subnet_count: 2
        vpc_flow_logs_enabled: false
```

<br/>

Having the components in the stack configured as shown above, we can now provision the `vpc-1` and `vpc-2` components into the `ue2-dev` stack by
executing the following `atmos` commands:

```shell
atmos terraform apply vpc-1 -s ue2-dev
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
  variables `namespace`, `tenant`, `environment`, `stage`)

- If the Atmos component (for which we want to get the remote state outputs) is provisioned in a different Atmos stack (in different AWS account, or
  different AWS region), we can override the context
  variables `tenant`, `stage` and `environment` to point the module to the correct stack. For example, if the component is provisioned in a
  different AWS region (let's say `us-west-2`), we can set `environment = "uw2"`, and
  the [remote-state](https://github.com/cloudposse/terraform-yaml-stack-config/tree/main/modules/remote-state) module will get the remote state
  outputs for the Atmos component provisioned in that region
