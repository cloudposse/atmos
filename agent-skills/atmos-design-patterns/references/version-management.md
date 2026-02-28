# Version Management Patterns -- Detailed Reference

This reference covers versioning strategies, implementation details, and decision guidance for managing component versions in Atmos.

## Core Concepts

### Deployment vs Release

- **Deployment**: Declaring that an environment should converge to a specific version (target state).
- **Release**: Actually updating the environment to that version (applying the change).

### Versioning Schemes

Two broad categories of naming conventions:

**Number-Based (Fixed Points)** -- immutable version identifiers:
- **SemVer** (`1.2.3`) -- MAJOR.MINOR.PATCH, communicates change impact
- **CalVer** (`2024.10.1`) -- date-based, temporal tracking
- **Sequential** (`v1`, `v2`) -- simple incrementing numbers
- **Major/Minor** (`1.0`, `2.0`) -- simplified SemVer

**Label-Based (Moving Targets)** -- stage identifiers that evolve:
- **Maturity Levels** (`alpha`, `beta`, `stable`) -- stability indicators
- **Environment Names** (`dev`, `staging`, `prod`) -- deployment stage alignment

| Pattern | Best Versioning Scheme |
|---------|----------------------|
| Strict Version Pinning | SemVer, CalVer, Sequential |
| Release Tracks/Channels | Maturity Levels, Environment Names |
| Git Flow | Environment Names (branches ARE environments) |
| Folder-Based Versioning | Simple naming, Sequential |

## Continuous Version Deployment

The recommended trunk-based strategy. All environments work from the main branch, converging through progressive automated rollout.

### How It Works

1. All stack configurations reference the same component path (no version qualifiers)
2. Changes merge to main branch
3. CI/CD pipeline deploys progressively: dev -> staging -> prod
4. Environments naturally diverge during rollout, then converge when pipeline completes

### Configuration

```yaml
# All environments reference the same component
components:
  terraform:
    vpc:
      metadata:
        component: vpc    # Same for dev, staging, prod
      vars:
        environment: prod
        cidr_block: "10.2.0.0/16"
```

### Divergence Model

```text
Time 0: Commit merged to main
  Dev:     new commit (deployed automatically)
  Staging: previous commit (operational divergence)
  Prod:    previous commit

Time +30min: Dev validated
  Staging: new commit (deployed after dev passes)

Time +2hrs: Staging validated, manual approval
  Prod:    new commit (convergence achieved)
```

This operational divergence is expected, time-bound, and automatically converging.

## Folder-Based Versioning

The foundational approach for organizing components with explicit folder structure.

### Directory Structure

```text
components/terraform/
  vpc/
    v1/                   # Version 1 implementation
      main.tf
      variables.tf
      outputs.tf
    v2/                   # Version 2 implementation
      main.tf
      variables.tf
      outputs.tf
      MIGRATION.md
    v3-preview/           # Version 3 in development
      main.tf
      variables.tf
```

### Stable Workspace Keys

Use `metadata.name` to ensure Terraform state remains stable across version upgrades:

```yaml
components:
  terraform:
    vpc:
      metadata:
        name: vpc            # Stable logical identity (workspace key)
        component: vpc/v2    # Physical version path (can change)
      # Result: workspace_key_prefix = "vpc" (stays same when upgrading to v3)
```

Priority order for workspace key calculation:
1. Explicit backend config (`backend.s3.workspace_key_prefix`)
2. `metadata.name` (recommended)
3. `metadata.component`
4. Atmos component name (YAML key)

### Migration Strategy

Roll out new versions progressively:

```yaml
# Week 1: Development
dev:
  vpc:
    metadata:
      component: vpc/v2

# Week 2: Staging
staging:
  vpc:
    metadata:
      component: vpc/v2

# Week 3-4: Production
prod:
  vpc:
    metadata:
      component: vpc/v2
```

### Rollback

Switch the folder reference back. Since old version folder still exists, rollback is instantaneous:

```yaml
vpc:
  metadata:
    component: vpc/v1    # Was: vpc/v2
```

No state migration needed when using stable `metadata.name`.

## Release Tracks/Channels

Named channels that environments subscribe to. Promotes tracks instead of individual pins.

### Two Organization Approaches

**Component-Centric** (`vpc/alpha/`, `vpc/beta/`, `vpc/prod/`):
- Each component has its own tracks
- Independent component evolution
- Best when components have independent release cycles

**Track-Centric** (`alpha/vpc/`, `beta/vpc/`, `prod/vpc/`):
- All components grouped by track
- Cohesive promotion (all components move together)
- Best when components are tightly coupled

### Configuration

```yaml
# stacks/catalog/vpc.yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
        name: vpc              # Stable identity

# stacks/prod/us-east-1.yaml
components:
  terraform:
    vpc:
      metadata:
        inherits: [vpc/defaults]
        component: prod/vpc    # Subscribe to production track
```

### Promotion Workflow

1. New version lands in alpha track
2. After validation, promote to beta track (update vendor.yaml, run `atmos vendor pull`)
3. After staging validation, promote to prod track
4. All environments on that track converge to the new version

### Vendoring to Tracks

```yaml
# vendor.yaml
spec:
  sources:
    - component: vpc-alpha
      source: "git::https://github.com/acme/components.git//vpc?ref=v1.14.0"
      targets:
        - "components/terraform/alpha/vpc"
    - component: vpc-prod
      source: "git::https://github.com/acme/components.git//vpc?ref=v1.12.8"
      targets:
        - "components/terraform/prod/vpc"
```

Each channel MUST be vendored to a distinct path. Vendoring multiple channels to the same path causes the last one to overwrite all others.

## Strict Version Pinning

Explicit SemVer versions for maximum control and audit trail.

### Configuration

```yaml
# vendor.yaml
spec:
  sources:
    - component: vpc
      source: "github.com/acme/components.git//modules/vpc?ref={{.Version}}"
      version: "v1.12.3"
      targets:
        - "components/terraform/vpc/{{.Version}}"

# stacks/catalog/vpc.yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
        name: vpc

# stacks/prod/us-east-1.yaml
components:
  terraform:
    vpc:
      metadata:
        inherits: [vpc/defaults]
        component: vpc/v1.12.3
```

### Trade-offs

Benefits:
- Clear audit trail of exact versions
- Simplified rollback (change pin to previous version)
- Regulatory compliance

Drawbacks:
- Environments naturally drift without constant updates
- Problems surface late in promotion cycle
- Cannot safely skip versions during promotion
- PR storms from automated dependency update tools

## Source-Based Version Pinning

Per-environment version control using the `source` field directly in stack configuration.

### String Form

```yaml
components:
  terraform:
    vpc:
      source: "github.com/org/components//modules/vpc?ref=1.450.0"
      vars:
        cidr_block: "10.0.0.0/16"
```

### Map Form (Full Control)

```yaml
components:
  terraform:
    vpc:
      source:
        uri: github.com/org/components//modules/vpc
        version: 1.450.0
        included_paths:
          - "*.tf"
          - "modules/**"
        excluded_paths:
          - "*.md"
          - "tests/**"
      vars:
        cidr_block: "10.0.0.0/16"
```

### Version Inheritance

Define source defaults in catalog, override versions per environment:

```yaml
# stacks/catalog/vpc/defaults.yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
      source:
        uri: github.com/org/components//modules/vpc
        version: 1.450.0

# stacks/dev/us-east-1.yaml (override version only)
components:
  terraform:
    vpc:
      metadata:
        inherits: [vpc/defaults]
      source:
        version: 1.451.0    # Latest in dev
```

### When to Use Source vs Vendoring

| Requirement | Source | Vendoring |
|------------|--------|-----------|
| No vendor manifest files | Yes | No |
| Code review of dependencies | No | Yes |
| Offline deployment | No | Yes |
| Local modifications | No | Yes |
| Audit trail in Git | No | Yes |
| AI coding assistant context | No | Yes |
| Minimal operational overhead | Yes | No |

## Vendoring Component Versions

Automates copying components from external sources into your repository.

### Basic Configuration

```yaml
# vendor.yaml
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: component-vendoring
spec:
  sources:
    - component: vpc
      source: "github.com/cloudposse/terraform-aws-vpc.git///?ref={{.Version}}"
      version: "2.1.0"
      targets:
        - "components/terraform/vpc"
      included_paths:
        - "**/*.tf"
        - "**/*.tfvars"
        - "README.md"
      excluded_paths:
        - "examples/**"
        - "test/**"
```

### Template Variables

| Variable | Description |
|----------|-------------|
| `{{.Component}}` | Component name from `component:` field |
| `{{.Version}}` | Version from `version:` field |
| `{{.Source}}` | Full source URL |

### Commands

```shell
atmos vendor pull                          # Pull all components
atmos vendor pull --component vpc          # Pull specific component
atmos vendor pull --component vpc --version 2.2.0  # Specific version
atmos vendor pull --dry-run                # Preview changes
```

### Divergence Management

When intentionally diverging from upstream:
1. Make local changes directly in vendored folder
2. Comment out or remove the component from `vendor.yaml`
3. Create `LOCAL_MODIFICATIONS.md` documenting changes
4. Re-enable vendoring when ready to reconverge

## Git Flow: Branches as Channels

Branch-based alternative where long-lived branches map to release channels.

### Branch Structure

```text
main (integration branch)
  channels/prod        # Production channel
    channels/staging   # Staging channel
      channels/dev     # Development channel
        feature/*      # Feature branches
```

### Vendoring from Branches

Each channel must be vendored to a distinct path:

```yaml
# vendor.yaml
spec:
  sources:
    - component: vpc-dev
      source: "git::https://github.com/acme/infra.git//components/terraform/vpc?ref=channels/dev"
      targets:
        - "components/terraform/channels/dev/vpc"
    - component: vpc-prod
      source: "git::https://github.com/acme/infra.git//components/terraform/vpc?ref=channels/prod"
      targets:
        - "components/terraform/channels/prod/vpc"
```

### Stack Configuration

```yaml
# stacks/prod/us-east-1.yaml
components:
  terraform:
    vpc:
      metadata:
        inherits: [vpc/defaults]
        component: channels/prod/vpc
```

### When to Use

- Team already practices Git Flow
- Need centralized promotion control through pull requests
- Require clear audit trails via merge history
- Want approval gates for promotions

## Choosing a Strategy

### Decision Factors

1. **Roll Forward vs Rollback**: Culture of fixing forward or requiring rollback?
2. **Team Size**: Strict pinning becomes painful as environment count grows.
3. **Release Cadence**: High-frequency updates favor convergent patterns.
4. **Third-Party Dependencies**: Heavy external usage benefits from vendoring.
5. **Operational Maturity**: Mature CI/CD enables safer convergent patterns.

### Quick Comparison

| Strategy | Convergence | Overhead | Best For |
|----------|-------------|----------|----------|
| Continuous Version Deployment | Very High | Low | Most teams |
| Folder-Based Versioning | High | Low | Breaking changes |
| Release Tracks/Channels | High | Medium | Many environments |
| Strict Version Pinning | Low | High | Compliance requirements |
| Source-Based Versioning | Low | Low | Simple per-env pinning |
| Git Flow | Medium | Medium | Established Git Flow teams |

### Mixing Strategies

You can mix strategies per component:

```text
components/terraform/
  vpc/                    # Trunk-based (all envs converge)
  eks/                    # Trunk-based (all envs converge)
  monitoring/             # Trunk-based (all envs converge)
  database/
    v1/                   # Folder-based (prod pinned here)
    v2/                   # Folder-based (dev/staging testing)
```

Common combinations:
- Tracks for applications, pinning for platform
- Vendoring + tracks for external + internal components
- Folder versioning for breaking changes, continuous for routine updates
