---
title: Stack Imports
sidebar_position: 7
sidebar_label: Imports
---

:::note
TODO
:::

Imports are how we reduce duplication of configurations by creating reusable baselines. The imports should be thought of almost like blueprints. Once a reusable catalog of Stacks is exists, robust architectures can be easily created simply by importing those blueprints.

Imports may be used in Stack configuratinos together with [inheritance](/core-concepts/components/component-inheritance) and [mixins](/core-concepts/stacks/mixins) to produce an exceptionally DRY configuration in a way that is logically organized and easier to maintain for your team.

:::info
The mechanics of mixins and inheritance apply only to the [Stack](/core-concepts/stacks) configurations. Atmos knows nothing about the underlying components (e.g. terraform), and does not magically implement inheritance for HCL. However, by designing highly reusable components that do one thing well, we're able to achieve many of the same benefits.
:::

## Conventions

By convention, we recommend placing all "imports" in the `stacks/catalog` folder. 
