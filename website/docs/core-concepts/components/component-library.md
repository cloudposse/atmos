---
title: Component Library
sidebar_position: 5
sidebar_label: Library
description: "A component library is a collection of reusable building blocks."
id: library
---

A component library is a collection of reusable "components" that are reused any number of times from within [Stacks](/core-concepts/stacks). 
It's helpful to think of these as the essential "building blocks" of infrastructure, like VPCs, clusters or databases.

:::tip
Get a head start by utilizing Cloud Posse's free [Terraform components for AWS](https://github.com/cloudposse/terraform-aws-components), available on GitHub.
:::

## Use-cases

- **Developer Productivity** Create a component library of vetted terraform root modules that should be used by teams anytime they need to spin
  up infrastructure for VPCs, clusters, and databases.
- **Compliance and Governance:** Establish a component library to enforce infrastructure standards, security policies, and compliance requirements.
  By using pre-approved modules, organizations can maintain control over their infrastructure's configuration, reducing the risk of non-compliance.
- **Rapid Prototyping and Scalability:** Utilize a component library to quickly prototype and scale applications. Pre-built modules for common
  infrastructure patterns allow teams to focus on application development rather than infrastructure setup, accelerating time-to-market and ensuring scalability from the outset.


## Filesystem Layouts

There's no "one way" to organize your components, since it's configurable based on your needs in the [CLI Configuration](/cli/configuration). However, here are some popular ways we've seen components organized.

### Simple Filesystem Layout by Toolchain

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


### Multi-Cloud Filesystem Layout

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
