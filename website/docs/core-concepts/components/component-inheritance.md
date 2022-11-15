---
title: Component Inheritance
sidebar_position: 7
sidebar_label: Inheritance
---

Component Inheritance is the ability to combine multiple configurations through ordered deep-merging of configurations. The concept is borrowed from
[Object-Oriented Programming](https://en.wikipedia.org/wiki/Inheritance_(object-oriented_programming)) to logically organize complex configurations in
a way that makes conceptual sense. The side effect of this are extremely DRY configurations.

Inheritance is accomplished by combining two features of atmos Stacks: [`import`](/core-concepts/stacks/imports) and `inherits`.

## Example


