---
title: Create Atmos Stacks
sidebar_position: 6
sidebar_label: Create Stacks
---

In the previous step, we've configured the Terraform components and described how they can be copied into the repository.

Next step is to create and configure [Atmos stacks](/core-concepts/stacks).

## Create Catalog for Components

Atmos supports the [Catalog](/core-concepts/stacks/catalogs) pattern to configure default settings for Atmos components.
All the common default settings for each Atmos component should be in a separate file in the `stacks/catalog` directory.
The file then get imported into the parent Atmos stacks.
This makes the stack configurations DRY by reusing the component's config that is common for all environments.

In the `stacks/catalog/vpc-flow-logs-bucket.yaml` file, add the following default configuration for the `vpc-flow-logs-bucket-defaults` Atmos
component:

```yaml title="stacks/catalog/vpc-flow-logs-bucket.yaml"
components:
  terraform:
    vpc-flow-logs-bucket-defaults:
      metadata:
        # `metadata.type: abstract` makes the component `abstract`,
        # explicitly prohibiting the component from being deployed.
        # `atmos terraform apply` will fail with an error.
        # If `metadata.type` attribute is not specified, it defaults to `real`.
        # `real` components can be provisioned by `atmos` and CI/CD like Spacelift and Atlantis.
        type: abstract
      # Default variables, which will be inherited and can be overridden in the derived components
      vars:
        force_destroy: false
        lifecycle_rule_enabled: false
        traffic_type: "ALL"
```

In the `stacks/catalog/vpc.yaml` file, add the following default config for the `vpc-defaults` Atmos component:

```yaml title="stacks/catalog/vpc.yaml"
components:
  terraform:
    vpc-defaults:
      metadata:
        # `metadata.type: abstract` makes the component `abstract`,
        # explicitly prohibiting the component from being deployed.
        # `atmos terraform apply` will fail with an error.
        # If `metadata.type` attribute is not specified, it defaults to `real`.
        # `real` components can be provisioned by `atmos` and CI/CD like Spacelift and Atlantis.
        type: abstract
      # Default variables, which will be inherited and can be overridden in the derived components
      vars:
        public_subnets_enabled: false
        nat_gateway_enabled: false
        nat_instance_enabled: false
        max_subnet_count: 3
        vpc_flow_logs_enabled: false
        vpc_flow_logs_log_destination_type: s3
        vpc_flow_logs_traffic_type: "ALL"
```

<br/>

These default Atmos components will be imported into the parent Atmos stacks. The default variables (in the `vars` sections) will be reused, and can
also be overridden in the derived Atmos components by using [Atmos Component Inheritance](/core-concepts/components/inheritance).

## Atmos Parent Stacks

When executing the [CLI commands](/cli/cheatsheet), Atmos does not use the stack file names and their filesystem locations to search for the stack
where the component is defined. Instead, Atmos uses the context variables (`namespace`, `tenant`, `environment`, `stage`) to search for the stack. The
stack config file names cam be anything, and they can be in any folders in any sub-folders in the `stacks` directory.

For example, when executing the `atmos terraform apply infra/vpc -s tenant1-ue2-dev`
command, the stack `tenant1-ue2-dev` is specified by the `-s` flag. By looking at `name_pattern: "{tenant}-{environment}-{stage}"`
(see [Configure CLI](/quick-start/configure-cli)) and processing the tokens, Atmos knows that the first part of the stack name is `tenant`, the second
part is `environment`, and the third part is `stage`. Then Atmos searches for the parent stack configuration file (in the `stacks` directory)
where `tenant: tenant1`, `environment: ue2` and `stage: dev` are defined (inline or via imports).

Atmos parent stacks can be configured using a Basic Layout or a Hierarchical Layout.

The Basic Layout can be used when you have a very simple configuration using just a few accounts and regions.
The Hierarchical Layout should be used when you have a very complex organization, for example, with many AWS Organizational Units (which Atmos
refers to as tenants) and dozens of AWS accounts and region.

### Basic Layout

A basic form of stack organization is to follow the pattern of naming where each `$environment-$stage.yaml` is a file. This works well until you have
so many environments and stages.

For example, `$environment` might be `ue2` (for `us-east-2`) and `$stage` might be `prod` which would result in `stacks/ue2-prod.yaml`

Some resources, however, are global in scope. For example, Route53 and IAM might not make sense to tie to a region. These are what we call "global
resources". You might want to put these into a file like `stacks/global-region.yaml` to connote that they are not tied to any particular region.

In out example, the filesystem layout for the stacks Basic Layout using `dev`, `staging` and `prod` accounts and `us-east-2` and `us-west-2` regions
would look like this:

```console
   │  
   │   # Centralized stacks configuration
   ├── stacks
   │   ├── catalog
   │   │    ├── vpc.yaml
   │   │    └── vpc-flow-logs-bucket.yaml
   │   ├── ue2-dev.yaml
   │   ├── ue2-staging.yaml
   │   ├── ue2-prod.yaml
   │   ├── uw2-dev.yaml
   │   ├── uw2-staging.yaml
   │   └── uw2-prod.yaml
   │  
   │   # Centralized components configuration. Components are broken down by tool
   ├── components
   │   └── terraform   # Terraform components (Terraform root modules)
   │       ├── infra
   │       │   ├── vpc
   │       │   └── vpc-flow-logs-bucket
```

<br/>

### Hierarchical Layout

We recommend using a hierarchical layout that follows the way AWS thinks about infrastructure. This works very well when you may have dozens or
hundreds of accounts and regions that you operate in.

## Create Parent Stacks

Although in this Quick Start guide we use just a few Terraform components which we want to provision into three AWS accounts in just two AWS regions
(which could be considered basic), we will use the Hierarchical Layout to show how the Atmos stacks can be configured for very complex organizations
and infrastructures.

We will assume we are using just one Organization `acme` and just one AWS Organizational Unit (OU) `core`. But as you will notice, the layout
can be easily extended to support many AWS Organizations and Organizational Units.

Create the following filesystem layout (which will be the final layout for this Quick Start guide):

```console
   │  
   │   # Centralized stacks configuration
   ├── stacks
   │   ├── catalog
   │   │    ├── vpc.yaml
   │   │    └── vpc-flow-logs-bucket.yaml
   │   ├── mixins
   │   │    ├── region
   │   │    │   ├── us-east-2.yaml
   │   │    │   └── us-west-2.yaml
   │   │    ├── stage
   │   │    │   ├── dev.yaml
   │   │    │   ├── prod.yaml
   │   │    │   └── staging.yaml
   │   ├── orgs
   │   │    ├── acme
   │   │    │   ├── _defaults.yaml
   │   │    │   ├── core
   │   │    │   │    ├── _defaults.yaml
   │   │    │   │    ├── dev
   │   │    │   │    │   ├── _defaults.yaml
   │   │    │   │    │   ├── us-east-2.yaml
   │   │    │   │    │   └── us-west-2.yaml
   │   │    │   │    ├── prod
   │   │    │   │    │   ├── _defaults.yaml
   │   │    │   │    │   ├── us-east-2.yaml
   │   │    │   │    │   └── us-west-2.yaml
   │   │    │   │    ├── staging
   │   │    │   │    │   ├── _defaults.yaml
   │   │    │   │    │   ├── us-east-2.yaml
   │   │    │   │    │   └── us-west-2.yaml
   │  
   │   # Centralized components configuration. Components are broken down by tool
   ├── components
   │   └── terraform   # Terraform components (Terraform root modules)
   │       ├── infra
   │       │   ├── vpc
   │       │   └── vpc-flow-logs-bucket
```

### Configure Region and Stage Mixins

[Mixins](/core-concepts/stacks/mixins) are a special kind of "[import](/core-concepts/stacks/imports)".
It's simply a convention we recommend to distribute reusable snippets of configuration that alter the behavior in some deliberate way.
Mixins are not handled in any special way. They are technically identical to all other imports.

In `stacks/mixins/region/us-east-2.yaml`, add the following config:

```yaml title="stacks/mixins/region/us-east-2.yaml"
vars:
  region: us-east-2
  environment: ue2

components:
  terraform:
    vpc:
      metadata:
        component: infra/vpc
      vars:
        availability_zones:
          - us-east-2a
          - us-east-2b
          - us-east-2c
```

In `stacks/mixins/region/us-west-2.yaml`, add the following config:

```yaml title="stacks/mixins/region/us-west-2.yaml"
vars:
  region: us-west-2
  environment: uw2

components:
  terraform:
    vpc:
      metadata:
        component: infra/vpc
      vars:
        availability_zones:
          - us-west-2a
          - us-west-2b
          - us-west-2c
```

In `stacks/mixins/stage/dev.yaml`, add the following config:

```yaml title="stacks/mixins/stage/dev.yaml"
vars:
  stage: dev
```

In `stacks/mixins/stage/prod.yaml`, add the following config:

```yaml title="stacks/mixins/stage/prod.yaml"
vars:
  stage: prod
```

In `stacks/mixins/stage/staging.yaml`, add the following config:

```yaml title="stacks/mixins/stage/staging.yaml"
vars:
  stage: staging
```

<br/>

### Configure Defaults for Organization, OU and accounts

### Configure Parent Stacks

<br/>
