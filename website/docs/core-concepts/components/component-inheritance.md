---
title: Component Inheritance
sidebar_position: 7
sidebar_label: Inheritance
---

Component Inheritance is the ability to combine multiple configurations through ordered deep-merging of configurations. The concept is borrowed from
[Object-Oriented Programming](https://en.wikipedia.org/wiki/Inheritance_(object-oriented_programming)) to logically organize complex configurations in
a way that makes conceptual sense. The side effect of this are extremely DRY and reusable configurations.

:::info
In Object-Oriented Programming (OOP), inheritance is the mechanism of basing an object or class upon another object (prototype-based inheritance) or
class (class-based inheritance), retaining similar implementation.

Similarly, in Atmos, Component inheritance is the mechanism of deriving a component from one or more base components, inheriting all the
properties of the base component(s) and overriding only some fields specific to the derived component. The derived component acquires all the
properties of the "parent" component(s), allowing creating very DRY configurations that are built upon existing components.

In Atmos, we call it **Component-Oriented Programming (COP)**.

COP supports inheritance, multiple inheritance, dynamic polymorphism (ability to override base component properties), and abstraction ("show" only
essential component's attributes in a given stack and "hide" unnecessary information derived from the base components).
:::

<br/>

With all the definitions out of the way, here's how all these concepts are implemented and used in Atmos.

Inheritance is accomplished by combining two features of Atmos: [`import`](/core-concepts/stacks/imports) and `metadata.inherits` component
section.

## Example


