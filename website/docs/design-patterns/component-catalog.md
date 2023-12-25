---
title: Component Catalog Atmos Design Pattern
sidebar_position: 5
sidebar_label: Component Catalog
description: Component Catalog Atmos Design Pattern
---

# Component Catalog

The **Component Catalog** pattern prescribes the following:

- For each Terraform component, create the same folder in `stacks/catalog` to make it symmetrical and easy to find.
  For example, the `stacks/catalog/vpc` folder should mirror the `components/terraform/vpc` folder.

- In the component's catalog folder, create `defaults.yaml` manifest with all the default values for the component.
  Define all the required Atmos sections, e.g. `metadata`, `settings`, `vars`, `env`.

- In the component's catalog folder, add other manifests for different combinations of component configurations.
  We refer to them as feature manifests. Each feature manifest should import the `defaults.yaml` file to reuse the default values and make the entire
  config DRY. For example:

  - `stacks/catalog/vpc/disabled.yaml` - component manifest with the component disabled (`vars.enabled: false`)
  - `stacks/catalog/vpc/dev.yaml` - component manifest with the settings related to the `dev` account
  - `stacks/catalog/vpc/staging.yaml` - component manifest with the settings related to `staging` account
  - `stacks/catalog/vpc/prod.yaml` - component manifest with the settings related to `prod` account
  - `stacks/catalog/vpc/ue2.yaml` - component manifest with the settings for `us-east-2` region
  - `stacks/catalog/vpc/uw2.yaml` - component manifest with the settings for `us-west-2` region
  - `stacks/catalog/vpc/feature-1.yaml` - component manifest with `feature-1` setting enabled

- After we have defined the manifests for different use-cases, we import them into different top-level stacks depending on a particular use-case.
  For example:

  - Import the `catalog/vpc/ue2.yaml` manifest into the `stacks/mixins/region/us-east-2.yaml` mixin because we need the `vpc`
    component with the `us-east-2` region-related config provisioned in the region
  - Import the `catalog/vpc/uw2.yaml` manifest into the `stacks/mixins/region/us-west-2.yaml` mixin because we need the `vpc`
    component with the `us-west-2` region-related config provisioned in the region
  - Import the `catalog/vpc/dev.yaml` manifest into the `stacks/orgs/acme/plat/dev/us-east-2.yaml` top-level stack because we need the `vpc`
    component with the dev-related config provisioned in the stack
  - Import the `catalog/vpc/prod.yaml` manifest into the `stacks/orgs/acme/plat/prod/us-east-2.yaml` top-level stack because we need the `vpc`
    component with the prod-related config provisioned in the stack
  - Import the `catalog/vpc/staging.yaml` manifest into the `stacks/orgs/acme/plat/staging/us-east-2.yaml` top-level stack because we need the `vpc`
    component with the dev-related config provisioned in the stack
  - Import the `catalog/vpc/disabled.yaml` manifest into a stack where we want the `vpc` component to be disabled (e.g. temporarily until it's needed)
  - Etc.

## Applicability

Use the **Component Catalog** pattern when:

- You have many components that are provisioned into multiple stacks with different configurations for each stack

- You need to make the components' default configurations reusable across different stacks

- You want the component catalog folders structures to mirror the Terraform components folder structure to make it easy to find and manage

- You want to keep the configurations [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   └── catalog (component-specific defaults)
   │       ├── vpc
   │       │   └── defaults.yaml
   │       │   └── disabled.yaml
   │       │   └── prod.yaml
   │       │   └── ue2.yaml
   │       │   └── uw2.yaml
   │       └── vpc-flow-logs-bucket
   │           └── defaults.yaml
   │           └── disabled.yaml
   │   # Centralized components configuration
   └── components
       └── terraform  # Terraform components (Terraform root modules)
           ├── vpc
           └── vpc-flow-logs-bucket
```

## Example

Add the following minimal configuration to `atmos.yaml` [CLI config file](/cli/configuration) :

```yaml title="atmos.yaml"
components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  name_pattern: "{stage}"
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

Add the following default configuration to the `stacks/defaults/vpc-flow-logs-bucket.yaml` manifest:

```yaml title="stacks/defaults/vpc-flow-logs-bucket.yaml"
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

Add the following default configuration to the `stacks/defaults/vpc.yaml` manifest:

```yaml title="stacks/defaults/vpc.yaml"
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
            # An array of filesystem paths (folders or individual files) to the additional modules for schema validation
            # Each path can be an absolute path or a path relative to `schemas.opa.base_path` defined in `atmos.yaml`
            # In this example, we have the additional Rego modules in `stacks/schemas/opa/catalog/constants`
            module_paths:
              - "catalog/constants"
            description: Check 'vpc' component configuration using OPA policy
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

Configure the `stacks/dev.yaml` top-level stack manifest:

```yaml title="stacks/dev.yaml"
vars:
  stage: dev

# Import the component default configurations
import:
  - defaults/vpc

components:
  terraform:
    # Customize the `vpc` component for the `dev` account
    # You can define variables or override the imported defaults
    vpc:
      vars:
        max_subnet_count: 2
        vpc_flow_logs_enabled: false
```

Configure the `stacks/staging.yaml` top-level stack manifest:

```yaml title="stacks/staging.yaml"
vars:
  stage: staging

# Import the component default configurations
import:
  - defaults/vpc-flow-logs-bucket
  - defaults/vpc

components:
  terraform:
    # Customize the `vpc` component for the `staging` account
    # You can define variables or override the imported defaults
    vpc:
      vars:
        map_public_ip_on_launch: false
        vpc_flow_logs_traffic_type: "REJECT"
```

Configure the `stacks/prod.yaml` top-level stack manifest:

```yaml title="stacks/prod.yaml"
vars:
  stage: prod

# Import the component default configurations
import:
  - defaults/vpc-flow-logs-bucket
  - defaults/vpc

components:
  terraform:
    # Customize the `vpc` component for the `prod` account
    # You can define variables or override the imported defaults
    vpc:
      vars:
        map_public_ip_on_launch: false
```

To provision the components, execute the following commands:

```shell
# `dev` stack
atmos terraform apply vpc -s dev

# `staging` stack
atmos terraform apply vpc-flow-logs-bucket -s staging
atmos terraform apply vpc -s staging

# `prod` stack
atmos terraform apply vpc-flow-logs-bucket -s prod
atmos terraform apply vpc -s prod
```

## Benefits

The **Component Catalog** pattern provides the following benefits:

- The defaults for the components are defined in just one place making the entire
  configuration [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

- The defaults for the components are reusable across many stacks

- Simple stack and component configurations

## Limitations

The **Component Catalog** pattern has the following limitations and drawbacks:

- The pattern is useful to customize components per account or region, but if you have more than one Organization, Organizational Unit (OU) or region,
  then the inline customizations would be repeated in the stack manifests, making the entire stack configuration
  not [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

- Should be used only for specific use-cases, e.g. when you use just one region, Organization or Organizational Unit (OU)

:::note

To address the limitations of the Component Catalog pattern, use the following patterns:

- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
- [Component Catalog Template](/design-patterns/component-catalog-template)

:::

## Related Patterns

- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
- [Component Catalog Template](/design-patterns/component-catalog-template)
- [Component Inheritance](/design-patterns/component-inheritance)
- [Inline Component Configuration](/design-patterns/inline-component-configuration)
- [Inline Component Customization](/design-patterns/inline-component-customization)
