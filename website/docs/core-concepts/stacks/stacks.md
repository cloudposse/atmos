---
title: Atmos Stacks
sidebar_position: 1
sidebar_label: Stacks
description: Stacks are a way to express the complete infrastructure needed for an environment
id: stacks
---

Stacks are a way to express the complete infrastructure needed for an environment. Think of a Stack as an architectural "Blueprint" composed
of one or more [Components](/core-concepts/components) and defined using a
[standardized YAML configuration](#schema).

Stacks are an abstraction layer that is used to instantiate Components (e.g. to deploy Terraform "root" modules). Stacks are defined
in YAML and managed in one more files that follow a standard schema to enable a fully declarative description of your various environments. This empowers you to separate your infrastructure’s environment configuration settings from the code itself (e.g. Terraform components).

Atmos utilizes a custom YAML configuration format for stacks. YAML is an ideal choice because it’s an easy-to-work-with format that is nicely portable across multiple tools and every developer understands it. The stack YAML format is natively supported by Atmos,
the [terraform-utils-provider](https://github.com/cloudposse/terraform-provider-utils) provider, and Spacelift via the
[terraform-spacelift-cloud-infrastructure-automation](https://github.com/cloudposse/terraform-spacelift-cloud-infrastructure-automation) module. This means that from Terraform, you can natively access the entire infrastructure configuration.

## Use-cases


## Conventions

The differentiation between the following two types of stacks is crucial for understanding how stacks are organized are are the basis for the
various [design patterns](/design-patterns/).

### Parent Stacks

*Parent Stacks* are the top-level stacks that are responsible for importing Child stacks. Components inside of Parent stacks are deployable, unlike in Child stacks.

### Child Stacks

*Child Stacks* are any stacks whose components cannot be deployed independently without being imported by a Parent Stack.
[Catalogs](/core-concepts/stacks/catalogs) are typically where we keep our Child Stacks.

## Schema

A Stack file contains a manifest defined in YAML that follows a simple, extensible schema. In fact, every Stack file follows exactly the same schema, and every setting in the configuration is optional. Enforcing a consistent schema ensures we can easily [import and deep-merge](/core-concepts/stacks/imports) configurations and use [inheritance](/core-concepts/components/inheritance) to achieve DRY configuration.

```yaml
# Configurations that should get deep-merged into this one
import:
  # each import is a "Stack" file. The `.yaml` extension is optional, and we do not recommend using it.
  - ue2-globals

# Top-level variables that are inherited by every single component. 
# Use these wisely. Too many global vars will pollute the variable namespace.
vars:
  # Variables can be anything you want. They can be scalars, lists, and maps. Whatever is supported by YAML.
  stage: dev

# There can then be global variables for each type of component. 
# Here we set global variables for any "terraform" component.
terraform:
  vars: { }

# Here we set global variables for any "helmfile" component.
helmfile:
  vars: { }

# Components are the building blocks of reusable infrastructure.
# They can be anything. Atmos natively supports "terraform" and "helmfile".
components:
  terraform:
    vpc:
      command: "/usr/bin/terraform-0.15"
      backend:
        s3:
          workspace_key_prefix: "vpc"
      vars:
        cidr_block: "10.102.0.0/18"
    eks:
      backend:
        s3:
          workspace_key_prefix: "eks"
      vars: { }

  helmfile:
    nginx-ingress:
      vars:
        installed: true
```

### Stack Attributes

#### components

The `components` is the list of all the building blocks.

Example:

```yaml
components:
  sometool: # "sometool" can be any tool
    somecomponent: # "somecomponent" can be the name of any "sometool" component
      vars: # etc...
```

So for `terraform`, it might look something like this:

```yaml
components:
  terraform:
    vpc:
      vars: # etc...
```

## Stack Files

Stack files can be very numerous in large cloud environments (think many dozens to hundreds of stack files). To enable the proper organization of
stack files, SweetOps has established some conventions that are good to follow. However, these are just conventions, and there are no limits enforced
by the tool.

By convention, we recommend to store all Stacks in a `stacks/` folder at the root of your infrastructure repository. This way it's clear where they
live and helps keep the configuration separate from your code (e.g. HCL).

The filename of individual environment stacks can follow any convention, and the best one will depend on how you model environments at your
organization.

### Basic Layout

A basic form of organization is to follow the pattern of naming where each `$environment-$stage.yaml` is a file. This works well until you have so
many environments and stages.

For example, `$environment` might be `ue2` (for `us-east-2`) and `$stage` might be `prod` which would result in `stacks/ue2-prod.yaml`

Some resources, however, are global in scope. For example, Route53 and IAM might not make sense to tie to a region. These are what we call "global
resources". You might want to put these into a file like `stacks/global-region.yaml` to connote that they are not tied to any particular region.

### Hierarchical Layout

We recommend using a hierarchical layout that follows the way AWS thinks about infrastructure. This works very well when you may have dozens or
hundreds of accounts and regions that you operate in. Use [Catalogs](/core-concepts/stacks/catalogs) to organize your Stack configurations.
