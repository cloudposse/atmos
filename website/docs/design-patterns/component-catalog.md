---
title: Component Catalog Atmos Design Pattern
sidebar_position: 5
sidebar_label: Component Catalog
description: Component Catalog Atmos Design Pattern
---

# Component Catalog

The **Component Catalog** pattern prescribes the following:

- For each Terraform component, create the same folder structure in `stacks/catalog` so everything is symmetrical and easy to find.
  For example, the `stacks/catalog/eks/echo-server` folder should mirror the `components/terraform/eks/echo-server` folder

- In the catalog, create `defaults.yaml` config with all the default values for the component.
  Define all Atmos sections, e.g. `metadata`, `settings`, `vars`. Enable the component by
  setting `vars.enabled: true`. Enable the Spacelift stack for the component by setting `settings.spacelift.workspace_enabled: true`

- In the catalog, add other files for different combinations of component configuration.
  We call them feature config files. Each feature config file should import the `defaults.yaml`
  file to reuse the default values and make the entire config DRY. For example:

  - `disabled.yaml` - component config with the component disabled (`vars.enabled: false`) and Spacelift stack for the component
    disabled (`settings.spacelift.workspace_enabled: false`)
  - `dev.yaml` - component config with the settings related to `dev` (imports `defaults.yaml`)
  - `dev-disabled.yaml` - component config that imports `dev.yaml` and disables the component and Spacelift stack
  - `staging.yaml` - component config with the settings related to `staging` (imports `defaults.yaml`)
  - `staging-disabled.yaml` - component config that imports `staging.yaml` and disables the component and Spacelift stack
  - `prod.yaml` - component config with the settings related to `prod` (imports `defaults.yaml`)
  - `prod-disabled.yaml` - component config that imports `prod.yaml` and disables the component and Spacelift stack
  - `feature-1.yaml` - component config with `feature-1` enabled (imports `defaults.yaml`)

- Now that we have the feature-defining catalog of configurations for different use-cases,
  we can import those catalog files into different top-level stacks depending on a particular use-case. For example:

  - Import `catalog/eks/echo-server/dev` into the `stacks/orgs/dev/us-east-1.yaml`stack because we need the `eks/echo-server`
    with the dev-related config provisioned in the stack
  - Import `catalog/eks/echo-server/staging` into the `stacks/orgs/staging/us-east-1.yaml` stack because we need the `eks/echo-server` with the
    staging-related config provisioned in the stack
  - Import `catalog/eks/echo-server/disabled` into a stack where we want the component and Spacelift stack to be
    disabled (e.g. temporarily until it's needed)
  - Etc.

- The design pattern simplifies all stack configurations, makes them DRY and easy to understand based on specific use-cases

The **Component Catalog** pattern is used when the defaults for the [components](/core-concepts/components) in
a [stack](/core-concepts/stacks)
are configured in default/base manifests, the manifests are [imported](/core-concepts/stacks/imports) into the top-level stacks, and the components
are customized inline in each top-level stack overriding the configuration for each environment (OU, account, region).

## Applicability

Use the **Component Catalog** pattern when:

- You have components that are provisioned in multiple stacks (e.g. `dev`, `staging`, `prod` accounts) with different configurations for each stack

- You need to make the components' default/baseline configurations reusable across different stacks

- You want to keep the configurations [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   └── catalog (component-specific defaults)
   │       ├── vpc
   │       │   └── defaults.yaml
   │       └── vpc-flow-logs-bucket
   │           └── defaults.yaml
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
