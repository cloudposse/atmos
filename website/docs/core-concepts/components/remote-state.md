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

## Configure and Provision `vpc-flow-logs-bucket` Component

## Configure Remote State for `vpc-flow-logs-bucket` Component

## Configure and Provision `vpc` Component

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

Single Inheritance is used when an Atmos component inherits from another base Atmos component.

Let's say we want to provision two VPCs with different settings into the same AWS account.

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

In the configuration above, the following **Component-Oriented Programming** concepts are implemented:

- **Abstract Components**: `atmos` component `vpc-defaults` is marked as abstract in `metadata.type`. This makes the component non-deployable, and it
  can be used only as a base for other components that inherit from it
- **Dynamic Polymorphism**: All the variables in the `vars` section become the default values for the derived components. This provides the ability to
  override and use the base component properties in the derived components to provision the same Terraform configuration many times but with different
  settings

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

In the configuration above, the following **Component-Oriented Programming** concepts are implemented:

- **Component Inheritance**: In the `ue2-dev` stack (`stacks/ue2-dev.yaml` stack config file), the Atmos components `vpc-1` and `vpc-2` inherit from
  the base component `vpc-defaults`. This makes `vpc-1` and `vpc-2` derived components
- **Principle of Abstraction**: In the `ue2-dev` stack, only the relevant information about the derived components in the stack is shown. All the base
  component settings are "hidden" (in the imported `catalog`), which reduces the configuration size and complexity
- **Dynamic Polymorphism**: The derived `vpc-1` and `vpc-2` components override and use the base component properties to be able to provision the same
  Terraform configuration many times but with different settings

<br/>

Having the components in the stack configured as shown above, we can now provision the `vpc-1` and `vpc-2` components into the `ue2-dev` stack by
executing the following `atmos` commands:

```shell
atmos terraform apply vpc-1 -s ue2-dev
atmos terraform apply vpc-2 -s ue2-dev
```

<br/>

As we can see, using the principles of **Component-Oriented Programming (COP)**, we are able to define two (or more) components with
different settings, and provision them into the same (or different) environment (account/region) using the same Terraform code (which is
environment-agnostic). And the configurations are extremely DRY and reusable.

## Multiple Inheritance

Multiple Inheritance is used when an Atmos component inherits from more than one Atmos component.

Multiple Inheritance allows a component to inherit from many base components or mixins, each base component having its own inheritance chain,
effectively making it an inheritance matrix. It uses a method similar to Method Resolution Order (MRO) using
the [C3 linearization](https://en.wikipedia.org/wiki/C3_linearization) algorithm, which is how Python supports multiple inheritance.

<br/>

:::info

In **Object-Oriented Programming (OOP)**, a mixin is a class that contains methods for use by other classes without having to be the parent class of
those other classes.

In **Component-Oriented Programming (COP)** implemented in Atmos, a [mixin](/core-concepts/stacks/mixins) is an abstract base component that is never
meant to be provisioned and does not have any physical implementation - it just contains default settings/variables/properties for use by other Atmos
components.

:::

<br/>

Multiple Inheritance, similarly to Single Inheritance, is defined by the `metadata.inherits` section in the component
configuration. `metadata.inherits` is a list of component or mixins names from which the current component inherits.
In the case of multiple base components, it is processed in the order by which it was declared.

For example, in the following configuration:

```yaml
metadata:
  inherits:
    - componentA
    - componentB
```

Atmos will recursively deep-merge all the base components of `componentA` (each component overriding its base),
then all the base components of `componentB` (each component overriding its base), then the two results are deep-merged together with `componentB`
inheritance chain overriding the values from `componentA` inheritance chain.

<br/>

:::caution
All the base components/mixins referenced by `metadata.inherits` must be already defined in the Stack configuration, either by using an `import`
statement or by explicitly defining them in the Stack configuration. The `metadata.inhertis` statement does not imply that we are importing anything.
:::

<br/>

Here is a concrete example:

<br/>

```yaml
# Import all the base components and mixins we want to inherit from.
# `import` supports POSIX-style Globs for file names/paths (double-star `**` is supported).
import:
  - catalog/terraform/test/test-component-override
  - catalog/terraform/test/test-component-override-2
  - catalog/terraform/mixins/test-*.*

components:
  terraform:
    test/test-component-override-3:
      vars: { }
      metadata:
        # `real` is implicit, you don't need to specify it.
        # `abstract` makes the component protected from being deployed.
        type: real
        # Terraform component. Must exist in `components/terraform` folder.
        # If not specified, it's assumed that this component `test/test-component-override-3` 
        # is also a Terraform component in 
        # `components/terraform/test/test-component-override-3` folder.
        component: "test/test-component"
        # Multiple inheritance.
        # It's a down-top/left-right matrix similar to Method Resolution Order (MRO) in Python.
        inherits:
          - "test/test-component-override"
          - "test/test-component-override-2"
          - "mixin/test-1"
          - "mixin/test-2"
```

<br/>

In the configuration above, all the base components and mixins are processed and deep-merged in the order they are specified in the `inherits` list:

- `test/test-component-override-2` overrides `test/test-component-override` and its base components (all the way up its inheritance chain)

- `mixin/test-1` overrides `test/test-component-override-2` and its base components (all the way up its inheritance chain)

- `mixin/test-2` overrides `mixin/test-1` and its base components (all the way up its inheritance chain)

- The current component `test/test-component-override-3` overrides `mixin/test-2` and its base components (all the way up its inheritance chain)

When we run the following command to provision the `test/test-component-override-3` Atmos component into the stack `tenant1-ue2-dev`:

```shell
atmos terraform apply test/test-component-override-3 -s tenant1-ue2-dev
```

`atmos` will process all configurations for the current component and all the base components/mixins and will show the following console output:

```text
Command info:
Atmos component: test/test-component-override-3
Terraform component: test/test-component
Terraform command: apply
Stack: tenant1-ue2-dev
Inheritance: test/test-component-override-3 -> mixin/test-2 -> mixin/test-1 -> 
             test/test-component-override-2 -> test/test-component-override -> test/test-component
```

<br/>

The `Inheritance` output shows the multiple inheritance steps that Atmos performed and deep-merged into the final configuration, including
the variables which are sent to the Terraform component `test/test-component` that is being provisioned.
