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
The file then gets imported into the parent Atmos stacks.
This makes the stack configurations DRY by reusing the component's config that is common for all environments.

Refer to [Stack Imports](/core-concepts/stacks/imports) for more details on Atmos imports.

In the `stacks/catalog/vpc-flow-logs-bucket.yaml` file, add the following default configuration for the `vpc-flow-logs-bucket/defaults` Atmos
component:

```yaml title="stacks/catalog/vpc-flow-logs-bucket.yaml"
components:
  terraform:
    vpc-flow-logs-bucket/defaults:
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

In the `stacks/catalog/vpc.yaml` file, add the following default config for the `vpc/defaults` Atmos component:

```yaml title="stacks/catalog/vpc.yaml"
components:
  terraform:
    vpc/defaults:
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
stack config file names can be anything, and they can be in any folder in any sub-folder in the `stacks` directory.

For example, when executing the `atmos terraform apply infra/vpc -s tenant1-ue2-dev`
command, the stack `tenant1-ue2-dev` is specified by the `-s` flag. By looking at `name_pattern: "{tenant}-{environment}-{stage}"`
(see [Configure CLI](/quick-start/configure-cli)) and processing the tokens, Atmos knows that the first part of the stack name is `tenant`, the second
part is `environment`, and the third part is `stage`. Then Atmos searches for the parent stack configuration file (in the `stacks` directory)
where `tenant: tenant1`, `environment: ue2` and `stage: dev` are defined (inline or via imports).

Atmos parent stacks can be configured using a Basic Layout or a Hierarchical Layout.

The Basic Layout can be used when you have a very simple configuration using just a few accounts and regions.
The Hierarchical Layout should be used when you have a very complex organization, for example, with many AWS Organizational Units (which Atmos
refers to as tenants) and dozens of AWS accounts and regions.

### Basic Layout

A basic form of stack organization is to follow the pattern of naming where each `$environment-$stage.yaml` is a file. This works well until you have
so many environments and stages.

For example, `$environment` might be `ue2` (for `us-east-2`) and `$stage` might be `prod` which would result in `stacks/ue2-prod.yaml`

Some resources, however, are global in scope. For example, Route53 and IAM might not make sense to tie to a region. These are what we call "global
resources". You might want to put these into a file like `stacks/global-region.yaml` to connote that they are not tied to any particular region.

In our example, the filesystem layout for the stacks Basic Layout using `dev`, `staging` and `prod` accounts and `us-east-2` and `us-west-2` regions
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

As we can see, in the region and stage mixins, besides some other common variables, we are defining the global context variables `environment`
and `stage`, which Atmos uses when searching for a component in a stack. These mixins then gets imported into the parent Atmos stacks without defining
the context variables in each parent stack, making the configuration DRY.

### Configure Defaults for Organization, OU and accounts

The `_defaults.yaml` files contain the default settings for the Organization(s), Organizational Unit(s) and AWS accounts.

In `stacks/orgs/acme/_defaults.yaml`, add the following config:

```yaml title="stacks/orgs/acme/_defaults.yaml"
vars:
  namespace: acme
```

The file defines the context variable `namespace` for the entire `acme` Organization.

In `stacks/orgs/acme/core/_defaults.yaml`, add the following config for the `core` OU (tenant):

```yaml title="stacks/orgs/acme/core/_defaults.yaml"
import:
  - orgs/acme/_defaults

vars:
  tenant: core
```

In the `stacks/orgs/acme/core/_defaults.yaml` file, we import the defaults for the Organization and define the context variable `tenant` (which
corresponds to the `core` Organizational Unit). When Atmos processes this stack config, it will import and deep-merge all the variables defined in the
imported files and inline.

In `stacks/orgs/acme/core/dev/_defaults.yaml`, add the following config for the `dev` account:

```yaml title="stacks/orgs/acme/core/dev/_defaults.yaml"
import:
  - mixins/stage/dev
  - orgs/acme/core/_defaults
```

In the file, we import the mixing for the `dev` account (which defines `stage: dev` variable) and then the defaults for the `core` tenant (which,
as was described above, imports the defaults for the Organization). After processing all these imports, Atmos determines the values for the three
context variables `namespace`, `tenant` and `stage`, which it then sends to the Terraform components as Terraform variables. We are using hierarchical
imports here.

Similar to the `dev` account, add the following configs for the `staging` and `prod` accounts:

```yaml title="stacks/orgs/acme/core/staging/_defaults.yaml"
import:
  - mixins/stage/staging
  - orgs/acme/core/_defaults
```

```yaml title="stacks/orgs/acme/core/prod/_defaults.yaml"
import:
  - mixins/stage/prod
  - orgs/acme/core/_defaults
```

<br/>

### Configure Parent Stacks

After we've configured the catalog for the components, the mixins for the regions and stages, and the defaults for the Organization, OU and accounts,
the final step is to configure the Atmos parent (top-level) stacks and the Atmos components in the stacks.

In `stacks/orgs/acme/core/dev/us-east-2.yaml`, add the following config:

```yaml title="stacks/orgs/acme/core/dev/us-east-2.yaml"
# Import the region mixin, the defaults, and the base component configurations from the `catalog`.
# `import` supports POSIX-style Globs for file names/paths (double-star `**` is supported).
# File extensions are optional (if not specified, `.yaml` is used by default).
import:
  - mixins/region/us-east-2
  - orgs/acme/core/dev/_defaults
  - catalog/vpc
  - catalog/vpc-flow-logs-bucket

components:
  terraform:

    vpc-flow-logs-bucket-1:
      metadata:
        # Point to the Terraform component in `components/terraform` folder
        component: infra/vpc-flow-logs-bucket
        inherits:
          # Inherit all settings and variables from the 
          # `vpc-flow-logs-bucket/defaults` base Atmos component
          - vpc-flow-logs-bucket/defaults
      vars:
        # Define variables that are specific for this component
        # and are not set in the base component
        name: vpc-flow-logs-bucket-1
        # Override the default variables from the base component
        traffic_type: "REJECT"

    vpc-1:
      metadata:
        # Point to the Terraform component in `components/terraform` folder
        component: infra/vpc
        inherits:
          # Inherit all settings and variables from the `vpc/defaults` base Atmos component
          - vpc/defaults
      vars:
        # Define variables that are specific for this component
        # and are not set in the base component
        name: vpc-1
        ipv4_primary_cidr_block: 10.8.0.0/18
        # Override the default variables from the base component
        vpc_flow_logs_enabled: true
        vpc_flow_logs_traffic_type: "REJECT"

        # Specify the name of the Atmos component that provides configuration
        # for the `infra/vpc-flow-logs-bucket` Terraform component
        vpc_flow_logs_bucket_component_name: vpc-flow-logs-bucket-1

        # Override the context variables to point to a different Atmos stack if the 
        # `vpc-flow-logs-bucket-1` Atmos component is provisioned in another AWS account, OU or region.

        # If the bucket is provisioned in a different AWS account, 
        # set `vpc_flow_logs_bucket_stage_name`
        # vpc_flow_logs_bucket_stage_name: prod

        # If the bucket is provisioned in a different AWS OU, 
        # set `vpc_flow_logs_bucket_tenant_name`
        # vpc_flow_logs_bucket_tenant_name: core

        # If the bucket is provisioned in a different AWS region, 
        # set `vpc_flow_logs_bucket_environment_name`
        # vpc_flow_logs_bucket_environment_name: uw2
```

In the file, we first import the region mixin, the defaults for the Organization, OU and account (using hierarchical imports), and then the base
component configurations from the catalog. Then we define two Atmos components `vpc-flow-logs-bucket-1` and `vpc-1`, which inherit the base config
from the default Atmos components in the catalog and define and override some variables specific to the components in the stacks.

Similarly, create the parent Atmos stack for the `dev` account in `us-west-2` region:

```yaml title="stacks/orgs/acme/core/dev/us-west-2.yaml"
import:
  - mixins/region/us-west-2
  - orgs/acme/core/dev/_defaults
  - catalog/vpc
  - catalog/vpc-flow-logs-bucket

components:
  terraform:

    vpc-flow-logs-bucket-1:
      metadata:
        component: infra/vpc-flow-logs-bucket
        inherits:
          - vpc-flow-logs-bucket/defaults
      vars:
        name: vpc-flow-logs-bucket-1

    vpc-1:
      metadata:
        component: infra/vpc
        inherits:
          - vpc/defaults
      vars:
        name: vpc-1
        ipv4_primary_cidr_block: 10.9.0.0/18
```

<br/>

Similar to the `dev` account, create the parent stacks for the `staging` and `prod` accounts for both `us-east-2` and `us-west-2` regions in the files
`stacks/orgs/acme/core/staging/us-east-2.yaml`, `stacks/orgs/acme/core/staging/us-west-2.yaml` , `stacks/orgs/acme/core/prod/us-east-2.yaml` and
`stacks/orgs/acme/core/prod/us-west-2.yaml`.

For clarity, we skip these configurations here since they are similar to what we showed for the `dev`
account except for importing different region mixins and the defaults, and providing different values for components' variables in different stacks.
