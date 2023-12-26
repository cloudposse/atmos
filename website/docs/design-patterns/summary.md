---
title: Summary
sidebar_position: 100
sidebar_label: Summary
description: Summary
---

Architecting and provisioning enterprise-grade infrastructure is challenging, and making all the components and
configurations [reusable](https://en.wikipedia.org/wiki/Reusability) and [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself) is even more
difficult.

Atmos Design Patterns can help with structuring and organizing your design to provision multi-account enterprise-grade environments for complex
organizations.

We've already identified and documented many Atmos Design Patterns.
You might be asking yourself: "How do I start with all of that and what steps to take?"

Here are some recommendations:

- This [Quick Start](https://github.com/cloudposse/atmos/tree/master/examples/quick-start) repository presents an example of an infrastructure managed
  by Atmos. You can clone it and configure to your own needs. The repository should be a good start to get yourself familiar with Atmos and the
  Design Patterns. The [Quick Start Guide](/category/quick-start) describes the step required to configure and start using the repository

- If you are just developing or testing your infrastructure, you can use any of the Design Patterns described in this guide, and find out which ones
  are best suited to your requirements

- If you are architecting the infrastructure to provision multi-account multi-region environments at scale, we suggest you start with the
  following Design Patterns:

  - [Organizational Structure Configuration](/design-patterns/organizational-structure-configuration)
  - [Component Catalog](/design-patterns/component-catalog)
  - [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
  - [Inline Component Customization](/design-patterns/inline-component-customization)
  - [Component Inheritance](/design-patterns/component-inheritance)
  - [Abstract Component](/design-patterns/abstract-component)
  - [Multiple Component Instances](/design-patterns/multiple-component-instances)
