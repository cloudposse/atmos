---
name: atmos-design-patterns
description: "Design patterns: stack organization, component catalogs, inheritance, configuration composition, version management, layered configuration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Design Patterns

Design patterns are proven solutions for structuring infrastructure configuration in Atmos. They address organizational complexity by providing reusable approaches for multi-account, multi-region, enterprise-grade environments.

## Pattern Progression

Most teams follow this growth path:

```
Inline Configuration (learning/prototyping)
    |
Basic Stack Organization (dev/staging/prod)
    |
Multi-Region Configuration (add regions)
    |
Organizational Hierarchy (add teams/accounts/OUs)
```

Start with the simplest pattern that meets your needs. You do not need to start with the most complex pattern -- start simple and evolve.

## Stack Organization Patterns

### Basic Stack Organization

One file per environment. Simplest setup for single-region, single-account-per-stage deployments.

```
stacks/
  catalog/
    vpc/
      defaults.yaml      # Shared component defaults
  deploy/
    dev.yaml             # Imports catalog, sets stage: dev
    staging.yaml
    prod.yaml
```

Each environment file imports shared defaults and adds environment-specific overrides:

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
```

Deploy with: `atmos terraform apply vpc -s dev`

### Multi-Region Configuration

Extends basic pattern to deploy across multiple AWS regions. Each region gets its own stack file with region-specific settings (CIDR blocks, availability zones).

```
stacks/deploy/dev/
  us-east-2.yaml          # region: us-east-2, environment: ue2
  us-west-2.yaml          # region: us-west-2, environment: uw2
```

Use `name_template: "{{.vars.environment}}-{{.vars.stage}}"` in `atmos.yaml` to generate stack names like `ue2-dev`.

### Organizational Hierarchy Configuration

Enterprise pattern for multiple organizations, OUs/tenants, and accounts. Uses `_defaults.yaml` files at each hierarchy level to create inheritance chains.

```
stacks/orgs/acme/
  _defaults.yaml                    # namespace: acme
  plat/
    _defaults.yaml                  # tenant: plat (imports org defaults)
    dev/
      _defaults.yaml                # stage: dev (imports tenant defaults)
      network.yaml                  # layer: network (imports stage defaults + catalog)
      data.yaml                     # layer: data
    prod/
      _defaults.yaml
      network.yaml
      data.yaml
      compute.yaml
```

Import chain: `network.yaml -> prod/_defaults.yaml -> plat/_defaults.yaml -> acme/_defaults.yaml`

Configure `atmos.yaml`:

```yaml
stacks:
  included_paths: ["orgs/**/*"]
  excluded_paths: ["**/_defaults.yaml"]
  name_template: "{{.vars.tenant}}-{{.vars.stage}}"
```

### Layered Stack Configuration

Groups components by infrastructure function (network, data, compute). Each layer imports its relevant catalog defaults. Different teams can own different layers. Environments import only the layers they need.

```yaml
# stacks/layers/network.yaml
import:
  - catalog/vpc/defaults
# stacks/layers/data.yaml
import:
  - catalog/rds/defaults
```

```yaml
# stacks/deploy/prod.yaml
import:
  - layers/network
  - layers/data
  - layers/compute
vars:
  stage: prod
```

## The _defaults.yaml Convention

A naming convention (not an Atmos feature) for organizing hierarchical defaults:

- Underscore prefix ensures files sort to top of directory listings
- Excluded from stack discovery via `excluded_paths: ["**/_defaults.yaml"]`
- Must be explicitly imported -- Atmos does NOT auto-import them
- Creates clear inheritance chains when each level imports its parent

Best practices: keep to 3-4 levels maximum, document import chains, use base-relative paths (resolved from `stacks.base_path`).

## Configuration Catalog Patterns

### Basic Catalog

Mirror your component directory in `stacks/catalog/`. Each component gets a `defaults.yaml` with shared configuration.

```
stacks/catalog/
  vpc/
    defaults.yaml          # Base defaults for all VPCs
    dev.yaml               # Dev-specific overrides
    prod.yaml              # Prod-specific overrides
    ue2.yaml               # Region-specific (imports defaults)
  s3-bucket/
    defaults.yaml
    public.yaml            # Archetype: public website bucket
    logging.yaml           # Archetype: log storage bucket
    artifacts.yaml         # Archetype: CI/CD artifacts
```

### Mixins

Reusable configuration fragments that encapsulate settings applied consistently across stacks. Two scopes:

**Global mixins** (`stacks/mixins/`) -- region defaults, stage defaults, tenant defaults:

```yaml
# stacks/mixins/region/us-east-2.yaml
vars:
  region: us-east-2
  environment: ue2
components:
  terraform:
    vpc:
      vars:
        availability_zones: [us-east-2a, us-east-2b, us-east-2c]
```

**Catalog mixins** (`stacks/catalog/<component>/mixins/`) -- feature flags, versions:

```yaml
# stacks/catalog/eks/mixins/1.28.yaml
components:
  terraform:
    eks/cluster:
      vars:
        cluster_kubernetes_version: "1.28"
        addons:
          vpc-cni:
            addon_version: "v1.14.1-eksbuild.1"
```

Import order matters -- later imports override earlier ones. Order from general to specific:

```yaml
import:
  - catalog/vpc/defaults         # 1. Component defaults
  - catalog/vpc/mixins/multi-az  # 2. Feature flags
  - mixins/region/us-east-2      # 3. Region settings
  - mixins/stage/prod            # 4. Stage settings (most specific)
```

### Component Archetypes

Pre-configured variants for specific use cases. Define abstract base components with `metadata.type: abstract`, then create archetypes that inherit from the base with use-case-specific settings.

### Catalog Templates

Use Go templates in imports to dynamically generate component instances. Import the same template multiple times with different `context` values:

```yaml
import:
  - path: catalog/eks/iam-role/defaults.tmpl
    context:
      app_name: "auth"
      service_account_name: "auth"
      service_account_namespace: "auth"
```

Use sparingly -- the templating engine is powerful but can reduce maintainability.

## Inheritance Patterns

### Component Inheritance

A component inherits configuration from a base using `metadata.inherits`:

```yaml
components:
  terraform:
    vpc:
      metadata:
        component: vpc
        inherits:
          - vpc/defaults    # Inherit all vars, then override
      vars:
        max_subnet_count: 2
```

Inheritance order: base component -> inherited components (in order) -> inline vars.

### Abstract Components

Mark components as non-deployable blueprints with `metadata.type: abstract`. Prevents accidental `atmos terraform apply` on base configurations. Components inheriting from abstract bases are deployable by default.

```yaml
# In catalog
vpc/defaults:
  metadata:
    type: abstract
  vars:
    enabled: true
    nat_gateway_enabled: true
```

### Multiple Component Instances

Deploy multiple instances of the same Terraform component in one environment by defining multiple Atmos components pointing to the same `metadata.component`:

```yaml
components:
  terraform:
    vpc/1:
      metadata:
        component: vpc
        inherits: [vpc/defaults]
      vars:
        name: vpc-1
        ipv4_primary_cidr_block: 10.9.0.0/18
    vpc/2:
      metadata:
        component: vpc
        inherits: [vpc/defaults]
      vars:
        name: vpc-2
        ipv4_primary_cidr_block: 10.10.0.0/18
```

### Multiple Inheritance

Inherit from multiple abstract bases to compose configuration from independent concerns:

```yaml
rds:
  metadata:
    component: rds
    inherits:
      - base/defaults     # Applied first
      - base/logging      # Applied second
      - base/production   # Applied last, highest precedence
```

Merge behavior: scalars -- later wins; maps -- deep merged; lists -- later replaces entirely.

## Configuration Composition

### Inline Configuration

Define components directly in stack manifests. Use for prototyping, single-environment deployments, or components unique to one stack.

### Partial Component Configuration

Split a component's configuration across multiple files imported into the same stack. Useful for independently managing parts of complex configurations (e.g., EKS cluster defaults + Kubernetes version mixin).

### Component Overrides

Apply configuration to a subset of components without affecting others using the `overrides` section. Overrides are file-scoped and do not get inherited.

```yaml
# stacks/teams/platform.yaml
import:
  - catalog/vpc/defaults
  - catalog/eks/defaults
terraform:
  overrides:
    vars:
      tags:
        Team: Platform     # Only applies to vpc and eks, not other teams' components
```

### DRY Configuration with Locals

File-scoped variables that reduce repetition within a single stack file:

```yaml
locals:
  prefix: "{{ .locals.namespace }}-{{ .locals.environment }}"
components:
  terraform:
    vpc:
      vars:
        name: "{{ .locals.prefix }}-vpc"
```

Locals are not inherited across imports. Use `vars` or `settings` for cross-file values.

## Version Management Patterns

### Continuous Version Deployment (Recommended)

Trunk-based strategy where all environments reference the same component path and converge through progressive automated rollout. Simplest approach with strongest feedback loops.

### Folder-Based Versioning

Components organized in explicit folders (`vpc/v1/`, `vpc/v2/`). Environments reference specific version folders. Use `metadata.name` for stable workspace keys across version upgrades.

```yaml
vpc:
  metadata:
    name: vpc            # Stable identity (workspace key stays same)
    component: vpc/v2    # Version can change freely
```

### Release Tracks/Channels

Named channels (`alpha/vpc`, `beta/vpc`, `prod/vpc`) that environments subscribe to. Promote tracks instead of individual environment pins. Use label-based versioning schemes (maturity levels, environment names).

### Strict Version Pinning

Explicit SemVer versions (`vpc/1.2.3`). Works with vendoring from external sources. Use number-based versioning schemes. Higher operational overhead but strongest audit trail.

### Source-Based Version Pinning

Per-environment version control using the `source` field in stack configuration. Just-in-time vendoring without managing separate vendor manifests.

```yaml
vpc:
  source:
    uri: github.com/org/components//modules/vpc
    version: 1.450.0
```

### Vendoring Component Versions

Automate copying components from external sources with `vendor.yaml` and `atmos vendor pull`. Provides local control, audit trail, and searchable codebase. Complements any deployment strategy.

### Git Flow: Branches as Channels

Branch-based alternative where long-lived branches map to release channels. Promotions happen via merges. Best for teams already practicing Git Flow.

## When to Use Which Pattern

| Scenario | Recommended Patterns |
|----------|---------------------|
| Learning / prototyping | Inline Configuration |
| Single region, few environments | Basic Stack Organization + Catalog |
| Multi-region deployment | Multi-Region Configuration + Mixins |
| Enterprise multi-account | Organizational Hierarchy + Layered + Catalog |
| Multiple instances of same component | Multiple Component Instances + Abstract Components |
| Many teams sharing infrastructure | Layered Configuration + Component Overrides |
| Complex component configuration | Partial Component Configuration + Mixins |
| External component dependencies | Vendoring + Folder-Based Versioning |
| Rapid iteration / trunk-based | Continuous Version Deployment |
| Strict compliance / audit | Strict Version Pinning + Vendoring |

## Anti-Patterns to Avoid

- **Vendoring multiple versions to the same path** -- last one overwrites all previous
- **Including version in `workspace_key_prefix`** -- breaks state continuity during upgrades
- **Mixing trunk-based and Git Flow** -- creates team confusion about promotion paths
- **Over-pinning environments** -- creates high operational overhead and weak feedback loops
- **Inconsistent path conventions** -- pick `{track}/{component}` or `{component}/{track}` and stick with it
- **Assuming `_defaults.yaml` auto-imports** -- they must always be explicitly imported
- **Too many inheritance levels** -- keep to 3-4 levels maximum for maintainability

## References

For detailed examples and directory layouts, see:
- [references/stack-organization.md](references/stack-organization.md) -- Stack organization patterns with directory layouts
- [references/version-management.md](references/version-management.md) -- Versioning strategies with implementation details
