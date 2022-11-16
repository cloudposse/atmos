---
title: Component Library
sidebar_label: Library
description: "A component library is a collection of reusable building blocks."
id: library
---

A component library is a collection of reusable building blocks (aka "components") that can be instantiated any number of times from [Stacks](/core-concepts/stacks).


## Filesystem Layout

By convention, we recommend placing components in a folder organized by the tool (e.g. `components/terraform` or `components/docker`).

```console
└── components/
    ├── docker/
    │   └── Dockerfile
    ├── helmfile/
    │   └── example-app
    │       └── helmfile.yaml
    └── terraform/
        └── example/
            ├── main.tf
            ├── outputs.tf
            ├── modules/
            │   ├── bar/
            │   └── foo/
            │       ├── main.tf
            │       ├── outputs.tf
            │       └── variables.tf
            └── variables.tf
```

## Terraform Conventions

For terraform, we recommend placing the terraform "root" modules in the `components/terraform` folder. If the root modules depend on other child modules that are not hosted by a registry, we recommend placing them in a subfolder called `modules/`.

Make your Terraform components small, so they are easily reusable, but not so small that they only do to provide a single resource, which results in large, complicated configurations. A good rule of thumb is they should do one thing well. For example, provision a VPC along with all the subnets, NAT gateways, Internet gateways, NACLs, etc. 

Use multiple component to break infrastructure apart into smaller pieces based on how their lifecycles are connected. For example, a single component seldom provides a VPC and a Kubernetes cluster. That's because we should be able to destroy the Kubernetes cluster without destroying the VPC and all the other resources provisioned inside of the VPC (e.g. databases). The VPC, Kubernetes cluster and Databases all have different lifecycles. Similarly, we should be able to deploy a database and destroy it without also destorying all associated backups. Therefore the backups of a database should be a separate component from the database itself. 
