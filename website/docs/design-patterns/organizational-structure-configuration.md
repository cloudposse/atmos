---
title: Organizational Structure Configuration Atmos Design Pattern
sidebar_position: 4
sidebar_label: Organizational Structure Configuration
description: Organizational Structure Configuration Atmos Design Pattern
---

# Organizational Structure Configuration

The **Organizational Structure Configuration** pattern describes core concepts and best practices to structure and organize components
and stacks to design for organizational complexity and provision multi-account enterprise-grade environments.

The pattern is used to model multi-region infrastructures for organizations with multiple OUs/departments/tenants and multiple accounts.

## Applicability

Use the **Organizational Structure Configuration** pattern when:

- You have one or more organizations with multiple OUs/departments/tenants

- Each OU/department/tenant has multiple accounts

- You want to provision the infrastructure into many regions

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   ├── mixins
   │   │   ├── region
   │   │   │   ├── global-region.yaml
   │   │   │   ├── us-east-2.yaml
   │   │   │   └── us-west-2.yaml
   │   │   └── stage
   │   │        ├── audit.yaml
   │   │        ├── automation.yaml
   │   │        ├── identity.yaml
   │   │        ├── root.yaml
   │   │        ├── dev.yaml
   │   │        ├── staging.yaml
   │   │        └── prod.yaml
   │   └── orgs
   │       ├── org1
   │       │   ├── core
   │       │   ├── plat
   │  
   │   # Centralized components configuration
   ├── components
   │   └── terraform  # Terraform components (Terraform root modules)
   │       ├── vpc
   │       ├── vpc-flow-logs-bucket
   │       ├── < other components >
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
    # Tell Atmos that all the `_defaults.yaml` files are not top-level stack manifests
    - "**/_defaults.yaml"
  name_pattern: "{tenant}-{environment}-{stage}"

schemas:
  jsonschema:
    base_path: "stacks/schemas/jsonschema"
  opa:
    base_path: "stacks/schemas/opa"
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

The Organizational Structure Configuration pattern provides the following benefits:

- The defaults for the components are defined in just one place making the entire configuration DRY

- The defaults for the components are reusable across many stacks

- Simple stack and component configurations

## Limitations

The Organizational Structure Configuration pattern has the following limitations and drawbacks:

- The pattern is useful to customize components per account or region, but if you have more than one Organization, Organizational Unit (OU) or region,
  then the inline customizations would be repeated in the stack manifests, making the entire stack configuration not DRY

- Should be used only for specific use-cases, e.g. when you use just one region, Organization or Organizational Unit (OU)

:::note

To address the limitations of the Organizational Structure Configuration pattern, use the following patterns:

- [Organizational Structure Configuration](/design-patterns/organizational-structure-configuration)
- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)

:::

## Related Patterns

- [Inline Component Configuration](/design-patterns/inline-component-configuration)
- [Organizational Structure Configuration](/design-patterns/organizational-structure-configuration)
- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
- [Component Catalog Template](/design-patterns/component-catalog-template)
- [Component Inheritance](/design-patterns/component-inheritance)
- [Partial Component Configuration](/design-patterns/partial-component-configuration)
