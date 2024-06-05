---
title: Atmos Components
sidebar_position: 1
sidebar_label: Components
description: Components are opinionated building blocks of infrastructure as code that solve one specific problem or use-case.
---

Components are opinionated, self-contained building blocks of Infrastructure-as-Code (IAC) that solve one specific problem or use-case. 
Those Components are then configured inside of one or more [Stacks](/core-concepts/stacks).

Atmos was designed to be tool-agnostic, but also supports several native integrations with tools like [`terraform`](/cli/commands/terraform/usage) and [`helmfile`](/cli/commands/helmfile/usage). 
A common use-case for Atmos is implementing components for [Terraform "root modules"](https://developer.hashicorp.com/terraform/language/modules#the-root-module).

:::tip
Components are things like [Terraform "root" modules](https://developer.hashicorp.com/terraform/language/modules#the-root-module), Helm Charts, Dockerfiles, or any fundamental building block of infrastructure.
:::

## Use-cases

Components offer a multitude of applications across various business scenarios. Cloud Posse publishes its AWS components for free, so you can see
some [technical use-cases for Terraform components](https://docs.cloudposse.com/components/category/aws/).

- **Accelerate Development Cycles:** By reusing components, development teams can significantly shorten the time from concept to deployment, facilitating faster product iterations and quicker responses to market changes.

- **Security policies and compliance controls** DevOps and SecOps teams implement components to uniformly apply security policies and compliance controls across all cloud environments, ensuring regulatory adherence.

- **Enhance Collaboration Across Teams:** Components foster a shared understanding and approach to infrastructure, promoting collaboration between development, operations, and security teams, leading to more cohesive and secure product development.

## Best Practices

Here are some essential best practices to follow when designing architectures using infrastructure as code (IaC), focusing on optimizing
component design, reusability, and lifecycle management. These guidelines are designed to help developers and operators build efficient,
scalable, and reliable systems, ensuring a smooth and effective infrastructure management process.

- **Keep Your Components Small to Reduce the Blast Radius of Changes.** <br/>Focus on creating small, reusable components that adhere to the UNIX philosophy
  by doing one thing well. This strategy leads to simpler updates, more straightforward troubleshooting, quicker plan/apply cycles, and a
  clearer separation of responsibilities.
- **Split Components By Lifecycle.** <br/>For instance, a VPC, which is rarely destroyed, should be managed separately from more dynamic
  resources like clusters or databases that may frequently scale or undergo updates.
- **Make Them Opinionated, But Not Too Opinionated.** <br/> Ensure components are generalized to prevent the proliferation of similar components,
  thereby promoting easier testing, reuse, and maintenance.
- **Use Parameterization, But Avoid Over-Parameterization.** <br/> Good parameterization ensures components are reusable, but components become difficult to
  test and document with too many parameters.
- **Avoid Creating Factories Inside of Components.** <br/> Minimize the blast radius of changes and maintain fast plan/apply cycles by not embedding factories within components that provision lists of resources. Instead, leverage [Stack configurations to serve as factories](https://en.wikipedia.org/wiki/Factory_(object-oriented_programming)) for provisioning multiple component instances. This approach keeps the state isolated and scales efficiently with the increasing number of component instances.
- **Use Component Libraries & Vendoring** Utilize a centralized [component library](/core-concepts/components/library) to distribute and
  share components across the organization efficiently. This approach enhances discoverability by centralizing where components are stored, preventing sprawl and ensuring components are easily accessible to everyone. Employ vendoring to retrieve remote dependencies, like components, ensuring the practice of immutable infrastructure.
- **Enforce Standards using OPA Policies** <br/> Apply component validation within stacks to establish policies governing component usage. These policies can be tailored as needed, allowing the same component to be validated differently depending on its context of use.
- **Organize Related Components with Folders.** <br/> Organize multiple related components in a common folder. Use nested folders as necessary, to logically group components. For example, by grouping components by cloud provider and layer (e.g. `components/terraform/aws/network/<vpc>`)
- **Document Component Interfaces and Usage.** <br/> Utilize tools such as [terraform-docs](https://terraform-docs.io) to thoroughly document the input variables and outputs of your component. Include snippets of stack configuration to simplify understanding for developers on integrating the component into their stack configurations. Providing examples that cover common use-cases of the component is particularly effective.
- **Version Components for Breaking Changes.** <br/> Use versioned folders within the component to delineate major versions (e.g. `/components/terraform/<something>/v1/`)
- **Use a Monorepo for Your Components.** <br/> For streamlined development and simplified dependency management, smaller companies should consolidate stacks and components in a single monorepo, facilitating easier updates and unified versioning. Larger companies and enterprises with multiple monorepos can benefit from a central repository for upstream components, and then use vendoring to easily pull in these shared components to team-specific monorepos.
- **Maintain Loose Coupling Between Components.** <br/> Avoid directly invoking one component from within another to ensure components remain loosely coupled. Specifically for Terraform components (root modules), this practice is unsupported due to the inability to define a backend in a child module, potentially leading to unexpected outcomes. It's crucial to steer clear of this approach to maintain system integrity.
- **Reserve Code Generation for Emergencies.** <br/> We generally advise against using code generation for application logic (components), because it's challenging to ensure good test coverage (e.g. with `terratest`) and no one likes to code review machine-generated boilerplate in Pull Requests.

## Component Schema

To configure a Component in a [Stack](/core-concepts/stacks), A Component consists of the infrastructure as code business logic (e.g. a Terraform "root" module) as well as the configuration of that
component. The configuration of a component is stored in a Stack configuration.

<br/>

:::info Disambiguation

- **Terraform Component** is a [Terraform Root Module](https://developer.hashicorp.com/terraform/language/modules#the-root-module)
  that consists of the resources defined in the `.tf` files in a working directory
  (e.g. [components/terraform/infra/vpc](https://github.com/cloudposse/atmos/tree/master/examples/tests/components/terraform/infra/vpc))

- **Atmos Component** provides configuration (variables and other settings) for a type of component (e.g. a Terraform component) and is defined in one or more YAML stack config
  files (which are called [Atmos stacks](/core-concepts/stacks))

:::

<br/>

The schema of an Atmos Component in an Atmos Stack is as follows:

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
        # In this example, we're referencing a component in the `components/terraform/stable/example` folder.
        component: stable/example

        # We can leverage multiple inheritance to sequentially deep merge multiple configurations
        inherits:
          - example-defaults

        # Settings are where we store configuration related to integrations.
        # It's a freeform map; anything can be placed here.
        settings:
          spacelift: { }

      # Define the terraform variables, which will get deep-merged and exported to a `.tfvars` file by atmos.
      vars:
        enabled: true
        name: superduper
        nodes: 10
```

### Component Attributes

#### vars

The `vars` section is a free-form map. Use [component validation](/core-concepts/components/validation) to enforce policies.

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
recommend that every tenant have its own Organizational Unit (OU).

Example:

```yaml
vars:
  tenant: platform
```

#### vars.stage

This is an *optional* [`terraform-null-label`](https://github.com/cloudposse/terraform-null-label) convention.

The `stage` is where workloads run. See our [glossary](/terms) for disambiguation.

Example:

```yaml
vars:
  # Production stage
  stage: prod
```

#### vars.environment

This is an *optional* [`terraform-null-label`](https://github.com/cloudposse/terraform-null-label) convention.

The `environment` is used for location where things run. See our [glossary](/terms) for disambiguation.

Example:

```yaml

vars:
  # us-east-1
  environment: ue1
```

#### metadata

The `metadata` section extends functionality of the component.

#### settings

The `settings` block is a free-form map used to pass configuration information to [integrations](/integrations).

### Types of Components

The type of component is expressed in the `metadata.type` parameter of a given component configuration.

There are two types of components:

- `real` - is a ["concrete"](https://en.wikipedia.org/wiki/Concrete_class) component instance that can be provisioned
- `abstract` - a component configuration, which cannot be instantiated directly. The concept is borrowed
  from ["abstract base classes"](https://en.wikipedia.org/wiki/Abstract_type) of Object-Oriented Programming

## Flavors of Components

Atmos natively supports two kinds of components, but using [custom commands](/core-concepts/custom-commands), the [CLI](/cli) can be extended to support anything (e.g. `docker`, `packer`, `ansible`, etc.)

1. **Terraform:** These are stand-alone "root modules" that implement some piece of your infrastructure. For example, typical components might be an
   EKS cluster, RDS cluster, EFS filesystem, S3 bucket, DynamoDB table, etc. You can find
   the [full library of SweetOps Terraform components on GitHub](https://github.com/cloudposse/terraform-aws-components). By convention, we store
   components in the `components/terraform/` directory within the infrastructure repository.

2. **Helmfiles**: These are stand-alone applications deployed using [`helmfile`](https://github.com/helmfile) to Kubernetes. For example, typical
   helmfiles might deploy the DataDog agent, `cert-manager` controller, `nginx-ingress` controller, etc. By convention, we store these types of components in the `components/helmfile/` directory within the infrastructure repository.

## Terraform Components

One important distinction about components that is worth noting about Terraform components is they should be opinionated Terraform "root" modules that typically call other child modules. Components are the building blocks of your infrastructure. This is where you define all the business logic for provisioning some common piece of infrastructure like ECR repos (with the [ecr](https://github.com/cloudposse/terraform-aws-components/tree/master/modules/ecr) component) or EKS clusters (with the [eks/cluster](https://github.com/cloudposse/terraform-aws-components/tree/master/modules/eks/cluster) component). Our convention is to stick Terraform components in the `components/terraform/` directory.

If your components rely on submodules, our convention is to use a `modules/` subfolder of the component to store them.

We do not recommend consuming one terraform component inside of another as that would defeat the purpose; each component is intended to be
a loosely coupled unit of IaC with its own lifecycle. Furthermore, since components define a state backend and providers, it's not advisable to call one root module from another root module.
