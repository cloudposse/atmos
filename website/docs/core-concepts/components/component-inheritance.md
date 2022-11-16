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
    using `atmos` or CI/CD systems like [Spacelift](https://spacelift.io) or [Atlantis](https://www.runatlantis.io) (see our [integrations](/category/integrations) for details)

:::

<br/>

These concepts and principles are implemented and used in Atmos by combining two features: [`import`](/core-concepts/stacks/imports)
and `metadata` component's configuration section.

## Single Inheritance

Single Inheritance is used when an Atmos component inherits from another Atmos component.

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

- **Abstract components**: `atmos` component `vpc-defaults` is marked as abstract in `metadata.type`. This makes the component non-deployable, and it
  can be used only as a base for other components that inherit from it
- **Dynamic polymorphism**: All the variables in the `vars` section become the default values for the derived components. This provides the ability to
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
different settings, and provision them into the same (or different) environment (account/region) using the same Terraform code (which is
environment-agnostic). And the configurations are extremely DRY and reusable.

## Multiple Inheritance

Multiple Inheritance is used when an Atmos component inherits from more than one base components.

Multiple Inheritance allows a component to inherit from many base components or mixins, each base component having its own inheritance chain,
effectively making it an inheritance matrix. It uses a method similar to Method Resolution Order (MRO) using
the [C3 linearization](https://en.wikipedia.org/wiki/C3_linearization) algorithm, which is how Python supports multiple inheritance.

<br/>

:::info

In **Object-Oriented Programming (OOP)**, a mixin is a class that contains methods for use by other classes without having to be the parent class of
those other classes.

In **Component-Oriented Programming (COP)** implemented by Atmos, a mixin is an abstract base component that is never meant to be provisioned and does
not have any physical implementation - it just contains default settings/variables/properties for use by other Atmos components.

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

`atmos` will process all configurations for the current component and all the base components/mixins and will show the following output:

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
