---
title: Component Inheritance
sidebar_position: 7
sidebar_label: Inheritance
---

Component Inheritance is the ability to combine multiple configurations through ordered deep-merging of configurations. The concept is borrowed from
[Object-Oriented Programming](https://en.wikipedia.org/wiki/Inheritance_(object-oriented_programming)) to logically organize complex configurations in
a way that makes conceptual sense. The side effect of this are extremely DRY configurations.

:::info
In Object-Oriented Programming, inheritance is the mechanism of basing an object or class upon another object (prototype-based inheritance) or class (
class-based inheritance), retaining similar implementation.

Similarly, in Atmos, Component inheritance is the mechanism of deriving a component configuration from one or many base components, inheriting all the
properties of the base component(s) and overriding those fields specific to the derived component. The derived component acquires all the properties
of the "parent" component(s), allowing creating component configurations that are built upon existing components.
:::

Inheritance is accomplished by combining two features of atmos Stacks: [`import`](/core-concepts/stacks/imports) and `inherits`.

## Example


