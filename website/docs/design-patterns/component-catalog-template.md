---
title: Component Catalog Template Atmos Design Pattern
sidebar_position: 7
sidebar_label: Component Catalog Template
description: Component Catalog Template Atmos Design Pattern
---

# Component Catalog Template

The **Component Catalog Template** design pattern is used when you have an unbounded number of a component's instances provisioned in one environment
(the same organization, OU/tenant, account and region). New instances with different settings can be configured and provisioned anytime. The old
instances must be kept unchanged and never destroyed.

This is achieved by using [`Go` Templates in Imports](/core-concepts/stacks/imports#go-templates-in-imports) and
[Hierarchical Imports with Context](/core-concepts/stacks/imports#hierarchical-imports-with-context).

The **Component Catalog Template** pattern recommends the following:

- In the component's catalog folder, create a [`Go` template](https://pkg.go.dev/text/template) manifest with all the configurations for the
  component (refer to [`Go` Templates in Imports](/core-concepts/stacks/imports#go-templates-in-imports) for more details)

- Import the `Go` template manifest into a top-level stack many times to configure the component's instances
  using [Hierarchical Imports with Context](/core-concepts/stacks/imports#hierarchical-imports-with-context) and providing different template values
  for each import

## Applicability

Use the **Component Catalog Template** pattern when:

- You have an unbounded number of a component's instances provisioned in one environment (the same organization, OU/tenant, account and region)

- New instances of the component with different settings can be configured and provisioned anytime

- The old instances of the component must be kept unchanged and never destroyed

- You want to keep the configurations [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   └── catalog
   │       └── eks
   │           └── iam-role
   │               └── component.tmpl
   │   # Centralized components configuration
   └── components
       └── terraform  # Terraform components (Terraform root modules)
           └── eks
               └── iam-role
```

## Example

Add the following minimal configuration to `atmos.yaml` [CLI config file](/cli/configuration) :

```yaml title="atmos.yaml"
components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  name_pattern: "{tenant}-{environment}-{stage}"
  included_paths:
    # Tell Atmos to search for the top-level stack manifests in the `orgs` folder and its sub-folders
    - "orgs/**/*"
  excluded_paths:
    # Tell Atmos that the `defaults` folder and all sub-folders don't contain top-level stack manifests
    - "defaults/**/*"

schemas:
  jsonschema:
    base_path: "stacks/schemas/jsonschema"
  opa:
    base_path: "stacks/schemas/opa"
  atmos:
    manifest: "stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
```

Add the following configuration to the `stacks/catalog/vpc-flow-logs-bucket/defaults.yaml` manifest:

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

Add the following default configuration to the `stacks/catalog/vpc/defaults.yaml` manifest:

```yaml title="stacks/catalog/vpc/defaults.yaml"
components:
  terraform:
    vpc:
      metadata:
        # Point to the Terraform component
        component: vpc
      settings:
        # All validation steps must succeed to allow the component to be provisioned
        validation:
          validate-vpc-component-with-jsonschema:
            schema_type: jsonschema
            schema_path: "vpc/validate-vpc-component.json"
            description: Validate 'vpc' component variables using JSON Schema
          check-vpc-component-config-with-opa-policy:
            schema_type: opa
            schema_path: "vpc/validate-vpc-component.rego"
            module_paths:
              - "catalog/constants"
            description: Check 'vpc' component configuration using OPA policy
      vars:
        enabled: true
        name: "common"
        max_subnet_count: 3
        map_public_ip_on_launch: true
        dns_hostnames_enabled: true
        assign_generated_ipv6_cidr_block: false
        nat_gateway_enabled: true
        nat_instance_enabled: false
        vpc_flow_logs_enabled: true
        vpc_flow_logs_traffic_type: "ALL"
        vpc_flow_logs_log_destination_type: "s3"
        nat_eip_aws_shield_protection_enabled: false
        subnet_type_tag_key: "acme/subnet/type"
```

Add the following default configuration to the `stacks/catalog/vpc/ue2.yaml` manifest:

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

Add the following default configuration to the `stacks/catalog/vpc/uw2.yaml` manifest:

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

Add the following default configuration to the `stacks/catalog/vpc/prod.yaml` manifest:

```yaml title="stacks/catalog/vpc/prod.yaml"
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

The **Component Catalog Template** pattern provides the following benefits:

- The defaults for the components are defined in just one place (in the catalog) making the entire
  configuration [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

- The defaults for the components are reusable across many environments by using hierarchical [imports](/core-concepts/stacks/imports)

- It's easy to add a new manifest in the component's catalog to enable a new component's feature, then import the manifest into the corresponding
  stacks where the feature is required

## Limitations

The **Component Catalog Template** pattern has the following limitations and drawbacks:

- Although it's always recommended to use, the catalog structure described by the pattern can be complex for basic infrastructures,
  e.g. for a very simple organizational structure (one organization and OU), and just a few components deployed into a few accounts and regions

:::note

To address the limitations of the **Component Catalog Template** design pattern when you are provisioning a very basic infrastructure, use the
following patterns:

- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)

:::

## Related Patterns

- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
- [Component Inheritance](/design-patterns/component-inheritance)
- [Inline Component Configuration](/design-patterns/inline-component-configuration)
- [Inline Component Customization](/design-patterns/inline-component-customization)
- [Organizational Structure Configuration](/design-patterns/organizational-structure-configuration)
