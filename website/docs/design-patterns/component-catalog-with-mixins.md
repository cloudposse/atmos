---
title: Component Catalog with Mixins Atmos Design Pattern
sidebar_position: 6
sidebar_label: Component Catalog with Mixins
description: Component Catalog with Mixins Atmos Design Pattern
---

# Component Catalog with Mixins

The **Component Catalog with Mixins** design pattern is a variation of the [Component Catalog](/design-patterns/component-catalog) pattern, with the
difference being that we first create parts of a component's configuration related to different environments (in `mixins` folder), and then assemble
environment-specific manifests by importing the parts, and then import the environment-specific manifests themselves into the top-level stacks.

It's similar to how [Helm](https://helm.sh/) and [helmfile](https://helmfile.readthedocs.io/en/latest/#environment) handle environments.

The **Component Catalog with Mixins** design pattern prescribes the following:

- For a Terraform component, create a folder with the same name in `stacks/catalog` to make it symmetrical and easy to find.
  For example, the `stacks/catalog/vpc` folder should mirror the `components/terraform/vpc` folder.

- In the component's catalog folder, in the `mixins` sub-folder, add manifests with component configurations for specific environments (organizations,
  tenants, regions, accounts). For example:

  - `stacks/catalog/vpc/mixins/defaults.yaml` - component manifest with all the default values for the component (the defaults that can be reused
    across multiple environments)
  - `stacks/catalog/vpc/mixins/dev.yaml` - component manifest with the settings related to the `dev` account
  - `stacks/catalog/vpc/mixins/prod.yaml` - component manifest with the settings related to the `prod` account
  - `stacks/catalog/vpc/mixins/staging.yaml` - component manifest with the settings related to the `staging` account
  - `stacks/catalog/vpc/mixins/ue2.yaml` - component manifest with the settings for `us-east-2` region
  - `stacks/catalog/vpc/mixins/uw2.yaml` - component manifest with the settings for `us-west-2` region
  - `stacks/catalog/vpc/mixins/core.yaml` - component manifest with the settings related to the `core` tenant
  - `stacks/catalog/vpc/mixins/plat.yaml` - component manifest with the settings related to the `plat` tenant
  - `stacks/catalog/vpc/mixins/org1.yaml` - component manifest with the settings related to the `org1` organization
  - `stacks/catalog/vpc/mixins/org2.yaml` - component manifest with the settings related to the `org2` organization

- In the component's catalog folder, add manifests for specific environments by assembling the corresponding mixins together (using imports). For
  example:

  - `stacks/catalog/vpc/org1-plat-ue2-dev.yaml` - manifest for the `org1` organization, `plat` tenant, `ue2` region, `dev` account
  - `stacks/catalog/vpc/org1-plat-ue2-prod.yaml` - manifest for the `org1` organization, `plat` tenant, `ue2` region, `prod` account
  - `stacks/catalog/vpc/org1-plat-ue2-staging.yaml` - manifest for the `org1` organization, `plat` tenant, `ue2` region, `staging` account
  - `stacks/catalog/vpc/org1-plat-uw2-dev.yaml` - manifest for the `org1` organization, `plat` tenant, `uw2` region, `dev` account
  - `stacks/catalog/vpc/org1-plat-uw2-prod.yaml` - manifest for the `org1` organization, `plat` tenant, `uw2` region, `prod` account
  - `stacks/catalog/vpc/org1-plat-uw2-staging.yaml` - manifest for the `org1` organization, `plat` tenant, `uw2` region, `staging` account
  - `stacks/catalog/vpc/org2-plat-ue2-dev.yaml` - manifest for the `org2` organization, `plat` tenant, `ue2` region, `dev` account
  - `stacks/catalog/vpc/org2-plat-ue2-prod.yaml` - manifest for the `org2` organization, `plat` tenant, `ue2` region, `prod` account
  - `stacks/catalog/vpc/org2-plat-ue2-staging.yaml` - manifest for the `org2` organization, `plat` tenant, `ue2` region, `staging` account
  - `stacks/catalog/vpc/org2-plat-uw2-dev.yaml` - manifest for the `org2` organization, `plat` tenant, `uw2` region, `dev` account
  - `stacks/catalog/vpc/org2-plat-uw2-prod.yaml` - manifest for the `org2` organization, `plat` tenant, `uw2` region, `prod` account
  - `stacks/catalog/vpc/org2-plat-uw2-staging.yaml` - manifest for the `org2` organization, `plat` tenant, `uw2` region, `staging` account

- Import the environment manifests into the top-level stacks. For example:

  - import the `stacks/catalog/vpc/org1-plat-ue2-dev.yaml` manifest into the `stacks/orgs/org1/plat/dev/us-east-2.yaml` top-level stack
  - import the `stacks/catalog/vpc/org1-plat-ue2-prod.yaml` manifest into the `stacks/orgs/org1/plat/prod/us-east-2.yaml` top-level stack
  - import the `stacks/catalog/vpc/org1-plat-uw2-staging.yaml` manifest into the `stacks/orgs/org1/plat/staging/us-west-2.yaml` top-level stack
  - import the `stacks/catalog/vpc/org2-plat-ue2-dev.yaml` manifest into the `stacks/orgs/org2/plat/dev/us-east-2.yaml` top-level stack
  - etc.

## Applicability

Use the **Component Catalog** pattern when:

- You have components that are provisioned into multiple top-level stacks with different configurations for each stack

- You need to make the component configurations reusable across different environments

- You want to keep the configurations [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   └── catalog
   │       └── vpc
   │           ├── mixins
   │           │   ├── defaults.yaml
   │           │   ├── dev.yaml
   │           │   ├── prod.yaml
   │           │   ├── staging.yaml
   │           │   ├── ue2.yaml
   │           │   ├── uw2.yaml
   │           │   ├── core.yaml
   │           │   ├── plat.yaml
   │           │   ├── org1.yaml
   │           │   └── org2.yaml
   │           ├── org1-plat-ue2-dev.yaml
   │           ├── org1-plat-ue2-prod.yaml
   │           ├── org1-plat-ue2-staging.yaml
   │           ├── org1-plat-uw2-dev.yaml
   │           ├── org1-plat-uw2-prod.yaml
   │           ├── org1-plat-uw2-staging.yaml
   │           ├── org2-plat-ue2-dev.yaml
   │           ├── org2-plat-ue2-prod.yaml
   │           ├── org2-plat-ue2-staging.yaml
   │           ├── org2-plat-uw2-dev.yaml
   │           ├── org2-plat-uw2-prod.yaml
   │           └── org2-plat-uw2-staging.yaml
   │   # Centralized components configuration
   └── components
       └── terraform  # Terraform components (Terraform root modules)
           └── vpc
```

## Example

Add the following minimal configuration to `atmos.yaml` [CLI config file](/cli/configuration) :

```yaml title="atmos.yaml"
components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    # Tell Atmos to search for the top-level stack manifests in the `orgs` folder and its sub-folders
    - "orgs/**/*"
  excluded_paths:
    # Tell Atmos that all `_defaults.yaml` files are not top-level stack manifests
    - "**/_defaults.yaml"
  # If you are using multiple organizations (namespaces), use the following `name_pattern`:
  name_pattern: "{namespace}-{tenant}-{environment}-{stage}"
  # If you are using a single organization (namespace), use the following `name_pattern`:
  # name_pattern: "{tenant}-{environment}-{stage}"

schemas:
  jsonschema:
    base_path: "stacks/schemas/jsonschema"
  opa:
    base_path: "stacks/schemas/opa"
  atmos:
    manifest: "stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
```

Add the following default configuration to the `stacks/catalog/vpc/mixins/defaults.yaml` manifest:

```yaml title="stacks/catalog/vpc/mixins/defaults.yaml"
components:
  terraform:
    vpc:
      metadata:
        # Point to the Terraform component
        component: vpc
      vars:
        enabled: true
        name: "common"
        max_subnet_count: 3
        map_public_ip_on_launch: true
        dns_hostnames_enabled: true
        vpc_flow_logs_enabled: true
        vpc_flow_logs_traffic_type: "ALL"
        vpc_flow_logs_log_destination_type: "s3"
```

Add the following default configuration to the `stacks/catalog/vpc/mixins/ue2.yaml` manifest:

```yaml title="stacks/catalog/vpc/mixins/ue2.yaml"
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

Add the following default configuration to the `stacks/catalog/vpc/mixins/uw2.yaml` manifest:

```yaml title="stacks/catalog/vpc/mixins/uw2.yaml"
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

Add the following default configuration to the `stacks/catalog/vpc/mixins/prod.yaml` manifest:

```yaml title="stacks/catalog/vpc/mixins/prod.yaml"
components:
  terraform:
    vpc:
      vars:
        # In `prod`, don't map public IPs on launch
        # Override `map_public_ip_on_launch` from the defaults
        map_public_ip_on_launch: false
```

Import `stacks/catalog/vpc/ue2.yaml` into the `stacks/mixins/region/us-east-2.yaml` manifest:

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

Import `stacks/catalog/vpc/uw2.yaml` into the `stacks/mixins/region/us-west-2.yaml` manifest:

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

Import `mixins/region/us-east-2` and `stacks/catalog/vpc/prod.yaml` into the `stacks/orgs/acme/plat/prod/us-east-2.yaml` top-level stack:

```yaml title="stacks/orgs/acme/plat/prod/us-east-2.yaml"
import:
  - orgs/acme/plat/prod/_defaults
  - mixins/region/us-east-2
  # Override the `vpc` component configuration for `prod` by importing the `catalog/vpc/prod` manifest
  - catalog/vpc/prod
```

Import `mixins/region/us-west-2` and `stacks/catalog/vpc/prod.yaml` into the `stacks/orgs/acme/plat/prod/us-west-2.yaml` top-level stack:

```yaml title="stacks/orgs/acme/plat/prod/us-west-2.yaml"
import:
  - orgs/acme/plat/prod/_defaults
  - mixins/region/us-west-2
  # Override the `vpc` component configuration for `prod` by importing the `catalog/vpc/prod` manifest
  - catalog/vpc/prod
```

## Benefits

The **Component Catalog with Mixins** pattern provides the following benefits:

- The defaults for the components are defined in just one place (in the catalog) making the entire
  configuration [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

- The defaults for the components are reusable across many environments by using hierarchical [imports](/core-concepts/stacks/imports)

- It's easy to add a new manifest in the component's catalog to enable a new component's feature, then import the manifest into the corresponding
  stacks where the feature is required

## Limitations

The **Component Catalog with Mixins** pattern has the following limitations and drawbacks:

- The structure described by the pattern can be complex for basic infrastructures, e.g. for a very simple organizational structure (one organization
  and OU), and just a few components deployed into a few accounts and regions

:::note

To address the limitations of the **Component Catalog with Mixins** pattern when you are provisioning a very basic infrastructure, use the following
patterns:

- [Component Catalog](/design-patterns/component-catalog)
- [Inline Component Configuration](/design-patterns/inline-component-configuration)
- [Inline Component Customization](/design-patterns/inline-component-customization)

:::

## Related Patterns

- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog Template](/design-patterns/component-catalog-template)
- [Component Inheritance](/design-patterns/component-inheritance)
- [Inline Component Configuration](/design-patterns/inline-component-configuration)
- [Inline Component Customization](/design-patterns/inline-component-customization)
- [Organizational Structure Configuration](/design-patterns/organizational-structure-configuration)
