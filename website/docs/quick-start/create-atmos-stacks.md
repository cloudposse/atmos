---
title: Create Atmos Stacks
sidebar_position: 6
sidebar_label: Create Stacks
---

In the previous step, we've configured the Terraform components and described how they can be vendored into the repository.

Next step is to create and configure [Atmos stacks](/core-concepts/stacks).

## Create Catalog for Components

Atmos supports the [Catalog](/core-concepts/stacks/catalogs) pattern to configure default settings for Atmos components.
All the common default settings for each Atmos component should be in a separate file in the `stacks/catalog` directory.
The file then gets imported into the parent Atmos stacks.
This makes the stack configurations DRY by reusing the component's config that is common for all environments.

Refer to [Stack Imports](/core-concepts/stacks/imports) for more details on Atmos imports.

In the `stacks/catalog/vpc-flow-logs-bucket/defaults.yaml` file, add the following manifest for the `vpc-flow-logs-bucket` Atmos component:

```yaml title="stacks/catalog/vpc-flow-logs-bucket/defaults.yaml"
components:
  terraform:
    vpc-flow-logs-bucket:
      metadata:
        # Point to the Terraform component
        component: vpc-flow-logs-bucket
      vars:
        enabled: true
        name: "vpc-flow-logs"
        traffic_type: "ALL"
        force_destroy: true
        lifecycle_rule_enabled: false
```

In the `stacks/catalog/vpc/defaults.yaml` file, add the following manifest for the `vpc` Atmos component:

```yaml title="stacks/catalog/vpc.yaml"
components:
  terraform:
    vpc:
      metadata:
        # Point to the Terraform component
        component: vpc
      settings:
        # Validation
        # Supports JSON Schema and OPA policies
        # All validation steps must succeed to allow the component to be provisioned
        validation:
          validate-vpc-component-with-jsonschema:
            schema_type: jsonschema
            # 'schema_path' can be an absolute path or a path relative to 'schemas.jsonschema.base_path' defined in `atmos.yaml`
            schema_path: "vpc/validate-vpc-component.json"
            description: Validate 'vpc' component variables using JSON Schema
          check-vpc-component-config-with-opa-policy:
            schema_type: opa
            # 'schema_path' can be an absolute path or a path relative to 'schemas.opa.base_path' defined in `atmos.yaml`
            schema_path: "vpc/validate-vpc-component.rego"
            # An array of filesystem paths (folders or individual files) to the additional modules for schema validation
            # Each path can be an absolute path or a path relative to `schemas.opa.base_path` defined in `atmos.yaml`
            # In this example, we have the additional Rego modules in `stacks/schemas/opa/catalog/constants`
            module_paths:
              - "catalog/constants"
            description: Check 'vpc' component configuration using OPA policy
            # Set `disabled` to `true` to skip the validation step
            # `disabled` is set to `false` by default, the step is allowed if `disabled` is not declared
            disabled: false
            # Validation timeout in seconds
            timeout: 10
      vars:
        enabled: true
        name: "common"
        nat_gateway_enabled: true
        nat_instance_enabled: false
        max_subnet_count: 3
        map_public_ip_on_launch: true
        dns_hostnames_enabled: true
        vpc_flow_logs_enabled: true
        vpc_flow_logs_traffic_type: "ALL"
        vpc_flow_logs_log_destination_type: "s3"
```

In the `stacks/catalog/vpc/ue2.yaml` file, add the following manifest for the `vpc` Atmos component:

```yaml title="stacks/catalog/vpc/ue2.yaml"
import:
  - catalog/vpc/defaults

components:
  terraform:
    vpc:
      vars:
        availability_zones:
          - us-east-2a
          - us-east-2b
          - us-east-2c
```

In the `stacks/catalog/vpc/uw2.yaml` file, add the following manifest for the `vpc` Atmos component:

```yaml title="stacks/catalog/vpc/uw2.yaml"
import:
  - catalog/vpc/defaults

components:
  terraform:
    vpc:
      vars:
        availability_zones:
          - us-west-2a
          - us-west-2b
          - us-west-2c
```

In the `stacks/catalog/vpc/prod.yaml` file, add the following manifest for the `vpc` Atmos component:

```yaml title="stacks/catalog/vpc/prod.yaml"
components:
  terraform:
    vpc:
      vars:
        # In `prod`, don't map public IPs on launch
        map_public_ip_on_launch: false
```

<br/>

These Atmos component manifests will be imported into the top-level Atmos stacks. The default variables (in the `vars` sections)
can be overridden in the derived Atmos components by using [Atmos Component Inheritance](/core-concepts/components/inheritance).

## Atmos Top-level Stacks

When executing the [CLI commands](/cli/cheatsheet), Atmos does not use the stack file names and their filesystem locations to search for the stack
where the component is defined. Instead, Atmos uses the context variables (`namespace`, `tenant`, `environment`, `stage`) to search for the stack. The
stack config file names (stack manifest names) can be anything, and they can be in any folder in any sub-folder in the `stacks` directory.

For example, when executing the `atmos terraform apply vpc -s plat-ue2-dev`
command, the Atmos stack `plat-ue2-dev` is specified by the `-s` flag. By looking at `name_pattern: "{tenant}-{environment}-{stage}"`
(see [Configure CLI](/quick-start/configure-cli)) and processing the tokens, Atmos knows that the first part of the stack name is `tenant`, the second
part is `environment`, and the third part is `stage`. Then Atmos searches for the top-level stack manifest (in the `stacks` directory)
where `tenant: plat`, `environment: ue2` and `stage: dev` are defined (inline or via imports).

Atmos top-level stacks can be configured using a Basic Layout or a Hierarchical Layout.

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
   └── components
       └── terraform   # Terraform components (Terraform root modules)
           ├── vpc
           └── vpc-flow-logs-bucket
```

<br/>

### Hierarchical Layout

We recommend using a hierarchical layout that follows the way AWS thinks about infrastructure. This works very well when you may have dozens or
hundreds of accounts and regions that you operate in.

## Create Top-level Stacks

Although in this Quick Start guide we use just a few Terraform components which we want to provision into three AWS accounts in just two AWS regions
(which could be considered basic), we will use the Hierarchical Layout to show how the Atmos stacks can be configured for very complex organizations
and infrastructures.

We will assume we are using just one Organization `acme` and just one AWS Organizational Unit (OU) `plat`. But as you will notice, the layout
can be easily extended to support many AWS Organizations and Organizational Units.

Create the following filesystem layout (which will be the final layout for this Quick Start guide):

```console
   │   # Centralized stacks configuration
   ├── stacks
   │   ├── catalog
   │   │    ├── vpc
   │   │    │   ├── defaults.yaml
   │   │    │   ├── disabled.yaml
   │   │    │   ├── prod.yaml
   │   │    │   ├── ue2.yaml
   │   │    │   └── uw2.yaml
   │   │    └── vpc-flow-logs-bucket
   │   │        ├── defaults.yaml
   │   │        └── disabled.yaml
   │   ├── mixins
   │   │    ├── tenant
   │   │    │   ├── core.yaml
   │   │    │   └── plat.yaml
   │   │    ├── region
   │   │    │   ├── us-east-2.yaml
   │   │    │   └── us-west-2.yaml
   │   │    └── stage
   │   │        ├── dev.yaml
   │   │        ├── prod.yaml
   │   │        └── staging.yaml
   │   └── orgs
   │        └── acme
   │            ├── _defaults.yaml
   │            └── plat
   │                 ├── _defaults.yaml
   │                 ├── dev
   │                 │   ├── _defaults.yaml
   │                 │   ├── us-east-2.yaml
   │                 │   ├── us-east-2-extras.yaml
   │                 │   └── us-west-2.yaml
   │                 ├── prod
   │                 │   ├── _defaults.yaml
   │                 │   ├── us-east-2.yaml
   │                 │   └── us-west-2.yaml
   │                 └── staging
   │                     ├── _defaults.yaml
   │                     ├── us-east-2.yaml
   │                     └── us-west-2.yaml
   │  
   │   # Centralized components configuration. Components are broken down by tool
   └── components
       └── terraform   # Terraform components (Terraform root modules)
           ├── vpc
           └── vpc-flow-logs-bucket
```

### Configure Region and Stage Mixins

[Mixins](/core-concepts/stacks/mixins) are a special kind of "[import](/core-concepts/stacks/imports)".
It's simply a convention we recommend to distribute reusable snippets of configuration that alter the behavior in some deliberate way.
Mixins are not handled in any special way. They are technically identical to all other imports.

In `stacks/mixins/tenant/core.yaml`, add the following config:

```yaml title="stacks/mixins/tenant/core.yaml"
vars:
  tenant: core

# Other defaults for the `core` tenant/OU
```

In `stacks/mixins/tenant/plat.yaml`, add the following config:

```yaml title="stacks/mixins/tenant/plat.yaml"
vars:
  tenant: plat

# Other defaults for the `plat` tenant/OU
```

In `stacks/mixins/region/us-east-2.yaml`, add the following config:

```yaml title="stacks/mixins/region/us-east-2.yaml"
import:
  # Import the `ue2` manifest with `vpc` configuration for `us-east-2` region
  - catalog/vpc/ue2
  # All accounts (stages) in `us-east-2` region will have the `vpc-flow-logs-bucket` component
  - catalog/vpc-flow-logs-bucket/defaults

vars:
  region: us-east-2
  environment: ue2

# Other defaults for the `us-east-2` region
```

In `stacks/mixins/region/us-west-2.yaml`, add the following config:

```yaml title="stacks/mixins/region/us-west-2.yaml"
import:
  # Import the `uw2` manifest with `vpc` configuration for `us-west-2` region
  - catalog/vpc/uw2
  # All accounts (stages) in `us-west-2` region will have the `vpc-flow-logs-bucket` component
  - catalog/vpc-flow-logs-bucket/defaults

vars:
  region: us-west-2
  environment: uw2

# Other defaults for the `us-west-2` region
```

In `stacks/mixins/stage/dev.yaml`, add the following config:

```yaml title="stacks/mixins/stage/dev.yaml"
vars:
  stage: dev

# Other defaults for the `dev` stage/account
```

In `stacks/mixins/stage/prod.yaml`, add the following config:

```yaml title="stacks/mixins/stage/prod.yaml"
vars:
  stage: prod

# Other defaults for the `prod` stage/account
```

In `stacks/mixins/stage/staging.yaml`, add the following config:

```yaml title="stacks/mixins/stage/staging.yaml"
vars:
  stage: staging

# Other defaults for the `staging` stage/account
```

<br/>

As we can see, in the tenant, region and stage mixins, besides some other common variables, we are defining the global context
variables `tenant`, `environment` and `stage`, which Atmos uses when searching for a component in a stack. These mixins then get imported into the
parent Atmos stacks without defining the context variables in each top-level stack, making the configuration DRY.

### Configure Defaults for Organization, OU and accounts

The `_defaults.yaml` files contain the default settings for the Organization(s), Organizational Units and AWS accounts.

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
  - mixins/tenant/core
```

In `stacks/orgs/acme/plat/_defaults.yaml`, add the following config for the `plat` OU (tenant):

```yaml title="stacks/orgs/acme/plat/_defaults.yaml"
import:
  - orgs/acme/_defaults
  - mixins/tenant/plat
```

In the `stacks/orgs/acme/plat/_defaults.yaml` file, we import the defaults for the Organization and for the `plat` tenant (which
corresponds to the `plat` Organizational Unit). When Atmos processes this stack config, it will import and deep-merge all the variables defined in the
imported files and inline. All imports are processed in the order they are defined.

In `stacks/orgs/acme/plat/dev/_defaults.yaml`, add the following config for the `dev` account:

```yaml title="stacks/orgs/acme/plat/dev/_defaults.yaml"
import:
  - orgs/acme/plat/_defaults
  - mixins/stage/dev
```

In the file, we import the mixin for the `plat` tenant (which, as was described above, imports the defaults for the Organization), and then the mixin
for the `dev` account (which defines `stage: dev` variable). After processing all these imports, Atmos determines the values for the three context
variables `namespace`, `tenant` and `stage`, which it then sends to the Terraform components as Terraform variables. We are using hierarchical imports
here.

Similar to the `dev` account, add the following configs for the `prod` and `staging` accounts:

```yaml title="stacks/orgs/acme/plat/prod/_defaults.yaml"
import:
  - orgs/acme/plat/_defaults
  - mixins/stage/prod
```

```yaml title="stacks/orgs/acme/plat/staging/_defaults.yaml"
import:
  - orgs/acme/plat/_defaults
  - mixins/stage/staging
```

<br/>

### Configure Top-level Stacks

After we've configured the catalog for the components, the mixins for the tenants, regions and stages, and the defaults for the Organization, OU and
accounts, the final step is to configure the Atmos root (top-level) stacks and the Atmos components in the stacks.

In `stacks/orgs/acme/plat/dev/us-east-2.yaml`, add the following config:

```yaml title="stacks/orgs/acme/plat/dev/us-east-2.yaml"
# `import` supports POSIX-style Globs for file names/paths (double-star `**` is supported).
# File extensions are optional (if not specified, `.yaml` is used by default).
import:
  - orgs/acme/plat/dev/_defaults
  - mixins/region/us-east-2
```

In the file, we import the region mixin and the defaults for the Organization, OU and account (using hierarchical imports).

In `stacks/orgs/acme/plat/dev/us-east-2-extras.yaml`, add the following config:

```yaml title="stacks/orgs/acme/plat/dev/us-east-2-extras.yaml"
import:
  - orgs/acme/plat/dev/_defaults
  - mixins/region/us-east-2
  # In this `orgs/acme/plat/dev/us-east-2-extras.yaml` manifest,
  # you can import or define other components that are not defined in the `orgs/acme/plat/dev/us-east-2.yaml` manifest
  # This pattern is called `Atmos Partial Stack Configuration`

components:
  terraform: {}
```

Similarly, create the top-level Atmos stack for the `dev` account in `us-west-2` region:

```yaml title="stacks/orgs/acme/plat/dev/us-west-2.yaml"
import:
  - orgs/acme/plat/dev/_defaults
  - mixins/region/us-west-2
```

In `stacks/orgs/acme/plat/staging/us-east-2.yaml`, add the following config:

```yaml title="stacks/orgs/acme/plat/staging/us-east-2.yaml"
import:
  - orgs/acme/plat/staging/_defaults
  - mixins/region/us-east-2
```

Similarly, create the top-level Atmos stack for the `staging` account in `us-west-2` region:

```yaml title="stacks/orgs/acme/plat/staging/us-west-2.yaml"
import:
  - orgs/acme/plat/staging/_defaults
  - mixins/region/us-west-2
```

In `stacks/orgs/acme/plat/prod/us-east-2.yaml`, add the following config:

```yaml title="stacks/orgs/acme/plat/prod/us-east-2.yaml"
# Import the tenant and region mixins, and the defaults for the components from the `catalog`.
# `import` supports POSIX-style Globs for file names/paths (double-star `**` is supported).
# File extensions are optional (if not specified, `.yaml` is used by default).
import:
  - orgs/acme/plat/prod/_defaults
  - mixins/region/us-east-2
  # Override the `vpc` component configuration for `prod` by importing the `vpc/prod` manifest
  - catalog/vpc/prod
```

In the file, we import the region mixin and the defaults for the Organization, OU and account (using hierarchical imports).

Similarly, create the top-level Atmos stack for the `prod` account in `us-west-2` region:

```yaml title="stacks/orgs/acme/plat/prod/us-west-2.yaml"
import:
  - orgs/acme/plat/prod/_defaults
  - mixins/region/us-west-2
  # Override the `vpc` component configuration for `prod` by importing the `vpc/prod` manifest
  - catalog/vpc/prod
```
