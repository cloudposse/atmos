---
title: Component-Oriented Programming
sidebar_position: 6
sidebar_label: Component-Oriented Programming
id: component-oriented-programming
---

[Component-Oriented Programming](https://en.wikipedia.org/wiki/Component-based_software_engineering) is a reuse-based approach to defining,
implementing and composing loosely-coupled independent components into systems.

Atmos supports the following concepts and principles of **Component-Oriented Programming (COP)**:

- [Single Inheritance](/core-concepts/components/inheritance#single-inheritance) - when an Atmos component inherits the configuration properties from
  another Atmos component

- [Multiple Inheritance](/core-concepts/components/inheritance#multiple-inheritance) - when an Atmos component inherits from more than one Atmos
  component

- Dynamic Polymorphism - ability to use and override base component(s) properties

- Encapsulation - enclose a set of related configuration properties into reusable loosely-coupled modules. Encapsulation is implemented
  by [Atmos Components](/core-concepts/components) which are opinionated building blocks of Infrastructure-as-Code (IAC) that solve one specific
  problem or use-case

- Abstraction, which is accomplished by these Atmos features:
  - Principle of Abstraction: in a given stack, "hide" all but the relevant information about a component configuration in order to reduce complexity
    and increase efficiency
  - Abstract Components: if a component is marked as `abstract`, it can be used only as a base for other components and can't be provisioned
    using `atmos` or CI/CD systems like [Spacelift](https://spacelift.io) or [Atlantis](https://www.runatlantis.io) (see
    our [integrations](/integrations) for details)

<br/>

:::info

These concepts and principles are implemented and used in Atmos by combining two features: [`import`](/core-concepts/stacks/imports)
and `metadata` component's configuration section.

:::

## References

- [Abstract Component Atmos Design Pattern](/design-patterns/abstract-component)
- [Component Inheritance Atmos Design Pattern](/design-patterns/component-inheritance)
