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

Features live in `stacks/features/` (configurable). Features are similar to mixins - they provide reusable configuration that can be applied across stacks. Good examples include versioned component configurations (like EKS versions), compliance requirements, and deployment strategies.

```
stacks/
├── features/
│   ├── eks-1.29/
│   │   └── eks.yaml           # EKS 1.29 configuration
│   ├── eks-1.30/
│   │   └── eks.yaml           # EKS 1.30 configuration
│   ├── eks-1.31/
│   │   └── eks.yaml           # EKS 1.31 configuration
│   ├── hipaa/
│   │   ├── vpc.yaml           # VPC hardening
│   │   ├── rds.yaml           # Encryption, audit logging
│   │   ├── eks.yaml           # Pod security, network policies
│   │   └── _defaults.yaml     # Feature-wide defaults
│   ├── cost-savings/
│   │   ├── eks.yaml           # Spot instances, smaller nodes
│   │   └── rds.yaml           # Reserved instances, smaller types
│   └── blue-green/
│       └── alb.yaml           # Dual target groups
├── catalog/
│   └── ...
├── ue2-dev.yaml
├── ue2-staging.yaml
└── ue2-prod.yaml
```

### Stack Declaration

Stacks declare features they want enabled:

```yaml
# stacks/ue2-prod.yaml
features:
  - eks-1.30
  - hipaa

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
  - eks-1.31      # Dev runs latest EKS version
  - cost-savings

import:
  - catalog/eks
  - catalog/rds
  - catalog/vpc
```

### Feature Context and Templating

Features can provide context that is accessible via the `.features` template variable. When multiple features are enabled, their context is deep merged in order and accessible via `.features.<setting>`.

**Feature providing context:**

```yaml
# stacks/features/eks-1.30/eks.yaml
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
# stacks/features/eks-1.31/eks.yaml
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
  - eks-1.30       # Provides: features.eks_version = "1.30"
  - hipaa          # Provides: features.compliance_framework = "hipaa"
  - cost-savings   # Provides: features.instance_type = "t3.medium"
```

The resulting `.features` context contains all merged values:
- `.features.eks_version` = `"1.30"`
- `.features.compliance_framework` = `"hipaa"`
- `.features.instance_type` = `"t3.medium"`

### Parameterized Features

Features can also accept explicit parameters via map syntax:

```yaml
# stacks/ue2-prod.yaml
features:
  - eks-1.30
  - retention-policy:
      days: 90
      archive_after: 30
  - scaling:
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

### CLI Flag Override

Features can also be specified or overridden at runtime:

```bash
# Add feature at runtime
atmos terraform plan vpc -s ue2-prod --feature blue-green

# Multiple features
atmos terraform plan vpc -s ue2-prod --feature hipaa,cost-savings

# Override stack-declared features entirely
atmos terraform plan vpc -s ue2-prod --features-override blue-green
```

### Environment Variable

```bash
# Additive - adds to stack-declared features
export ATMOS_FEATURE=blue-green

# Override - replaces stack-declared features
export ATMOS_FEATURES_OVERRIDE=blue-green
```

### Profile-Based Default Features

Atmos profiles can define features that are automatically enabled when the profile is active. This creates a powerful integration between the two systems - profiles control CLI behavior while also enabling stack-level features.

```yaml
# profiles/ci/features.yaml
features:
  defaults:
    - cost-savings      # Always enable cost savings in CI
    - fast-iteration    # Faster builds for CI

# profiles/production/features.yaml
features:
  defaults:
    - hipaa             # Production requires HIPAA compliance
    - high-availability # Production requires HA
    - enhanced-monitoring
```

**Usage:**

```bash
# CI profile automatically enables cost-savings and fast-iteration features
atmos terraform plan vpc -s ue2-dev --profile ci

# Production profile automatically enables hipaa, high-availability, enhanced-monitoring
atmos terraform apply vpc -s ue2-prod --profile production

# Override profile defaults with explicit features
atmos terraform plan vpc -s ue2-prod --profile production --features-override blue-green
```

**Precedence for feature activation:**

1. `--features-override` CLI flag (replaces all other features)
2. `--feature` CLI flag (additive)
3. `ATMOS_FEATURES_OVERRIDE` environment variable (replaces all other features)
4. `ATMOS_FEATURE` environment variable (additive)
5. Stack-declared `features` list
6. Profile-declared `features.defaults` list (from active profile)

**Benefits:**

- **Environment consistency**: CI profiles ensure dev/test environments use cost-saving features
- **Compliance enforcement**: Production profiles can mandate compliance features
- **Reduced repetition**: Common feature sets are defined once in the profile
- **Layered control**: Stack-level features extend profile defaults; CLI can override both

## Feature File Format

Feature files are standard Atmos stack configuration with an optional `features` block for context:

```yaml
# stacks/features/eks-1.30/eks.yaml
metadata:
  name: eks-1.30
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
# stacks/features/hipaa/rds.yaml
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
# stacks/features/cost-savings/eks.yaml
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
# stacks/features/eks-1.31/eks.yaml
metadata:
  name: eks-1.31
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
  eks-1.29      EKS 1.29 cluster configuration
  eks-1.30      EKS 1.30 cluster configuration
  eks-1.31      EKS 1.31 with AL2023 AMI support
  hipaa         HIPAA compliance configuration
  pci-dss       PCI-DSS compliance configuration
  cost-savings  Cost optimization settings
  blue-green    Blue-green deployment support
```

### Describe Feature

```bash
$ atmos describe feature eks-1.30

# Outputs valid stack YAML format
metadata:
  name: eks-1.30
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

### 1. Versioned Components (EKS, RDS, etc.)

Features are ideal for managing component versions across environments:

```yaml
# stacks/ue2-dev.yaml
features:
  - eks-1.31      # Dev runs latest version

# stacks/ue2-staging.yaml
features:
  - eks-1.30      # Staging mirrors prod

# stacks/ue2-prod.yaml
features:
  - eks-1.30      # Prod on stable version
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
  - eks-1.30
  - hipaa
  - sox
```

All components automatically get compliance-required settings. The `.features` context includes:
- `.features.eks_version` = `"1.30"`
- `.features.compliance_framework` = `"hipaa"`

### 3. Environment Tiers

```yaml
# stacks/ue2-dev.yaml
features:
  - eks-1.31
  - cost-savings
  - fast-iteration  # Fewer replicas, faster deploys

# stacks/ue2-prod.yaml
features:
  - eks-1.30
  - high-availability
  - enhanced-monitoring
```

### 4. Deployment Strategies

```bash
# Enable blue-green for a specific deployment
atmos terraform apply eks -s ue2-prod --feature blue-green
```

### 5. Temporary Toggles

```bash
# Debug mode for troubleshooting
atmos terraform plan vpc -s ue2-prod --feature debug-logging

# Test EKS upgrade without changing stack file
atmos terraform plan eks -s ue2-prod --feature eks-1.31
```

### 6. Customer/Tenant Specific

```yaml
# stacks/acme-corp/prod.yaml
features:
  - eks-1.30
  - enterprise
  - dedicated-resources
  - custom-domain
```

### 7. Profile-Based Environment Defaults

```yaml
# profiles/ci/features.yaml
features:
  defaults:
    - cost-savings
    - fast-iteration
    - debug-logging

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
| Flag | `--profile` | `--feature` |
| Env var | `ATMOS_PROFILE` | `ATMOS_FEATURE` |

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

**FR2.2**: Features MUST be activated via `--feature` CLI flag (StringSlice flag).

**FR2.3**: Features MUST be activated via `ATMOS_FEATURE` environment variable (comma-separated).

**FR2.4**: CLI flag MUST take precedence over environment variable.

**FR2.5**: Stack-declared features MUST be applied unless overridden with `--features-override`.

**FR2.6**: When no features are specified, Atmos MUST behave identically to current behavior.

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

**FR6.4**: CLI `--feature` flag MUST be additive to both profile and stack features.

**FR6.5**: CLI `--features-override` flag MUST replace all features (profile, stack, and env var).

**FR6.6**: Multiple active profiles MUST have their default features merged (left-to-right precedence).

**FR6.7**: Profile default features MUST be resolved before stack features in the merge order.

### Technical Requirements

#### TR1: Schema Updates

**TR1.1**: Add `features` field to stack schema (list of strings or maps).

**TR1.2**: Add `stacks.features.base_path` to `atmos.yaml` schema.

**TR1.3**: Add `stacks.features.enabled` to `atmos.yaml` schema.

**TR1.4**: Update JSON schemas in `pkg/datafetcher/schema/`.

#### TR2: CLI Flag Implementation

**TR2.1**: Add `--feature` flag to terraform/helmfile commands.

**TR2.2**: Add `--features-override` flag to terraform/helmfile commands.

**TR2.3**: Use `pkg/flags/` infrastructure for flag parsing.

**TR2.4**: Bind `ATMOS_FEATURE` and `ATMOS_FEATURES_OVERRIDE` environment variables.

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

**TR6.5**: Tests MUST verify precedence: CLI override > CLI additive > env override > env additive > stack > profile.

## Implementation Plan

### Phase 1: Core Functionality

**Tasks:**
1. Add `features` field to stack schema
2. Add `stacks.features` configuration to `atmos.yaml` schema
3. Implement feature directory discovery
4. Implement feature configuration loading and merging
5. Integrate feature loading into stack resolution pipeline
6. Add `--feature` CLI flag to terraform/helmfile commands
7. Add `ATMOS_FEATURE` environment variable support
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

### Phase 3: Advanced Features

**Tasks:**
1. Implement parameterized features with templating
2. Add feature metadata support
3. Implement `--features-override` flag
4. Add feature validation (requires/conflicts)
5. Add feature composition validation

**Deliverables:**
- Parameterized feature support
- Feature metadata rendering
- Override functionality
- Validation rules

### Phase 4: Documentation

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
   - **Recommendation**: Defer to Phase 3, consider `features_disabled` list instead

4. **CLI parameters**: How to pass feature parameters via `--feature` flag?
   - **Option A**: JSON-ish syntax: `--feature "retention-policy:{days:90}"`
   - **Option B**: Dot notation: `--feature retention-policy --feature-var retention-policy.days=90`
   - **Recommendation**: Option A for simplicity, Option B for complex cases

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
