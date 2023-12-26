---
title: Multiple Component Instances Atmos Design Pattern
sidebar_position: 8
sidebar_label: Multiple Component Instances
description: Multiple Component Instances Atmos Design Pattern
---

# Multiple Component Instances

Atmos provides unlimited flexibility in defining and configuring stacks and components in the stacks.

- Terraform components can be in different sub-folders in the `components/terraform` directory. The sub-folders can be organized by type, by teams
  that are responsible for the components, by operations that are performed on the components, or by any other category

- Atmos stack manifests can have arbitrary names and can be located in any sub-folder in the `stacks` directory. Atmos stack filesystem layout is for
  people to better organize the stacks and make the configurations DRY. Atmos (the CLI) does not care about the filesystem layout, all it cares about
  is how to find the stacks and the components in the stacks by using the context variables `namespace`, `tenant`, `environment` and `stage`

- An Atmos component can have any name different from the Terraform component name. For example, two different Atmos components `vpc-1` and `vpc-2`
  can provide configuration for the same Terraform component `vpc`

- We can provision more than one instance of the same Terraform component (with the same or different settings) into the same environment by defining
  many Atmos components that provide configuration for the Terraform component. For example, the following config shows how to define two Atmos
  components, `vpc-1` and `vpc-2`, which both point to the same Terraform component `vpc`:

The **Multiple Component Instances** pattern prescribes the following:

- For each Terraform component, create a folder with the same name in `stacks/catalog` to make it symmetrical and easy to find.
  For example, the `stacks/catalog/vpc` folder should mirror the `components/terraform/vpc` folder.

- In the component's catalog folder, create `defaults.yaml` manifest with all the default values for the component (the defaults that can be reused
  across multiple environments). Define all the required Atmos sections, e.g. `metadata`, `settings`, `vars`, `env`.

## Applicability

Use the **Multiple Component Instances** pattern when:

- You have many components that are provisioned into multiple stacks with different configurations for each stack

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   └── catalog
   │       └── vpc
   │           └── defaults.yaml
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

Add the following default configuration to the `stacks/catalog/vpc/defaults.yaml` manifest:

```yaml title="stacks/catalog/vpc/defaults.yaml"
components:
  terraform:
    vpc/defaults:
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
        ipv4_primary_cidr_block: 10.9.0.0/18
```

Configure multiple `vpc` component instances in the `stacks/orgs/acme/plat/prod/us-east-2.yaml` top-level stack:

```yaml title="stacks/orgs/acme/plat/prod/us-west-2.yaml"
import:
  import:
    - orgs/acme/plat/dev/_defaults
    - mixins/region/us-east-2
    # Import the defaults for all VPC components
    - catalog/vpc/defaults

  components:
    terraform:
      # Atmos component `vpc-1`
      vpc-1:
        metadata:
          # Point to the Terraform component in `components/terraform/vpc`
          component: vpc
          # Inherit the defaults for all VPC components
          inherits:
            - vpc/defaults
        # Define variables specific to this `vpc-1` component
        vars:
          name: vpc-1
          ipv4_primary_cidr_block: 10.9.0.0/18

      # Atmos component `vpc-2`
      vpc-2:
        metadata:
          # Point to the Terraform component in `components/terraform/vpc`
          component: vpc
          # Inherit the defaults for all VPC components
          inherits:
            - vpc/defaults
        # Define variables specific to this `vpc-2` component
        vars:
          name: vpc-2
          ipv4_primary_cidr_block: 10.10.0.0/18
```

## Benefits

The **Multiple Component Instances** pattern provides the following benefits:

- Separation of the code (Terraform component) from the configuration (Atmos components)

- The same Terraform component code is reused by multiple Atmos component instances with different configurations

- The defaults for the components are defined in just one place (in the catalog) making the entire
  configuration [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

- The defaults for the components are reusable across many environments by using [imports](/core-concepts/stacks/imports)

## Related Patterns

- [Component Inheritance](/design-patterns/component-inheritance)
- [Abstract Component](/design-patterns/abstract-component)
- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
- [Component Catalog Template](/design-patterns/component-catalog-template)
- [Inline Component Configuration](/design-patterns/inline-component-configuration)
- [Inline Component Customization](/design-patterns/inline-component-customization)
