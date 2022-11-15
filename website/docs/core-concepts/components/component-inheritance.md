---
title: Component Inheritance
sidebar_position: 7
sidebar_label: Inheritance
---

Component inheritance is the ability to combine multiple configurations through ordered deep-merging of configurations. The concept is borrowed from
[Object-Oriented Programming](https://en.wikipedia.org/wiki/Inheritance_(object-oriented_programming)) to logically organize complex configurations in
a way that makes conceptual sense. The side effect of this are extremely DRY and reusable configurations.

:::info

In Object-Oriented Programming (OOP), inheritance is the mechanism of basing an object or class upon another object (prototype-based inheritance) or
class (class-based inheritance), retaining similar implementation.

Similarly, in Atmos, Component inheritance is the mechanism of deriving a component from one or more base components, inheriting all the
properties of the base component(s) and overriding only some fields specific to the derived component. The derived component acquires all the
properties of the "parent" component(s), allowing creating very DRY configurations that are built upon existing components.

In Atmos, we call it **Component-Oriented Programming (COP)**.

:::

:::info

**Component-Oriented Programming** supports:

- Single inheritance
- Multiple inheritance
- Dynamic polymorphism (ability to override base component properties)
- Abstraction, which is accomplished by these Atmos features:
  - Principle of abstraction: in a given stack, "hide" all but the relevant information about a component configuration in order to reduce complexity
    and increase efficiency
  - Abstract components: if a component is marked as `abstract`, it can be used only as a base for other components and can't be provisioned
    using `atmos` or CI/CD systems like [Spacelift](https://spacelift.io) or [Atlantis](https://www.runatlantis.io)

:::

<br/>

These concepts and principles are implemented and used in Atmos by combining two features: [`import`](/core-concepts/stacks/imports)
and `metadata` component's configuration section.

## Single Inheritance

Single Inheritance is used when an Atmos component inherits from another Atmos component.

Let's say we want to provision two VPCs with different settings into the same AWS account.

In `stacks/catalog/vpc.yaml`, add the following config for the VPC component:

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

- **Abstract components**: `atmos` component `vpc-defaults` is marked as abstract in `metadata.type`. This makes the component non-deployable, and it
  can be used only as a base for other components that inherit from it
- **Dynamic polymorphism**: All the variables in the `vars` section become the default values for the derived components. This provides the ability to
  override and use the base component properties in the derived components to provision the same Terraform configuration many times but with different
  settings

<br/>

In the `stacks/ue2-dev.yaml` stack config file, add the following config for the derived VPC components in the `ue2-dev` stack:

```yaml title="stacks/ue2-dev.yaml"
# Import the base component configuration from the `catalog`
# `import` supports POSIX-style Globs for file names/paths (double-star `**` is supported)
# File extensions are optional (if not specified, `.yaml` is used by default)
import:
  - catalog/vpc

components:
  terraform:

    vpc-1:
      metadata:
        component: ingra/vpc # Point to the Terraform component in `components/terraform` folder
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
        component: ingra/vpc # Point to the same Terraform component in `components/terraform` folder
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

- **Component inheritance**: In the `ue2-dev` stack (`stacks/ue2-dev.yaml` stack config file), the Atmos components `vpc-1` and `vpc-2` inherit from
  the base component `vpc-defaults`. This makes `vpc-1` and `vpc-2` derived components
- **Principle of abstraction**: In the `ue2-dev` stack, only the relevant information about the derived components in the stack is shown. All the base
  component settings are "hidden" (in the imported `catalog`), which reduces the configuration size and complexity
- **Dynamic polymorphism**: The derived `vpc-1` and `vpc-2` components override and use the base component properties to be able to provision the same
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
different settings, and provision them into the same (or different) environment (account/region) using the same Terraform code.
And the configurations are extremely DRY and reusable.

## Multiple Inheritance

