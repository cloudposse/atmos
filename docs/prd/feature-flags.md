# Product Requirements Document: Atmos Feature Flags

## Overview

This document describes the requirements and implementation for Atmos feature flags, which enable stacks to declaratively enable cross-cutting capabilities. Features follow the same pattern as Atmos profiles - directory-based configuration that gets merged into stacks - but target stack/component configuration rather than CLI behavior.

## Problem Statement

### Current State

Today, enabling cross-cutting concerns (compliance requirements, cost optimization, deployment strategies) requires:
- Duplicating configuration across many stacks
- Complex mixin hierarchies
- Manual coordination to ensure consistency

### Proposed Solution

Features provide a clean abstraction for capabilities that span components:

```yaml
# stacks/ue2-prod.yaml
features:
  - hipaa
  - cost-savings
```

This declaratively says "this stack has HIPAA compliance and cost optimization enabled" and automatically loads the corresponding configuration.

## Goals

- **Declarative capabilities**: Enable cross-cutting concerns with simple list syntax
- **CLI control**: Override or add features at runtime without modifying stack files
- **Discoverability**: Built-in commands to list and describe available features
- **Configuration inheritance**: Features merge with stack configuration using familiar semantics
- **Zero breaking changes**: Existing configurations continue working without modification

## Non-Goals

- **Feature generation UI**: No interactive feature creation wizard (users edit YAML manually)
- **Feature versioning**: No version control or migration system for features
- **Remote feature storage**: Features are always local filesystem-based
- **Feature schema validation**: No validation specific to features (use existing Atmos config validation)

## Design

### Directory Structure

Features live in `stacks/features/` (configurable). Features are similar to mixins - they provide reusable configuration that can be applied across stacks. **Features can be nested using directory paths**, enabling logical organization by category.

```
stacks/
├── features/
│   ├── versions/
│   │   ├── eks/
│   │   │   ├── 1.29/
│   │   │   │   └── eks.yaml
│   │   │   ├── 1.30/
│   │   │   │   └── eks.yaml
│   │   │   └── 1.31/
│   │   │       └── eks.yaml
│   │   ├── rds/
│   │   │   ├── postgres-15/
│   │   │   │   └── rds.yaml
│   │   │   └── postgres-16/
│   │   │       └── rds.yaml
│   │   └── grafana/
│   │       ├── 10.0/
│   │       │   └── grafana.yaml
│   │       └── 11.0/
│   │           └── grafana.yaml
│   ├── compliance/
│   │   ├── hipaa/
│   │   │   ├── vpc.yaml
│   │   │   ├── rds.yaml
│   │   │   └── eks.yaml
│   │   ├── pci-dss/
│   │   │   └── ...
│   │   └── sox/
│   │       └── ...
│   ├── deployment/
│   │   ├── blue/
│   │   │   └── alb.yaml
│   │   └── green/
│   │       └── alb.yaml
│   └── sizing/
│       ├── small/
│       │   └── defaults.yaml
│       ├── medium/
│       │   └── defaults.yaml
│       └── large/
│           └── defaults.yaml
├── catalog/
│   └── ...
├── ue2-dev.yaml
├── ue2-staging.yaml
└── ue2-prod.yaml
```

**Feature paths use forward slashes** to reference nested features:
- `versions/eks/1.30`
- `compliance/hipaa`
- `deployment/blue`
- `sizing/medium`

### Stack Declaration

Stacks declare features they want enabled using path notation:

```yaml
# stacks/ue2-prod.yaml
features:
  - versions/eks/1.30
  - versions/rds/postgres-16
  - compliance/hipaa
  - sizing/large

import:
  - catalog/eks
  - catalog/rds
  - catalog/vpc

components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.0.0.0/16"
```

```yaml
# stacks/ue2-dev.yaml
features:
  - versions/eks/1.31      # Dev runs latest EKS version
  - versions/rds/postgres-16
  - sizing/small           # Smaller resources for dev

import:
  - catalog/eks
  - catalog/rds
  - catalog/vpc
```

### Feature Context and Templating

Features can provide context that is accessible via the `.features` template variable. When multiple features are enabled, their context is deep merged in order and accessible via `.features.<setting>`.

**Feature providing context:**

```yaml
# stacks/features/versions/eks/1.30/eks.yaml
features:
  eks_version: "1.30"
  eks_ami_type: "AL2_x86_64"

components:
  terraform:
    eks:
      vars:
        cluster_version: "1.30"
```

```yaml
# stacks/features/versions/eks/1.31/eks.yaml
features:
  eks_version: "1.31"
  eks_ami_type: "AL2023_x86_64_STANDARD"

components:
  terraform:
    eks:
      vars:
        cluster_version: "1.31"
```

**Components consuming feature context with defaults:**

```yaml
# stacks/catalog/eks.yaml
components:
  terraform:
    eks:
      vars:
        # Use feature context with fallback default
        kubernetes_version: {{ .features.eks_version || "1.29" }}
        ami_type: {{ .features.eks_ami_type || "AL2_x86_64" }}
```

**Multiple features with deep merge:**

When multiple features are enabled, their `features` blocks are deep merged in declaration order:

```yaml
# stacks/ue2-prod.yaml
features:
  - versions/eks/1.30       # Provides: features.eks_version = "1.30"
  - compliance/hipaa        # Provides: features.compliance_framework = "hipaa"
  - sizing/large            # Provides: features.instance_type = "m5.xlarge"
```

The resulting `.features` context contains all merged values:
- `.features.eks_version` = `"1.30"`
- `.features.compliance_framework` = `"hipaa"`
- `.features.instance_type` = `"m5.xlarge"`

### Parameterized Features

Features can also accept explicit parameters via map syntax:

```yaml
# stacks/ue2-prod.yaml
features:
  - versions/eks/1.30
  - policies/retention:
      days: 90
      archive_after: 30
  - scaling/autoscale:
      min: 2
      max: 10
```

Parameters are merged into the feature context and accessible via `.features`:

```yaml
# stacks/features/retention-policy/defaults.yaml
metadata:
  name: retention-policy
  description: "Configurable data retention policy"

# Parameters provided by the stack are accessible via .features
features:
  retention_days: {{ .features.days || 30 }}

components:
  terraform:
    s3:
      vars:
        lifecycle_rules:
          - expiration_days: {{ .features.days }}
            transition_days: {{ .features.archive_after }}
    cloudwatch:
      vars:
        log_retention_days: {{ .features.days }}
```

### Merge Behavior

Features are loaded and merged in order:

1. Base stack configuration (imports, components)
2. Features in declared order (first to last)
3. Stack-level overrides

Later features override earlier ones, and explicit stack configuration always wins.

```yaml
features:
  - hipaa        # Loaded first
  - cost-savings # Loaded second, overrides hipaa where conflicts exist
```

### CLI Flag

Features can be specified at runtime using the `--features` flag, which replaces stack-declared features:

```bash
# Single feature - replaces stack features
atmos terraform plan vpc -s ue2-prod --features deployment/blue

# Multiple features - replaces stack features with this list
atmos terraform plan vpc -s ue2-prod --features compliance/hipaa,deployment/blue

# Explicit control over all features
atmos terraform plan vpc -s ue2-prod --features versions/eks/1.30,compliance/hipaa,sizing/large
```

Note: `--feature` is an alias for `--features`. Both accept a single value or comma-separated list.

### Environment Variable

```bash
# Set features via environment variable (replaces stack features)
export ATMOS_FEATURES=deployment/blue

# Multiple features
export ATMOS_FEATURES=compliance/hipaa,deployment/green
```

### Profile-Based Default Features

Atmos profiles can define features that are automatically enabled when the profile is active. This creates a powerful integration between the two systems - profiles control CLI behavior while also enabling stack-level features.

```yaml
# profiles/ci/features.yaml
features:
  defaults:
    - sizing/small           # Smaller resources for CI
    - observability/minimal  # Minimal logging in CI

# profiles/production/features.yaml
features:
  defaults:
    - compliance/hipaa       # Production requires HIPAA compliance
    - reliability/ha         # Production requires HA
    - observability/full     # Full monitoring in production
```

**Usage:**

```bash
# CI profile automatically enables sizing/small, observability/minimal
atmos terraform plan vpc -s ue2-dev --profile ci

# Production profile automatically enables compliance, reliability, monitoring
atmos terraform apply vpc -s ue2-prod --profile production

# Override profile and stack features with explicit list
atmos terraform plan vpc -s ue2-prod --profile production --features deployment/green
```

**Precedence for feature activation:**

1. `--features` CLI flag (replaces all below)
2. `ATMOS_FEATURES` environment variable (replaces all below)
3. Stack-declared `features` list (extends profile defaults)
4. Profile-declared `features.defaults` list (baseline)

**Benefits:**

- **Environment consistency**: CI profiles ensure dev/test environments use appropriate features
- **Compliance enforcement**: Production profiles can mandate compliance features
- **Reduced repetition**: Common feature sets are defined once in the profile
- **Explicit control**: CLI flag gives full control when needed

## Feature File Format

Feature files are standard Atmos stack configuration with an optional `features` block for context:

```yaml
# stacks/features/versions/eks/1.30/eks.yaml
metadata:
  name: versions/eks/1.30
  description: "EKS 1.30 cluster configuration"

# Context provided to templates via .features
features:
  eks_version: "1.30"
  eks_ami_type: "AL2_x86_64"
  eks_addons_version: "v1.30.0-eksbuild.1"

components:
  terraform:
    eks:
      vars:
        cluster_version: "1.30"
        cluster_addons:
          coredns:
            addon_version: "v1.11.1-eksbuild.4"
          kube-proxy:
            addon_version: "v1.30.0-eksbuild.1"
```

```yaml
# stacks/features/compliance/hipaa/rds.yaml
features:
  compliance_framework: hipaa
  audit_logging: required

components:
  terraform:
    rds:
      vars:
        storage_encrypted: true
        enabled_cloudwatch_logs_exports:
          - audit
          - error
          - general
          - slowquery
        deletion_protection: true
        backup_retention_period: 35
        performance_insights_enabled: true
        performance_insights_retention_period: 731  # 2 years
```

```yaml
# stacks/features/sizing/small/defaults.yaml
features:
  instance_type: "t3.medium"
  use_spot: true

components:
  terraform:
    eks:
      vars:
        node_groups:
          default:
            instance_types: ["t3.medium", "t3a.medium"]
            capacity_type: "SPOT"
```

### Feature Metadata

Features use the standard `metadata` block (same pattern as components):

```yaml
# stacks/features/versions/eks/1.31/eks.yaml
metadata:
  name: versions/eks/1.31
  description: "EKS 1.31 with AL2023 AMI support"

features:
  eks_version: "1.31"
  eks_ami_type: "AL2023_x86_64_STANDARD"

components:
  terraform:
    eks:
      vars:
        cluster_version: "1.31"
```

The `metadata.name` and `metadata.description` are used by `atmos feature list` for discoverability. The `features` block provides context accessible via `.features` in templates.

## Configuration

In `atmos.yaml`:

```yaml
stacks:
  features:
    base_path: "stacks/features"  # Default
    enabled: true                  # Default
```

## CLI Commands

### List Available Features

```bash
$ atmos feature list

Available features:
  compliance/hipaa           HIPAA compliance configuration
  compliance/pci-dss         PCI-DSS compliance configuration
  compliance/sox             SOX compliance configuration
  deployment/blue            Target blue deployment
  deployment/green           Target green deployment
  sizing/small               Small resource profile (dev/test)
  sizing/medium              Medium resource profile
  sizing/large               Large resource profile (production)
  versions/eks/1.29          EKS 1.29 cluster configuration
  versions/eks/1.30          EKS 1.30 cluster configuration
  versions/eks/1.31          EKS 1.31 with AL2023 AMI support
  versions/grafana/10.0      Grafana 10.0 configuration
  versions/grafana/11.0      Grafana 11.0 configuration
  versions/rds/postgres-15   PostgreSQL 15 configuration
  versions/rds/postgres-16   PostgreSQL 16 configuration
```

### Describe Feature

```bash
$ atmos describe feature versions/eks/1.30

# Outputs valid stack YAML format
metadata:
  name: versions/eks/1.30
  description: "EKS 1.30 cluster configuration"

features:
  eks_version: "1.30"
  eks_ami_type: "AL2_x86_64"
  eks_addons_version: "v1.30.0-eksbuild.1"

components:
  terraform:
    eks:
      vars:
        cluster_version: "1.30"
        cluster_addons:
          coredns:
            addon_version: "v1.11.1-eksbuild.4"
```

### Show Stack with Features Resolved

```bash
$ atmos describe stacks ue2-prod --format yaml

# Shows fully resolved configuration with features applied
```

## Use Cases

### 1. Versioned Components (EKS, RDS, Grafana, etc.)

Features are ideal for managing component versions across environments:

```yaml
# stacks/ue2-dev.yaml
features:
  - versions/eks/1.31          # Dev runs latest EKS
  - versions/rds/postgres-16   # Dev runs latest PostgreSQL
  - versions/grafana/11.0      # Dev runs latest Grafana

# stacks/ue2-staging.yaml
features:
  - versions/eks/1.30          # Staging mirrors prod
  - versions/rds/postgres-16
  - versions/grafana/10.0

# stacks/ue2-prod.yaml
features:
  - versions/eks/1.30          # Prod on stable EKS version
  - versions/rds/postgres-16
  - versions/grafana/10.0
```

Components can reference version-specific settings via `.features`:

```yaml
# stacks/catalog/eks.yaml
components:
  terraform:
    eks:
      vars:
        kubernetes_version: {{ .features.eks_version || "1.29" }}
        ami_type: {{ .features.eks_ami_type || "AL2_x86_64" }}
```

### 2. Compliance Requirements

```yaml
# stacks/ue2-prod.yaml
features:
  - versions/eks/1.30
  - compliance/hipaa
  - compliance/sox
```

All components automatically get compliance-required settings. The `.features` context includes:
- `.features.eks_version` = `"1.30"`
- `.features.compliance_framework` = `"hipaa"`

### 3. Environment Tiers

```yaml
# stacks/ue2-dev.yaml
features:
  - versions/eks/1.31
  - sizing/small
  - observability/minimal

# stacks/ue2-prod.yaml
features:
  - versions/eks/1.30
  - sizing/large
  - observability/full
```

### 4. Deployment Strategies

```bash
# Deploy to blue target group
atmos terraform apply eks -s ue2-prod --feature deployment/blue

# Switch traffic to green
atmos terraform apply eks -s ue2-prod --feature deployment/green
```

### 5. Temporary Toggles

```bash
# Debug mode for troubleshooting
atmos terraform plan vpc -s ue2-prod --feature observability/debug

# Test EKS upgrade without changing stack file
atmos terraform plan eks -s ue2-prod --feature versions/eks/1.31
```

### 6. Customer/Tenant Specific

```yaml
# stacks/acme-corp/prod.yaml
features:
  - versions/eks/1.30
  - tiers/enterprise
  - resources/dedicated
  - networking/custom-domain
```

### 7. Profile-Based Environment Defaults

```yaml
# profiles/ci/features.yaml
features:
  defaults:
    - sizing/small
    - observability/debug

# profiles/production/features.yaml
features:
  defaults:
    - hipaa
    - high-availability
    - enhanced-monitoring
```

```bash
# All CI runs automatically get cost-savings, fast-iteration, debug-logging
ATMOS_PROFILE=ci atmos terraform plan vpc -s ue2-dev

# Production deploys automatically enforce compliance
atmos terraform apply vpc -s ue2-prod --profile production
```

This pattern ensures environment-appropriate features are always enabled without requiring every stack to declare them.

### 8. Affected Workflows with Features

Features integrate with Atmos affected functionality for CI/CD pipelines:

```bash
# List affected stacks with their features
atmos list affected --columns stack,component,features

# Describe affected with feature details
atmos describe affected --include-settings

# Plan all affected components with a feature override
atmos terraform plan --affected --feature blue

# Apply to affected components targeting green deployment
atmos terraform apply --affected --feature green
```

**Detecting feature-related changes:**

```bash
# When stacks/features/eks-1.31/eks.yaml is modified:
$ atmos list affected

STACK        COMPONENT  AFFECTED_BY
ue2-dev      eks        feature:eks-1.31
ue2-staging  eks        feature:eks-1.31

# When ue2-prod.yaml adds a new feature:
$ atmos list affected

STACK        COMPONENT  AFFECTED_BY
ue2-prod     vpc        feature-activation:hipaa
ue2-prod     rds        feature-activation:hipaa
ue2-prod     eks        feature-activation:hipaa
```

This enables CI/CD pipelines to:
- Detect when feature changes affect specific stacks
- Apply feature overrides during deployment (e.g., blue-green)
- Track feature-related drift across environments

## Comparison

### vs Terragrunt Feature Flags

| Aspect | Terragrunt | Atmos Features |
|--------|------------|----------------|
| Scope | Single unit | Entire stack |
| Definition | `feature` block in HCL | Directory of YAML files |
| Purpose | Runtime toggle | Cross-cutting capabilities |
| Composition | Override single value | Merge full configuration |
| Declaration | In unit config | In stack or CLI |

### vs Atmos Mixins

**Features are mixins with two additions: CLI control and discoverability.**

| Aspect | Mixins | Features |
|--------|--------|----------|
| Activation | `import` statement only | `features` list or `--feature` flag |
| Runtime override | No | Yes |
| Discoverability | Grep through imports | `atmos feature list` |
| Mechanism | Identical | Identical |

Same merge behavior, same file format, same everything - just with CLI control and built-in discoverability.

### vs Atmos Profiles

| Aspect | Profiles | Features |
|--------|----------|----------|
| Target | CLI/Atmos behavior | Stack/component config |
| Directory | `profiles/` | `stacks/features/` |
| Flag | `--profile` | `--features` |
| Env var | `ATMOS_PROFILE` | `ATMOS_FEATURES` |

## Requirements

### Functional Requirements

#### FR1: Feature Definition and Location

**FR1.1**: Features MUST be stored in a configurable directory (default: `stacks/features/`).

**FR1.2**: Each feature MUST be a directory containing one or more YAML configuration files.

**FR1.3**: Feature configuration files MUST follow the same naming conventions as stack files.

**FR1.4**: Features SHOULD support organizing configuration by component (e.g., `vpc.yaml`, `rds.yaml`).

**FR1.5**: Feature metadata SHOULD be defined using the standard `metadata` block.

#### FR2: Feature Activation

**FR2.1**: Features MUST be activated via `features` list in stack configuration.

**FR2.2**: Features MUST be activated via `--features` CLI flag (StringSlice flag, replaces stack/profile features).

**FR2.3**: Features MUST be activated via `ATMOS_FEATURES` environment variable (comma-separated, replaces stack/profile features).

**FR2.4**: Precedence MUST be: CLI flag > environment variable > stack declaration > profile defaults.

**FR2.5**: When no features are specified via CLI or env var, stack and profile features MUST be used.

**FR2.6**: When no features are specified anywhere, Atmos MUST behave identically to current behavior.

#### FR3: Feature Merging

**FR3.1**: Features MUST be merged after base stack configuration (imports, components).

**FR3.2**: Features MUST be merged in declared order (first to last).

**FR3.3**: Later features MUST override earlier features where conflicts exist.

**FR3.4**: Stack-level overrides MUST always win over feature configuration.

**FR3.5**: Feature merging MUST use the same deep merge semantics as stack imports.

#### FR4: Feature Context and Templating

**FR4.1**: Features MUST support a `features` block for providing context.

**FR4.2**: Feature context MUST be accessible via `.features.<setting>` in templates.

**FR4.3**: Multiple features MUST have their context deep merged in declaration order.

**FR4.4**: Templates MUST support default values via `{{ .features.setting || "default" }}` syntax.

**FR4.5**: Features MUST support optional parameters via map syntax for parameterized features.

**FR4.6**: Parameters from parameterized features MUST be merged into the `.features` context.

#### FR5: Feature Management Commands

**FR5.1**: New command `atmos feature list` MUST list all available features.

**FR5.2**: New command `atmos describe feature <name>` MUST display feature configuration.

**FR5.3**: `atmos describe stacks` MUST show features resolved in output.

#### FR6: Profile Integration

**FR6.1**: Profiles MUST support defining default features via `features.defaults` list.

**FR6.2**: Profile-declared default features MUST be applied when the profile is active.

**FR6.3**: Stack-declared features MUST extend (not replace) profile-declared default features.

**FR6.4**: CLI `--features` flag MUST replace all features (profile, stack, and env var).

**FR6.5**: Multiple active profiles MUST have their default features merged (left-to-right precedence).

**FR6.6**: Profile default features MUST be resolved before stack features in the merge order.

### Technical Requirements

#### TR1: Schema Updates

**TR1.1**: Add `features` field to stack schema (list of strings or maps).

**TR1.2**: Add `stacks.features.base_path` to `atmos.yaml` schema.

**TR1.3**: Add `stacks.features.enabled` to `atmos.yaml` schema.

**TR1.4**: Update JSON schemas in `pkg/datafetcher/schema/`.

#### TR2: CLI Flag Implementation

**TR2.1**: Add `--features` flag to terraform/helmfile commands (with `--feature` as alias).

**TR2.2**: Use `pkg/flags/` infrastructure for flag parsing.

**TR2.3**: Bind `ATMOS_FEATURES` environment variable.

#### TR3: Feature Loading

**TR3.1**: Feature loading MUST reuse existing stack merge logic from `pkg/stack/`.

**TR3.2**: Feature loading MUST support the same templating as stack files.

**TR3.3**: Feature loading MUST validate that requested features exist.

#### TR4: Performance

**TR4.1**: Feature discovery MUST be cached during command execution.

**TR4.2**: Feature loading MUST complete within 100ms for typical features.

**TR4.3**: Memory usage MUST scale linearly with number of active features.

#### TR5: Testing

**TR5.1**: Unit tests MUST verify feature merge precedence.

**TR5.2**: Integration tests MUST verify feature activation via CLI and env var.

**TR5.3**: Tests MUST verify parameterized features with templating.

**TR5.4**: Tests MUST verify backward compatibility (no features specified).

#### TR6: Profile Integration

**TR6.1**: Add `features.defaults` field to profile schema (list of strings).

**TR6.2**: Profile feature loading MUST integrate with existing profile loading in `pkg/config/`.

**TR6.3**: Feature precedence resolution MUST occur before stack resolution.

**TR6.4**: Tests MUST verify profile-based feature activation with single and multiple profiles.

**TR6.5**: Tests MUST verify precedence: CLI flag > env var > stack > profile.

#### FR7: Affected Integration

**FR7.1**: `atmos describe affected` MUST include active features in its change detection scope.

**FR7.2**: `atmos describe affected` output MUST include a `features` field showing resolved features for each affected component/stack.

**FR7.3**: `atmos list affected` MUST display active features when present (e.g., via `--columns features` or default columns).

**FR7.4**: `atmos terraform <subcommand> --affected` MUST respect the `--features` flag.

**FR7.5**: When `--features` is passed with `--affected`, the feature configuration MUST be applied to all affected components.

**FR7.6**: Feature changes (additions, removals, modifications to feature files) MUST be detected as changes that affect stacks using those features.

**FR7.7**: The affected output MUST distinguish between:
- Component/stack changes
- Feature file changes that affect the component/stack
- Feature activation changes (feature added/removed from stack's `features` list)

#### TR7: Affected Integration

**TR7.1**: The affected detection logic in `internal/exec/describe_affected.go` MUST be extended to track feature file changes.

**TR7.2**: Feature resolution MUST occur before affected calculation to determine feature scope.

**TR7.3**: The `--features` flag on terraform commands with `--affected` MUST be validated and applied consistently.

**TR7.4**: Tests MUST verify that modifying a feature file marks all stacks using that feature as affected.

**TR7.5**: Tests MUST verify that `--affected` with `--features` applies the feature to all affected components.

**TR7.6**: Performance MUST remain acceptable when combining `--affected` with features (no N×M explosion).

## Implementation Plan

### Phase 1: Core Functionality

**Tasks:**
1. Add `features` field to stack schema
2. Add `stacks.features` configuration to `atmos.yaml` schema
3. Implement feature directory discovery
4. Implement feature configuration loading and merging
5. Integrate feature loading into stack resolution pipeline
6. Add `--features` CLI flag to terraform/helmfile commands (with `--feature` alias)
7. Add `ATMOS_FEATURES` environment variable support
8. Add `features.defaults` field to profile schema
9. Integrate profile default features into feature resolution pipeline

**Deliverables:**
- Feature discovery in `pkg/stack/`
- Feature merging in stack resolution
- Profile integration in `pkg/config/`
- CLI flags in `cmd/terraform/` and `cmd/helmfile/`
- Unit tests for feature loading, merging, and profile integration

### Phase 2: CLI Commands

**Tasks:**
1. Implement `atmos feature list` command
2. Implement `atmos describe feature <name>` command
3. Update `atmos describe stacks` to show resolved features
4. Add feature resolution to stack describe output

**Deliverables:**
- Feature list command in `cmd/feature/`
- Feature describe command in `cmd/describe/`
- Updated describe stacks output
- CLI integration tests

### Phase 3: Affected Integration

**Tasks:**
1. Extend `describe affected` to track feature file changes
2. Add `features` field to affected output
3. Update `list affected` to display features column
4. Add `--features` flag support to terraform commands with `--affected`
5. Implement feature change detection (additions, removals, modifications)
6. Add `AFFECTED_BY` categorization for feature-related changes

**Deliverables:**
- Feature change detection in `internal/exec/describe_affected.go`
- Updated affected output schema with features
- `--features` flag integration with `--affected`
- Unit tests for feature-aware affected detection

### Phase 4: Advanced Features

**Tasks:**
1. Implement parameterized features with templating
2. Add feature metadata support
3. Add feature validation (requires/conflicts)
4. Add feature composition validation

**Deliverables:**
- Parameterized feature support
- Feature metadata rendering
- Validation rules

### Phase 5: Documentation

**Tasks:**
1. Create feature configuration examples
2. Document merge behavior with diagrams
3. Create migration guide from mixins to features
4. Write blog post announcing feature flags
5. Update JSON schemas

**Deliverables:**
- Documentation in `website/docs/`
- Blog post in `website/blog/`
- Example features in `examples/`
- Updated schemas

## Open Questions

1. **Naming**: `features` vs `capabilities` vs `traits`?
   - **Recommendation**: `features` - familiar term, aligns with Terragrunt terminology

2. **Merge order**: Should features merge before or after catalog imports?
   - **Recommendation**: After imports, before stack-level config

3. **Negative features**: Support `features: [!debug-logging]` to explicitly disable?
   - **Recommendation**: Defer to Phase 4, consider `features_disabled` list instead

## Success Metrics

- Reduction in duplicated compliance configuration (target: 50% less duplication)
- Faster onboarding of new compliance requirements (target: 80% faster)
- Cleaner stack files (target: 30% fewer imports on average)
- Runtime flexibility without stack file changes (adoption metric)
- `atmos feature list` among top 20 most-used commands

## References

- [Atmos Profiles PRD](./atmos-profiles.md) - Similar pattern for CLI configuration
- [Command Registry Pattern PRD](./command-registry-pattern.md) - Command implementation pattern
- [Terragrunt Feature Flags](https://terragrunt.gruntwork.io/docs/features/feature-flags/) - Prior art
- [Atmos Stack Configuration](https://atmos.tools/stacks) - Existing stack documentation
