---
title: Glossary
sidebar_position: 1
sidebar_label: Glossary
---

Here's a list of the terms, concepts and conventions used throughout the Atmos project.

:::info
Atmos borrows from many of the concepts of [Object-Oriented Programming](https://en.wikipedia.org/wiki/Object-oriented_programming) and applies them
to configuration, enabling you to model configuration in a way that makes sense for your organization.
:::

| **Term**                                                                                              | **Definition**                                                                                                                                                                      |
|:------------------------------------------------------------------------------------------------------|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Abstract Component](/core-concepts/components)                                                       | A component baseline required to be inherited as a *Real Component* in order to be instantiated                                                                                     |
| [Catalog](/core-concepts/stacks)                                                                      | A collection of reusable configurations (e.g. Stack configurations)                                                                                                                 |
| [Component Instance](/core-concepts/components)                                                       | A configuration of a real component in a parent stack that can be instantiated (e.g. deployed)                                                                                      |
| [Components](/core-concepts/components)                                                               | Components are resuable building blocks of some tool. (e.g. terraform "root" modules.)                                                                                              |
| [Concrete Component](/core-concepts/components)                                                       | Also known as a "Real Component"                                                                                                                                                    |
| [Imports](/core-concepts/stacks/imports)                                                              | A mechanism to include one configuration in another for the purpose of reducing duplication.                                                                                        |
| [Inheritance](/core-concepts/components/inheritance)                                                  | A mechanism to derive a configuration from a hierarchy of other configurations that share a set of attributes.                                                                      |
| [Integration](/category/integrations)                                                                 | A mechanism of working with other tools and APIs                                                                                                                                    |
| [Library](/core-concepts/components/library)                                                          | A collection of "Components" that can be treated like reusable building blocks.                                                                                                     |
| [Mixins](/core-concepts/stacks/mixins)                                                                | A partial configuration that is imported for use by other stacks without having to be the parent stack.                                                                             |
| [Multiple Inheritance](/core-concepts/components)                                                     | A mechanism to inerit configurations from multiple sources                                                                                                                          |
| [Parent Stack](/core-concepts/components)                                                             | The Stack configuration which defines all components for an environment                                                                                                             |
| [Real Component](/core-concepts/components)                                                           | A component that is instantiated in a Parent Stack                                                                                                                                  |
| [Stack Manifest](/core-concepts/stacks)                                                               | Stack manifests are YAML files in which the configuration for all Atmos stacks and components are defined)                                                                          |
| [Stacks](/core-concepts/stacks)                                                                       | Atmos stacks are configurations that function like blueprints to describe all [components](/core-concepts/components)\nAn Atmos stack can be defined in one or more stack manifests |
| [Terraform "Child Modules"](https://developer.hashicorp.com/terraform/language/modules#child-modules) | Any terraform module that is called from another module. A child module does not have terraform state.                                                                              |
| [Terraform "Root Module"](https://developer.hashicorp.com/terraform/language/modules#child-modules)   | These are components. It's any top-level terraform module with a state backend. This is where "terraform" is run.                                                                   |
| [Vendoring](/core-concepts/components/vendoring)                                                      | A mechanism of making a copy of the 3rd party components in your repository.                                                                                                        |
| [Namespace](/core-concepts/stacks)                                                                    | A prefix for all resources in a Stack                                                                                                                                               |
| [Tenant](/core-concepts/stacks)                                                                       | A logical grouping of resources. In AWS we use the Tenant to represent the Organizational Unit (OU).                                                                                |
| [Environment](/core-concepts/stacks)                                                                  | A location where resources are deployed (e.g. `us-east-1`). See [Disambiguation](#disambiguation)                                                                                   |

# Disambiguation

Let's face it. These terms are frequently overloaded across tools and products.

| Term        | Context    | Atmos Analog                                                                                                                                                    |
|:------------|:-----------|:----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Stack       | Spacelift  | Component Instance                                                                                                                                              |
| Environment | Atmos      | Parent Stack                                                                                                                                                    |
| Environment | Atmos      | Frequently refers to a region (we want to move away from this, but it's an artifact of how we use `terraform-null-label`                                        |
| Stage       | Atmos      | E.g. Dev, Production, Staging, QA, etc. By convention, we recommend a minimum of one account per stage. Some companys use "Environment" the way we use "Stage". |
| Namespace   | Kubernetes | Frequently it will be represented by a variable named `kubernetes_namespace`; it should not be confused with what atmos calls `namespace`                       |
