---
title: Component Library
sidebar_position: 5
sidebar_label: Library
description: "A component library is a collection of reusable building blocks."
id: library
---

A component library is a collection of reusable "components" that are reused any number of times from within [Stacks](/core-concepts/stacks). 
It's helpful to think of these are the essential "building blocks" of infrastructure, like VPCs, clusters or databases.

:::tip
Get a head start by utilizing Cloud Posse's free [Terraform components for AWS](https://github.com/cloudposse/terraform-aws-components), available on GitHub.
:::

## Use-cases

- Create a library of vetted terraform root modules that should be used by teams anytime they need to spin up infrastructure for VPCs, clusters, and databases.
- 

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
- Use vendoring of components pull down remote dependencies.
- Use component validation with stacks to define policies on how components should be used.
- **Implement Versioning for Components.** <br/> Organize multiple related components in a common folder
- **Document Component Interfaces and Usage.** <br/> Utilize tools such as terraform-docs to thoroughly document the input variables and outputs of your component. Include snippets of stack configuration to simplify understanding for developers on integrating the component into their stack configurations. Providing examples that cover common use-cases of the component is particularly effective.
- **Version Components for Breaking Changes.** <br/> 

## Component Filesystem Layout

By convention, we recommend placing components in a folder organized by the tool, within the `components/` folder. 
In the following example, our toolchain consists of `docker`, `helmfile` and `terraform`, so a folder is created for each one, with the code
for that component inside of it.

If using `terraform` with multiple clouds, use the [multi-cloud filesytem layout](#multi-cloud-filesystem-layout).

```console
└── components/
    ├── docker/
    │   └── Dockerfile
    ├── helmfile/
    │   └── example-app
    │       └── helmfile.yaml
    └── terraform/
        └── example/                  # This is a terraform "root" module
            ├── main.tf
            ├── outputs.tf
            ├── modules/              # You can include submodules inside the component folder,
            │   ├── bar/              # and then reference them inside the of your root module.
            │   └── foo/              # e.g.
            │       ├── main.tf       # module "foo" {
            │       ├── outputs.tf    #   source = "./modules/foo"
            │       └── variables.tf  #   ...
            └── variables.tf          # }
```

:::tip
Organizing the components on the filesystem is configurable in the [Atmos CLI configuration](/cli/configuration#configuration-file-atmosyaml).
:::


## Multi-Cloud Filesystem Layout

One good way to organize components is by the cloud provider for multi-cloud architectures.

For example, if an architecture consists of infrastructure in AWS, GCP, and Azure, it would look like this:

```console
└── components/
    └── terraform/
        ├── aws/                   # Components for Amazon Web Services (AWS)
        │   └── example/
        │       ├── main.tf
        │       ├── outputs.tf
        │       └── variables.tf
        ├── gcp/                   # Components for Google Cloud (GCP)
        │   └── example/
        │       ├── main.tf
        │       ├── outputs.tf
        │       └── variables.tf
        └── azure/                 # Components for Microsoft Azure (Azure)
            └── example/
                ├── main.tf
                ├── outputs.tf
                └── variables.tf
```

## Terraform Conventions

For terraform, we recommend placing the terraform "root" modules in the `components/terraform` folder. If the root modules depend on other child modules that are not hosted by a registry, we recommend placing them in a subfolder called `modules/`.

Make your Terraform components small, so they are easily reusable, but not so small that they only do to provide a single resource, which results in large, complicated configurations. A good rule of thumb is they should do one thing well. For example, provision a VPC along with all the subnets, NAT gateways, Internet gateways, NACLs, etc. 

Use multiple component to break infrastructure apart into smaller pieces based on how their lifecycles are connected. For example, a single component seldom provides a VPC and a Kubernetes cluster. That's because we should be able to destroy the Kubernetes cluster without destroying the VPC and all the other resources provisioned inside of the VPC (e.g. databases). The VPC, Kubernetes cluster and Databases all have different lifecycles. Similarly, we should be able to deploy a database and destroy it without also destorying all associated backups. Therefore the backups of a database should be a separate component from the database itself. 
