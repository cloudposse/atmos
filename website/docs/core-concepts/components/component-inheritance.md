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
- abstraction, which itself is accomplished by these Atmos features:
  - principle of abstraction: "hide" all but the relevant information about a component configuration in a given stack in order to reduce complexity
    and increase efficiency
  - abstract components: if a component is marked as an abstract, it can be used only as a base for other components and can't be provisioned
    using `atmos` or CI/CD systems like Spacelift, Atlantis, or GitHub Actions

:::

<br/>

With all the definitions out of the way, here's how all these concepts and principles are implemented and used in Atmos.

Inheritance (and multiple inheritance) is accomplished by combining two features of Atmos: [`import`](/core-concepts/stacks/imports)
and `metadata.inherits` component section.

## Example


