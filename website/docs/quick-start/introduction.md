---
title: Quick Start Introduction
sidebar_position: 1
sidebar_label: Introduction
---

# Introduction

Atmos is a CLI and a powerful enterprise-grade workflow automation tool for DevOps.

It's also a framework that prescribes patterns and best practices to structure and organize components and stacks to design for organizational
complexity and provision multi-account environments for complex organizations.

It allows you to very quickly configure and provision infrastructure into many environments (e.g. AWS accounts and regions), and make those
configurations extremely [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself).

One of the main principles of Atmos is the separation of configuration from code, so the code is generic and can be deployed anywhere (in any
Organization, Organizational Unit, region, account). This design principle is
called [separation of concerns](https://en.wikipedia.org/wiki/Separation_of_concerns).

Atmos separates the components (logic) and stacks (configuration of the components for diff environments) so they can be independently managed and
evolved. In the case of using Terraform, the components
are [Terraform root modules](https://developer.hashicorp.com/terraform/language/modules#the-root-module).

In many cases, with enterprise-grade infrastructures (multi-org, multi-tenant, multi-account, multi-region, multi-team), the configuration is much
more complicated than the code. That's what Atmos is trying to solve - to make the configuration manageable, reusable (by
using [Imports](/core-concepts/stacks/imports), [Inheritance](/core-concepts/components/inheritance), and other
Atmos features) and [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself), and to make the code completely generic.

In this Quick Start guide, we describe how to provision infrastructure managed by Terraform into different AWS environments.
The configurations for the environments are managed by Atmos.

<br/>

:::tip

This Quick Start guide describes the steps to configure and provision the infrastructure
from this [Quick Start](https://github.com/cloudposse/atmos/tree/master/examples/quick-start) repository.

You can clone it and configure to your own needs. The repository should be a good start to get yourself familiar with Atmos.

:::

<br/>

The steps to configure and provision the infrastructure are as follows:

- [Install Atmos](/quick-start/install-atmos)
- [Configure Repository](/quick-start/configure-repository)
- [Configure CLI](/quick-start/configure-cli)
- [Vendor Components](/quick-start/vendor-components)
- [Create Atmos Stacks](/quick-start/create-atmos-stacks)
- [Configure Validation](/quick-start/configure-validation)
- [Create Workflows](/quick-start/create-workflows)
- [Add Custom Commands](/quick-start/add-custom-commands)
- [Configure Terraform Backend](/quick-start/configure-terraform-backend)
- [Provision](/quick-start/provision)
- [Final Notes](/quick-start/final-notes)
- [Next Steps](/quick-start/next-steps)
