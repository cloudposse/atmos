---
title: Stack Catalogs
sidebar_position: 7
sidebar_label: Catalogs
id: catalogs
description: Catalogs are how to organize all Stack configurations for easy imports. 
---

Catalogs are how to logically organize all of the [Stack](/core-concepts/stacks) configurations for use by [imports](/core-concepts/stacks/imports). There's no "right or wrong" way to do it, and Atmos does not enforce any one convention.  What we've come to realize is there's no "one way" to organize Stack configurations. The best way to organize them will come down to the way an organization wants to model infrastructure.

Below is how we implement them at [Cloud Posse](https://cloudposse.com).


# Conventions

We provide a number of recommended conventions for your Stack catalogs. You can use all of them or some of them. These conventions have come about from our [customer engagements](https://cloudposse.com/services).

[Cloud Posse](https://cloudposse.com) typically uses `orgs` as the parent stacks, which import `teams`, `mixins` and other services from a `catalog`.

## Filesystem Layout

Here's an example of how Stack imports might be organized on disk.

```console
└── stacks/
    ├── mixins/
    │   └── region/
    │       ├── us-east-1.yaml
    │       ├── us-west-2.yaml
    │       └── eu-west-1.yaml    
    │   └── stage/
    ├── teams/
    │   └── frontend/
    │       └── example-application/
    │           └── microservice/
    │               ├── prod.yaml
    │               ├── dev.yaml
    │               └── staging.yaml
    └── catalogs/
        ├── vpc/
        │   └── baseline.yaml
        └── database/
            ├── baseline.yaml
            ├── small.yaml
            ├── medium.yaml
            └── large.yaml
```

## Types of Catalogs


### Mixins

We go into more detail on using [Mixins](/core-concepts/stacks/mixins) to manage snippets of reusable configuration. These Mixins are frequently used along side the other conventions such as Teams and Organizations.

### Teams

When infrastructure gets very large and there's numerous teams managing it, it can be helpful to organize Stack configurations around the notion of "teams". This way it's possible to leverage [`CODEOWNERS`](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners) together with [branch protection rules](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/defining-the-mergeability-of-pull-requests/about-protected-branches#require-pull-request-reviews-before-merging) to restrict who can merge pull requests that affect infrastructure.

Here's what that might look like:
```console
└── stacks/
    └── teams/
        └── frontend/
            └── ecom-store/
                ├── checkout/
                │    ├── prod.yaml
                │    ├── dev.yaml
                │    └── staging.yaml
                └── cart/
                    ├── prod.yaml
                    ├── dev.yaml
                    └── staging.yaml
```

In this example, there's a `frontend` team that owns an `ecom-store` application. The application consists of two microservices called `checkout` and `cart`. Each microservice has (3) stages: `dev`, `staging` and `prod`.

### Organizations

The organizational layout of Stacks is useful for modeling how infrastructure gets "physically" deployed with a given Infrastructure as a Service (IaaS) platform like AWS.

What's important to point out is that all these conventions are not mutually exclusive. In fact, we like to combine them.

Here's what that might look like:
```console
└── orgs/
    └── acme/
        └── platform/
            ├── prod/
            │   ├── us-east-1/
            │   │    ├── networking.yaml
            │   │    ├── compliance.yaml
            │   │    ├── backing-services.yaml
            │   │    └── teams.yaml
            │   └── us-west-2/
            │       ├── networking.yaml
            │       ├── compliance.yaml                        
            │       ├── backing-services.yaml
            │       └── teams.yaml
            ├── staging/
            │   └── us-west-1/
            │       ├── networking.yaml
            │       ├── compliance.yaml   
            │       ├── backing-services.yaml
            │       └── teams.yaml
            └── dev/
                └── us-west-2/
                    ├── networking.yaml
                    ├── backing-services.yaml
                    └── teams.yaml                  
                 
```

In this example, there's a single organization called `acme` with an example of one organizational unit (OU) called `platform`. The OU has 3 stages: `dev`, `staging`, and `prod`. Each stage then operates in a number of regions. Each region then has a `networking` layer, a `backing-services` layer, and a `teams` layer. The `staging` and `prod` accounts have both have a `compliance` layer, which isn't needed in the `dev` stages.

### Everything Else

For everything else, we usually have catalog that we just call `catalog/`. We place it underneath the `stacks/` folder. This is for everything else we want to define once and reuse. Use whatever convention makes sense for your company.

## Refactoring Configurations

One of the amazing things about the Atmos [Stack](/core-concepts/stacks) configurations is that the entire state of configuration is stored in the YAML configurations. The filesystem layout has no bearing on the desired state of the configuratino. This means that configurations can be easily refactored at at time in the future, if you discover there's a better way to organize your Stack configurations. So long as the deep-merged configuration is the same, it will not affect any of the [Components](/core-concepts/components).
