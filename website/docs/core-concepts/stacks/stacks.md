---
title: Atmos Stacks
sidebar_position: 1
sidebar_label: Stacks
---

Stacks are a way to express the complete infrastructure needed for an that is environment. Think of a Stack as an architectural "Blueprint" composed of one or more [Components](/core-concepts/components) and defined using a
[standardized YAML configuration](#schema).

Stacks are an abstraction layer that is used to instantiate Components. They’re a set of YAML files that follow a standard schema to
enable a fully declarative description of your various environments. This empowers you with the ability to separate your infrastructure’s environment
configuration settings from the business logic behind it (provided via components).

Atmos utilizes a custom YAML configuration format for stacks because it’s an easy-to-work-with format that is nicely portable across multiple
tools. The stack YAML format is natively supported today via Atmos, the [terraform-yaml-stack-config](https://github.com/cloudposse/terraform-yaml-stack-config) module, and Spacelift via the
[terraform-spacelift-cloud-infrastructure-automation](https://github.com/cloudposse/terraform-spacelift-cloud-infrastructure-automation) module.

## Supported 
## Schema

A Stack file is defined in YAML and follows a simple, extensible schema. Every Stack file follows the same schema; however, every setting in
the configuration is optional. Enforcing a consistent schema ensures we can easily [import and deep-merge](/core-concepts/stacks/imports) configurations and implement [inheritance](/core-concepts/components/component-inheritance).

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

We recommend using a hierarchical layout that follows the way AWS thinks about infrastructure. This works very well when you may have dozens-hundreds
of accounts and regions that you operate in.

AWS organizes infrastructure like this:

1. The top-level account is the "Organization"
2. An "Organization" can have any number of "Organizational Units" (OUs)
3. Each "OU" can have "Member Accounts"
4. Each "Member Account" has "Regions"
5. Each "Region" has "Resources" (the top-level stack)

In sticking with this theme, a good filesystem layout for infrastructure looks like this:

```text
└── stacks/
    └── orgs/
        └── acme/
            ├── ou1/
            │   ├── account1/
            │   │   ├── global-region.yaml
            │   │   └── us-east-2.yaml
            │   ├── account2/
            │   │   ├── global-region.yaml
            │   │   └── us-east-2.yaml
            │   └── account3/
            │       ├── global-region.yaml
            │       └── us-east-2.yaml
            └── ou2/
                ├── dev/
                │   ├── global-region.yaml
                │   └── us-east-2.yaml
                ├── prod/
                │   ├── global-region.yaml
                │   └── us-east-2.yaml
                └── staging/
                    ├── global-region.yaml
                    └── us-east-2.yaml
```
