---
title: Organizational Structure Configuration Atmos Design Pattern
sidebar_position: 4
sidebar_label: Organizational Structure Configuration
description: Organizational Structure Configuration Atmos Design Pattern
---

# Organizational Structure Configuration

The **Organizational Structure Configuration** pattern describes core concepts and best practices to structure and organize components
and stacks to design for organizational complexity and provision multi-account enterprise-grade environments.

The pattern is used to model multi-region infrastructures for organizations with multiple organizational units/departments/tenants and multiple
accounts.

## Applicability

Use the **Organizational Structure Configuration** pattern when:

- You have one or more organizations with multiple OUs/departments/tenants

- Each OU/department/tenant has multiple accounts

- You want to provision the infrastructure into many regions

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   ├── catalog (component-specific defaults)
   │   │   ├── vpc-flow-logs-bucket
   │   │   │   └── defaults.yaml
   │   │   └── vpc
   │   │       └── defaults.yaml
   │   ├── mixins
   │   │   ├── tenant  (tenant-specific defaults)
   │   │   │   ├── core.yaml
   │   │   │   └── plat.yaml
   │   │   ├── region  (region-specific defaults)
   │   │   │   ├── global-region.yaml
   │   │   │   ├── us-east-2.yaml
   │   │   │   └── us-west-2.yaml
   │   │   └── stage  (stage-specific defaults)
   │   │       ├── audit.yaml
   │   │       ├── automation.yaml
   │   │       ├── identity.yaml
   │   │       ├── root.yaml
   │   │       ├── dev.yaml
   │   │       ├── staging.yaml
   │   │       └── prod.yaml
   │   └── orgs  (Organizations)
   │       ├── org1
   │       │   ├── _defaults.yaml
   │       │   ├── core  ('core' OU/tenant)
   │       │   │   ├── _defaults.yaml
   │       │   │   ├── audit
   │       │   │   │   ├── _defaults.yaml
   │       │   │   │   ├── global-region.yaml
   │       │   │   │   ├── us-east-2.yaml
   │       │   │   │   └── us-west-2.yaml
   │       │   │   ├── automation
   │       │   │   │   ├── _defaults.yaml
   │       │   │   │   ├── global-region.yaml
   │       │   │   │   ├── us-east-2.yaml
   │       │   │   │   └── us-west-2.yaml
   │       │   │   ├── identity
   │       │   │   │   ├── _defaults.yaml
   │       │   │   │   ├── global-region.yaml
   │       │   │   │   ├── us-east-2.yaml
   │       │   │   │   └── us-west-2.yaml
   │       │   │   └── root
   │       │   │       ├── _defaults.yaml
   │       │   │       ├── global-region.yaml
   │       │   │       ├── us-east-2.yaml
   │       │   │       └── us-west-2.yaml
   │       │   └── plat ('plat' OU/tenant)
   │       │       ├── _defaults.yaml
   │       │       ├── dev
   │       │       │   ├── _defaults.yaml
   │       │       │   ├── global-region.yaml
   │       │       │   ├── us-east-2.yaml
   │       │       │   └── us-west-2.yaml
   │       │       ├── staging
   │       │       │   ├── _defaults.yaml
   │       │       │   ├── global-region.yaml
   │       │       │   ├── us-east-2.yaml
   │       │       │   └── us-west-2.yaml
   │       │       └── prod
   │       │           ├── _defaults.yaml
   │       │           ├── global-region.yaml
   │       │           ├── us-east-2.yaml
   │       │           └── us-west-2.yaml
   │       └── org2
   │           ├── _defaults.yaml
   │           ├── core  ('core' OU/tenant)
   │           │   ├── _defaults.yaml
   │           │   ├── audit
   │           │   ├── automation
   │           │   ├── identity
   │           │   └── root
   │           └── plat  ('plat' OU/tenant)
   │               ├── _defaults.yaml
   │               ├── dev
   │               ├── staging
   │               └── prod
   │  
   │   # Centralized components configuration
   └── components
       └── terraform  # Terraform components (Terraform root modules)
           ├── account
           ├── vpc
           ├── vpc-flow-logs-bucket
           ├── < other components >
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
  # If you are using multiple Organizations (namespaces), use the following `name_pattern`:
  name_pattern: "{namespace}-{tenant}-{environment}-{stage}"
  # If you are using a single Organization (namespace), use the following `name_pattern`:
  # name_pattern: "{tenant}-{environment}-{stage}"

schemas:
  jsonschema:
    base_path: "stacks/schemas/jsonschema"
  opa:
    base_path: "stacks/schemas/opa"
  atmos:
    manifest: "stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
```

### Configure Component Catalog

Add the following default configuration to the `stacks/catalog/vpc-flow-logs-bucket/defaults.yaml` manifest:

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
        vpc_flow_logs_enabled: true
        vpc_flow_logs_traffic_type: "ALL"
        vpc_flow_logs_log_destination_type: "s3"
```

### Configure OU/Tenant Manifests

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

### Configure Region Manifests

In `stacks/mixins/region/us-east-2.yaml`, add the following config:

```yaml title="stacks/mixins/region/us-east-2.yaml"
import:
  # All accounts (stages) in `us-east-2` region will have the `vpc-flow-logs-bucket` component
  - catalog/vpc-flow-logs-bucket/defaults
  # All accounts (stages) in `us-east-2` region will have the `vpc` component
  - catalog/vpc/defaults

vars:
  region: us-east-2
  environment: ue2

# Other defaults for the `us-east-2` region
```

In `stacks/mixins/region/us-west-2.yaml`, add the following config:

```yaml title="stacks/mixins/region/us-west-2.yaml"
import:
  # All accounts (stages) in `us-west-2` region will have the `vpc-flow-logs-bucket` component
  - catalog/vpc-flow-logs-bucket/defaults
  # All accounts (stages) in `us-east-2` region will have the `vpc` component
  - catalog/vpc/defaults

vars:
  region: us-west-2
  environment: uw2

# Other defaults for the `us-west-2` region
```

### Configure Stage/Account Manifests

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

### Configure Organization Defaults

In `stacks/orgs/org1/_defaults.yaml`, add the following config:

```yaml title="stacks/orgs/org1/_defaults.yaml"
vars:
  namespace: org1
```

The file defines the context variable `namespace` for the entire `org1` Organization.

In `stacks/orgs/org2/_defaults.yaml`, add the following config:

```yaml title="stacks/orgs/org2/_defaults.yaml"
vars:
  namespace: org2
```

The file defines the context variable `namespace` for the entire `org2` Organization.

### Configure Tenant/OU Defaults

In `stacks/orgs/org1/core/_defaults.yaml`, add the following config for the `org1` Organization and `core` OU (tenant):

```yaml title="stacks/orgs/org1/core/_defaults.yaml"
import:
  - orgs/org1/_defaults
  - mixins/tenant/core
```

In `stacks/orgs/org1/plat/_defaults.yaml`, add the following config for the `org1` Organization and `plat` OU (tenant):

```yaml title="stacks/orgs/org1/plat/_defaults.yaml"
import:
  - orgs/org1/_defaults
  - mixins/tenant/plat
```

In `stacks/orgs/org2/core/_defaults.yaml`, add the following config for the `org2` Organization and `core` OU (tenant):

```yaml title="stacks/orgs/org2/core/_defaults.yaml"
import:
  - orgs/org2/_defaults
  - mixins/tenant/core
```

In `stacks/orgs/org2/plat/_defaults.yaml`, add the following config for the `org2` Organization and `plat` OU (tenant):

```yaml title="stacks/orgs/org2/plat/_defaults.yaml"
import:
  - orgs/org2/_defaults
  - mixins/tenant/plat
```

### Configure Stage/Account Defaults

In `stacks/orgs/org1/plat/dev/_defaults.yaml`, add the following config for the `org1` Organization, `plat` tenant, `dev` account:

```yaml title="stacks/orgs/org1/plat/dev/_defaults.yaml"
import:
  - orgs/org1/plat/_defaults
  - mixins/stage/dev
```

In `stacks/orgs/org2/plat/dev/_defaults.yaml`, add the following config for the `org2` Organization, `plat` tenant, `dev` account:

```yaml title="stacks/orgs/org2/plat/dev/_defaults.yaml"
import:
  - orgs/org2/plat/_defaults
  - mixins/stage/dev
```

In `stacks/orgs/org1/plat/prod/_defaults.yaml`, add the following config for the `org1` Organization, `plat` tenant, `prod` account:

```yaml title="stacks/orgs/org1/plat/prod/_defaults.yaml"
import:
  - orgs/org1/plat/_defaults
  - mixins/stage/prod
```

In `stacks/orgs/org2/plat/prod/_defaults.yaml`, add the following config for the `org2` Organization, `plat` tenant, `prod` account:

```yaml title="stacks/orgs/org2/plat/prod/_defaults.yaml"
import:
  - orgs/org2/plat/_defaults
  - mixins/stage/prod
```

Similarly, configure the defaults for the other accounts in the `core` and `plat` tenants in the `org1` and `org2` Organizations.

### Configure Top-Level Stack Manifests

### Provision Atmos Components into the Stacks

To provision the components, execute the following commands:

```shell
# `dev` account, `us-east-2` region
atmos terraform apply vpc-flow-logs-bucket -s plat-ue2-dev
atmos terraform apply vpc -s plat-ue2-dev

# `dev` account, `us-west-2` region
atmos terraform apply vpc-flow-logs-bucket -s plat-uw2-dev
atmos terraform apply vpc -s plat-uw2-dev

# `staging` account, `us-east-2` region
atmos terraform apply vpc-flow-logs-bucket -s plat-ue2-staging
atmos terraform apply vpc -s plat-ue2-staging

# `staging` account, `us-west-2` region
atmos terraform apply vpc-flow-logs-bucket -s plat-uw2-staging
atmos terraform apply vpc -s plat-uw2-staging

# `prod` account, `us-east-2` region
atmos terraform apply vpc-flow-logs-bucket -s plat-ue2-prod
atmos terraform apply vpc -s plat-ue2-prod

# `prod` account, `us-west-2` region
atmos terraform apply vpc-flow-logs-bucket -s plat-uw2-prod
atmos terraform apply vpc -s plat-uw2-prod
```

## Benefits

The Organizational Structure Configuration pattern provides the following benefits:

- The defaults for the components are defined in just one place making the entire
  configuration [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

- The defaults for the components are reusable across many stacks

- Simple stack and component configurations

## Limitations

The Organizational Structure Configuration pattern has the following limitations and drawbacks:

- The pattern is useful to customize components per account or region, but if you have more than one Organization, Organizational Unit (OU) or region,
  then the inline customizations would be repeated in the stack manifests, making the entire stack configuration
  not [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

- Should be used only for specific use-cases, e.g. when you use just one region, Organization or Organizational Unit (OU)

<br/>

:::note

To address the limitations of the Organizational Structure Configuration pattern, use the following patterns:

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
