---
title: Atmos Components
sidebar_position: 1
sidebar_label: Components
description: Components are opinionated building blocks of infrastructure as code that solve one specific problem or use-case.
---

Components are opinionated, self-contained building blocks of infrastructure as code that solve one, specific problem or use-case. Atmos was written to support any number of tools, but also supports a couple of native integations with tools like `terraform` and `helmfile`. A common use-case for atmos is implementing workflows for `terraform` "root modules".

## Component Schema

A Component consists of the infrastructure as code business logic (e.g. a terraform "root" module) as well as the configuration of that component. The configuration of a component is stored in a Stack configuration.

The schema of a Component in a Stack configuration is as follows:

```yaml
components:
  terraform:
    # the slug of the component
    example:

      # configuration specific to atmos
      metadata:
        # Components can be of type "real" (default) or "abstract"
        type: real
        # This is the directory path of the component. 
        # In this example, we're referencing a component in the `componentns/terraform/stable/example` folder.
        component: stable/example

        # We can leverage multiple inheritance to sequentially deep merge multiple configurations
        inherits:
          - example-defaults

        # Settings are where we store configuration related to integrations.
        # It's a freeform map; anything can be placed here.
        settings:
          spacelift: {}

      # Define the terraform variables, which will get deep-merged and exported to a `.tfvars` file by atmos.
      vars:
        enabled: true
        name: superduper
        nodes: 10
```


### Component Attributes

#### vars

The `vars` section is a free-form map. Use [component validation](/core-concepts/components/component-validation) to enforce policies.

#### vars.namespace

This is an *optional* [`terraform-null-label`](https://github.com/cloudposse/terraform-null-label) convention. 

The namespace of all stacks. Typically, there will be one namespace for the organization.

Example:

```yaml
vars:
  namespace: acme
```

#### vars.tenant

This is an *optional* [`terraform-null-label`](https://github.com/cloudposse/terraform-null-label) convention. 

In a multi-tenant configuration, the tenant represents a single `tenant`. By convention, we typically
recommend that every tenant have it's own Organizational Unit (OU).

Example:

```yaml
vars:
  tenant: platform
```


#### vars.stage

This is an *optional* [`terraform-null-label`](https://github.com/cloudposse/terraform-null-label) convention. 

The `stage` is where workloads run. See our [glossary](/reference/glossary) for disamgiguation.

Example:
```yaml
vars:
  # Production stage
  stage: prod
```

#### vars.environment

This is an *optional* [`terraform-null-label`](https://github.com/cloudposse/terraform-null-label) convention. 

The `environment` is used for location where things run. See our [glossary](/reference/glossary) for disamgiguation.

Example:
```yaml

vars:
  # us-east-1
  environment: ue1
```

#### metadata

The `metadata` section extends functionality of the component.

#### settings

The `settings` block is a free-form map used to pass configuration information to [integrations](/category/integrations).

## Types of Components

The type of a component is expressed in the `metadata.type` parameter of a given component configuration.

There are two types of components:

- `real` - is a ["concrete"](https://en.wikipedia.org/wiki/Concrete_class) component instance
- `abstract` - a component configuration, which cannot be instantiated directly. The concept is borrowed
  from ["abstract base classes"](https://en.wikipedia.org/wiki/Abstract_type) of Object Oriented Programming.

## Flavors of Components

Atmos natively supports two types of components, but the convention can be extended to anything (e.g. `docker`, `packer`, `ansible`, etc.)

1. **Terraform:**These are stand-alone "root modules" that implement some piece of your infrastructure. For example, typical components might be an
   EKS cluster, RDS cluster, EFS filesystem, S3 bucket, DynamoDB table, etc. You can find
   the [full library of SweetOps Terraform components on GitHub](https://github.com/cloudposse/terraform-aws-components "https://github.com/cloudposse/terraform-aws-components")
   . By convention, we store components in the `components/terraform/` directory within the infrastructure repository.

2. **Helmfiles**: These are stand-alone applications deployed using[`helmfile`](https://github.com/helmfile)to Kubernetes. For example, typical
   helmfiles might deploy the DataDog agent, `cert-manager` controller, `nginx-ingress` controller, etc. Similarly,
   the [full library of SweetOps Helmfile components is on GitHub](https://github.com/cloudposse/helmfiles "https://github.com/cloudposse/helmfiles").
   By convention, we store these types of components in the `components/helmfile/` directory within the infrastructure repository. Please note, use
   these public helmfiles as examples; they may not be current.

## Terraform Components

One important distinction about components that is worth noting: components should be opinionated terraform "root" modules that typically call other child modules. Components are the building blocks of your infrastructure. This is where you define all the business logic for how to provision some common piece of infrastructure like ECR repos (with the [ecr](https://github.com/cloudposse/terraform-aws-components/tree/master/modules/ecr) component) or EKS clusters (with the [eks](https://github.com/cloudposse/terraform-aws-components/tree/master/modules/eks/cluster) component). Our convention is to stick components in the`components/terraform/`directory.

If your components rely on submodules, our convention is to use a `modules/` subfolder of the component to store them.

We do not recommend consuming one terraform component inside of another as that would defeat the purpose; each component is intended to be a loosely coupled unit of IaC with its own lifecycle. Further more, since components define a state backend and providers, it's not advisable to call one root module from another root module.
