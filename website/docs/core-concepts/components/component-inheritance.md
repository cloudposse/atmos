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

- single inheritance
- multiple inheritance
- dynamic polymorphism (ability to override base component properties)
- abstraction, which is accomplished by these Atmos features:
  - principle of abstraction: in a given stack, "hide" all but the relevant information about a component configuration in order to reduce complexity
    and increase efficiency
  - abstract components: if a component is marked as `abstract`, it can be used only as a base for other components and can't be provisioned
    using `atmos` or CI/CD systems like [Spacelift](https://spacelift.io) or [Atlantis](https://www.runatlantis.io)

:::

<br/>

These concepts and principles are implemented and used in Atmos by combining two features: [`import`](/core-concepts/stacks/imports)
and `metadata` component's configuration section.

## Single Inheritance

Let's say we want to provision two different VPCs into an AWS account.

In `stacks/catalog/vpc.yaml` add the following config for the VPC component:

```yaml title="stacks/catalog/vpc.yaml"
components:
  terraform:
    vpc-defaults:
      metadata:
        # Setting `metadata.type: abstract` makes the component `abstract` 
        # (similar to OOP abstract classes), explicitly prohibiting the component from being deployed.
        # `terraform apply` and `terraform deploy` will fail with an error.
        # If `metadata.type` attribute is not specified, it defaults to `real`.
        # `real` components can be provisioned by `atmos` and CI/CD like Spacelift and Atlantis).
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

In the configuration above, the following concepts are implemented:

- **Abstract components**: Terraform component `vpc-defaults` is marked as abstract in `metadata.type`. This makes the component non-deployable, and it
  can be used only as a base for other components that inherit from it
- **Dynamic polymorphism**: All the variables in the `vars` section become the default values for the derived components. This provides the ability to
  override and use the base component properties in the derived components to provision the same Terraform configuration but with different settings

## Multiple Inheritance

