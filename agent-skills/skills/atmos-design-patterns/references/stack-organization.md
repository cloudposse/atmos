# Stack Organization Patterns -- Detailed Reference

This reference provides complete directory layouts and configuration examples for each stack organization pattern.

## Basic Stack Organization

### When to Use

- Single AWS account per environment (dev, staging, prod)
- Deploying to one region
- Simplest possible setup to start

### Directory Layout

```text
project/
  atmos.yaml
  stacks/
    catalog/
      vpc/
        defaults.yaml
      vpc-flow-logs-bucket/
        defaults.yaml
    deploy/
      dev.yaml
      staging.yaml
      prod.yaml
  components/
    terraform/
      vpc/
      vpc-flow-logs-bucket/
```

### atmos.yaml Configuration

```yaml
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{.vars.stage}}"
```

### Catalog Defaults

```yaml
# stacks/catalog/vpc/defaults.yaml
components:
  terraform:
    vpc:
      vars:
        enabled: true
        nat_gateway_enabled: true
        max_subnet_count: 3
```

### Environment Stack Files

```yaml
# stacks/deploy/dev.yaml
import:
  - catalog/vpc/defaults
vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        nat_gateway_enabled: false
        max_subnet_count: 2
```

```yaml
# stacks/deploy/prod.yaml
import:
  - catalog/vpc/defaults
vars:
  stage: prod
components:
  terraform:
    vpc:
      vars:
        map_public_ip_on_launch: false
```

### Deploy Commands

```shell
atmos terraform apply vpc -s dev
atmos terraform apply vpc -s staging
atmos terraform apply vpc -s prod
```

## Multi-Region Configuration

### When to Use

- Deploying to multiple AWS regions for DR, latency, or compliance
- Region-specific resources with shared common configuration

### Directory Layout

```text
project/
  atmos.yaml
  stacks/
    catalog/
      vpc-flow-logs-bucket/
        defaults.yaml
      vpc/
        defaults.yaml
    deploy/
      dev/
        us-east-2.yaml
        us-west-2.yaml
      staging/
        us-east-2.yaml
        us-west-2.yaml
      prod/
        us-east-2.yaml
        us-west-2.yaml
  components/
    terraform/
      vpc/
      vpc-flow-logs-bucket/
```

### atmos.yaml Configuration

```yaml
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{.vars.environment}}-{{.vars.stage}}"
```

### Region Stack Files

```yaml
# stacks/deploy/dev/us-east-2.yaml
import:
  - catalog/vpc-flow-logs-bucket/defaults
  - catalog/vpc/defaults
vars:
  region: us-east-2
  environment: ue2
  stage: dev
components:
  terraform:
    vpc:
      vars:
        ipv4_primary_cidr_block: "10.10.0.0/16"
        availability_zones:
          - us-east-2a
          - us-east-2b
          - us-east-2c
```

### Reducing Duplication with Region Mixins

Extract region-specific settings into reusable mixins:

```yaml
# stacks/mixins/region/us-east-2.yaml
vars:
  region: us-east-2
  environment: ue2
components:
  terraform:
    vpc:
      vars:
        availability_zones:
          - us-east-2a
          - us-east-2b
          - us-east-2c
```

Then simplify the stack file:

```yaml
# stacks/deploy/dev/us-east-2.yaml
import:
  - catalog/vpc-flow-logs-bucket/defaults
  - catalog/vpc/defaults
  - mixins/region/us-east-2
vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        ipv4_primary_cidr_block: "10.10.0.0/16"
```

### Adding a New Region

Create a new stack file importing shared defaults and region-specific settings:

```yaml
# stacks/deploy/dev/eu-west-1.yaml
import:
  - catalog/vpc-flow-logs-bucket/defaults
  - catalog/vpc/defaults
vars:
  region: eu-west-1
  environment: ew1
  stage: dev
components:
  terraform:
    vpc:
      vars:
        ipv4_primary_cidr_block: "10.30.0.0/16"
        availability_zones:
          - eu-west-1a
          - eu-west-1b
          - eu-west-1c
```

### Environment Abbreviations

The `environment` variable is typically set to an abbreviation of the region:
- `ue2` for `us-east-2`
- `uw2` for `us-west-2`
- `ew1` for `eu-west-1`

This makes stack names shorter: `ue2-dev` instead of `us-east-2-dev`.

## Organizational Hierarchy Configuration

### When to Use

- Multiple organizations, OUs/departments/tenants
- Each OU has multiple accounts (dev, staging, prod)
- Configuration at different levels (org, tenant, account)

### Context Variables

| Variable | Purpose | Example |
|----------|---------|---------|
| `namespace` | Organization | `acme` |
| `tenant` | OU/department/team | `core`, `plat` |
| `stage` | Account/deployment stage | `dev`, `staging`, `prod` |
| `environment` | Region abbreviation | `ue2`, `uw2` |

### Directory Layout

```text
project/
  atmos.yaml
  stacks/
    catalog/
      vpc/
        defaults.yaml
      rds/
        defaults.yaml
      eks/
        defaults.yaml
    orgs/
      acme/
        _defaults.yaml                     # namespace: acme
        core/
          _defaults.yaml                   # tenant: core
          audit/
            _defaults.yaml                 # stage: audit
            network.yaml
        plat/
          _defaults.yaml                   # tenant: plat
          dev/
            _defaults.yaml                 # stage: dev
            network.yaml
            data.yaml
          staging/
            _defaults.yaml
            network.yaml
            data.yaml
          prod/
            _defaults.yaml
            network.yaml
            data.yaml
            compute.yaml
  components/
    terraform/
      vpc/
      rds/
      eks/
```

### atmos.yaml Configuration

```yaml
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "orgs/**/*"
  excluded_paths:
    - "**/_defaults.yaml"
  name_template: "{{.vars.tenant}}-{{.vars.stage}}"
```

### Hierarchy Defaults Chain

```yaml
# stacks/orgs/acme/_defaults.yaml
vars:
  namespace: acme
terraform:
  vars:
    tags:
      Organization: acme
```

```yaml
# stacks/orgs/acme/plat/_defaults.yaml
import:
  - orgs/acme/_defaults
vars:
  tenant: plat
```

```yaml
# stacks/orgs/acme/plat/dev/_defaults.yaml
import:
  - orgs/acme/plat/_defaults
vars:
  stage: dev
```

```yaml
# stacks/orgs/acme/plat/dev/network.yaml
import:
  - orgs/acme/plat/dev/_defaults
  - catalog/vpc/defaults
vars:
  layer: network
```

### Import Chain Visualization

```text
network.yaml
  -> dev/_defaults.yaml (stage: dev)
    -> plat/_defaults.yaml (tenant: plat)
      -> acme/_defaults.yaml (namespace: acme)
  -> catalog/vpc/defaults.yaml (component defaults)
```

### Deploy Commands

```shell
atmos terraform apply vpc -s plat-dev-network
atmos terraform apply rds -s plat-prod-data
atmos terraform apply eks -s plat-prod-compute
```

## Layered Stack Configuration

### When to Use

- Many components that group by function (networking, data, compute)
- Different teams own different layers
- Need to import only layers needed for specific environments

### Directory Layout

```text
stacks/
  catalog/
    vpc/
      defaults.yaml
    rds/
      defaults.yaml
    eks/
      defaults.yaml
  layers/
    network.yaml
    data.yaml
    compute.yaml
    observability.yaml
    security.yaml
  deploy/
    dev.yaml
    prod.yaml
```

### Layer Definitions

```yaml
# stacks/layers/network.yaml
import:
  - catalog/vpc/defaults
```

```yaml
# stacks/layers/data.yaml
import:
  - catalog/rds/defaults
```

### Stack Importing Layers

```yaml
# stacks/deploy/dev.yaml
import:
  - layers/network
  - layers/compute
  # Skip observability in dev
vars:
  stage: dev
```

```yaml
# stacks/deploy/prod.yaml
import:
  - layers/network
  - layers/security
  - layers/data
  - layers/compute
  - layers/observability
vars:
  stage: prod
```

### Common Layer Examples

| Layer | Components | Purpose |
|-------|-----------|---------|
| `network` | VPC, subnets, NAT, VPN | Foundation networking |
| `security` | WAF, security groups, KMS | Security controls |
| `data` | RDS, ElastiCache, S3 | Data storage |
| `compute` | EKS, ECS, EC2 | Application runtime |
| `observability` | CloudWatch, Datadog | Monitoring and logging |

## Mixin Patterns

### Global Mixin Types

| Type | Location | Contains |
|------|----------|----------|
| Region | `stacks/mixins/region/` | Region name, AZs, environment abbreviation |
| Stage | `stacks/mixins/stage/` | Stage name, stage-specific defaults |
| Tenant | `stacks/mixins/tenant/` | Team/OU name, team-specific settings |

### Catalog Mixin Types

| Type | Location | Contains |
|------|----------|----------|
| Feature flags | `stacks/catalog/<component>/mixins/` | Enable/disable features |
| Versions | `stacks/catalog/<component>/mixins/` | Component-specific version settings |

### Mixin Directory Structure

```text
stacks/
  catalog/
    vpc/
      defaults.yaml
      mixins/
        multi-az.yaml
        nat-gateway.yaml
    eks/
      defaults.yaml
      mixins/
        1.27.yaml
        1.28.yaml
  mixins/
    region/
      us-east-2.yaml
      us-west-2.yaml
    stage/
      dev.yaml
      staging.yaml
      prod.yaml
  deploy/
    us-east-2/
      dev.yaml
      prod.yaml
```

## Combining Patterns

Enterprise-scale deployments typically combine multiple patterns:

1. **Organizational Hierarchy** for the directory structure
2. **_defaults.yaml Convention** for inheritance at each level
3. **Configuration Catalog** for reusable component defaults
4. **Mixins** for region and stage-specific settings
5. **Layered Configuration** within each stage folder
6. **Component Inheritance** for abstract base components
7. **Partial Component Configuration** for complex components like EKS

The combination produces a DRY, maintainable structure where adding a new account, region, or component requires minimal changes.
