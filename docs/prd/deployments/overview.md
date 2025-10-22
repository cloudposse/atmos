# Atmos Deployments - Overview

## Definitions

### Core Concepts

**Deployment**
- A YAML configuration file that defines an isolated subset of stacks and components representing a complete application or service
- Contains component definitions (nixpack, terraform, helmfile, lambda), targets (dev/staging/prod), and vendor configuration
- **Example**: `deployments/payment-service.yaml`
- **Purpose**: Scopes what Atmos processes (10-20x faster than scanning entire repository)

**Target**
- An environment-specific variant of a deployment (e.g., `dev`, `staging`, `prod`)
- Each target has its own labels and context (CPU, memory, replicas, etc.)
- Targets use the same component definitions but with different configuration overrides
- **Example**: `payment-service` deployment has targets: `dev`, `staging`, `prod`

**Build**
- The process of creating a container image from source code using nixpacks (or Dockerfile)
- Executed via `atmos deployment build <deployment> --target <target>`
- **Input**: Source code + nixpack configuration
- **Output**: Container image with digest (e.g., `sha256:abc123...`)
- **Artifact**: Image pushed to registry + local SBOM

**Release**
- An immutable record capturing the state of a deployment at a specific point in time
- Contains image digests, Git metadata, SBOM, provenance, and annotations
- Stored as YAML in `releases/<deployment>/<target>/release-<id>.yaml`
- **Purpose**: Enables promotion across targets and rollbacks
- **Created by**: `atmos deployment release <deployment> --target <target>`

**Rollout** (formerly "Apply")
- The act of updating infrastructure components (Terraform/Helm/Lambda) to use a specific release
- Updates ECS task definitions, Lambda functions, Helm charts with new image digests from a release
- Executed via `atmos deployment rollout <deployment> --target <target> [--release <id>]`
- **Default behavior**: Uses latest release for target
- **Promotion**: Can rollout a release from one target to another (e.g., dev release to staging)

### Deployment Stages

Atmos deployments follow a **multi-stage pipeline** where each stage is extensible and optional:

```
Source Code
    ↓
[BUILD STAGE] ────→ Container Image (sha256:abc123...)
    ↓
[TEST STAGE] ─────→ Test Results (JUnit, coverage)
    ↓
[RELEASE STAGE] ──→ Release Record (release-abc123.yaml)
    ↓                 ↓
    │                 └──→ [PROMOTE] ─────→ staging/prod
    ↓
[ROLLOUT STAGE] ──→ Infrastructure Updated (ECS/Lambda/Helm)
```

**Stage Abstraction**:
- Each stage is a discrete, reusable operation
- Stages can be run independently: `atmos deployment build`, `atmos deployment test`, etc.
- Future stages can be added without breaking existing workflows
- Not all stages required for every deployment (e.g., Lambda deployments skip build stage)

**Future Extensibility** (potential stages):
- **Scan Stage**: Security scanning (Trivy, Grype, Snyk) of container images
- **Validate Stage**: Policy validation (OPA, JSON Schema) before rollout
- **Package Stage**: Bundle deployment artifacts (OCI, Helm charts)
- **Promote Stage**: Cross-target promotion with approval gates
- **Verify Stage**: Post-deployment smoke tests and health checks
- **Benchmark Stage**: Performance testing and regression detection

**Stage Design Principles**:
1. **Composable**: Stages can be combined in any order
2. **Idempotent**: Running a stage multiple times produces same result
3. **Stateless**: Each stage operates on inputs, produces outputs
4. **CI-Friendly**: Stages map naturally to CI/CD pipeline steps
5. **Local-First**: All stages work identically on laptop and in CI

**Example Workflow**:
```bash
# 1. Build: Create container images for dev target
atmos deployment build payment-service --target dev
# → Builds nixpack components, pushes to registry
# → Output: sha256:abc123...

# 2. Release: Capture current state as immutable record
atmos deployment release payment-service --target dev
# → Creates releases/payment-service/dev/release-xyz789.yaml
# → Contains: image digests, Git SHA, SBOM, provenance

# 3. Rollout: Update infrastructure to use release
atmos deployment rollout payment-service --target dev
# → Runs terraform apply on ECS components with new image digest
# → Updates task definitions, services

# 4. Promote: Use dev release in staging
atmos deployment rollout payment-service --target staging --release xyz789
# → Same release, different target
# → No rebuild needed

# 5. Rollback: Revert to previous release
atmos deployment rollout payment-service --target prod --release abc456
# → Infrastructure reverts to older image digests
```

### Key Distinctions

| Concept | What It Does | Command | Artifact Created |
|---------|--------------|---------|------------------|
| **Build** | Compiles source → container image | `atmos deployment build` | Image + digest |
| **Release** | Captures deployment state snapshot | `atmos deployment release` | Release record (YAML) |
| **Rollout** | Updates infrastructure with release | `atmos deployment rollout` | Terraform state changes |

**Why separate Release from Rollout?**
- **Release** is environment-agnostic: same image can be promoted to staging/prod
- **Rollout** is environment-specific: applies Terraform changes to specific target
- Enables: Build once → Release once → Rollout many times (dev → staging → prod)

## Overview

Deployments are a new first-class concept in Atmos that enable reproducible, efficient application lifecycle management from build through production rollout. Like vendoring, deployments become a core primitive with their own configuration schema, CLI commands, and workflows.

A deployment defines an isolated subset of stacks and components that represent a complete application or service, enabling Atmos to process only the relevant configuration files rather than scanning the entire repository. This dramatically improves performance and creates clear boundaries for build, release, and rollout operations.

## Problem Statement

### Current State
- Atmos processes all stacks and components on every operation, scanning the entire repository
- Vendoring is repository-wide; teams must vendor all components even if only using a subset
- No standard pattern for application lifecycle (build → test → release → rollout)
- No way to create reproducible, versioned artifacts that capture both application code and infrastructure configuration
- Teams build custom workflows around Atmos for container builds, releases, and deployments
- Rollbacks require manual coordination across multiple systems

### Desired State
- Define isolated deployments that only load required stacks/components
- Just-in-time vendoring that pulls only components needed by a deployment
- First-class support for nixpacks with Dockerfile escape hatch
- Reproducible builds locally and in CI with identical results
- Immutable release artifacts that can be promoted across environments
- Infrastructure-native rollouts that update existing Terraform/Helm/Lambda components
- Optional fully self-contained OCI artifacts for point-in-time deployments and rollbacks

## Goals

### Must Have (P0)
1. **Deployment Configuration**: YAML schema defining deployments with stacks, components, targets
2. **Performance Optimization**: Only load/process files listed in deployment stacks (not entire repo)
3. **JIT Vendoring**: Vendor only the components referenced by a deployment on-demand
4. **Build Stage**: Reproducible container builds via nixpacks Go SDK (no shelling out)
5. **Release Stage**: Create immutable release records with image digests, SBOM, provenance
6. **Rollout Stage**: Update infrastructure components (Terraform/Helm/Lambda) with new digests
7. **Migration Command**: Generate deployment definitions from existing top-level stacks
8. **Discovery Commands**: List deployments, show deployment details
9. **Target Support**: Environment-specific configuration (dev/staging/prod) within deployments

### Should Have (P1)
10. **Test Stage**: Run tests inside built containers for environment parity
11. **Label-Based Binding**: Auto-wire nixpack outputs to infrastructure consumers via labels
12. **Promotion**: Deploy same release artifact to multiple targets without rebuilding
13. **OCI Bundle Artifacts**: Optionally bundle complete deployment context into OCI artifact
14. **Rollback Support**: Deploy previous release artifacts to any target
15. **SBOM Generation**: Software Bill of Materials for security/compliance
16. **Provenance Tracking**: Build metadata for supply chain security
17. **Vendor Caching**: Cache vendored components per deployment for faster subsequent operations
18. **Native CI/CD Integration**: Zero-bash workflows in GitHub Actions, GitLab CI via provider abstraction
19. **Local Iteration**: Same `atmos deployment` commands work locally and in CI
20. **Simplified Workflows**: Matrix generation, approvals, job summaries built-in

### Nice to Have (P2)
21. **Dockerfile Escape**: Use Dockerfile when nixpacks doesn't meet requirements
22. **Multi-Component Deployments**: Coordinate releases across multiple nixpack components
23. **Deployment Status**: Track which releases are deployed to which targets
24. **Drift Detection**: Identify when infrastructure differs from expected release
25. **Vendor Garbage Collection**: Clean up unused vendored components
26. **Additional CI/CD Providers**: GitLab CI, Bitbucket Pipelines, Azure DevOps, CircleCI

### Local Development & Iteration
- **Same commands locally and in CI**: `atmos deployment build/test/rollout` work identically
- **Fast feedback loops**: Build/test locally before pushing to CI
- **No CI lock-in**: Test entire deployment workflow on laptop
- **Reproducible builds**: Same inputs → same outputs (container digests)

### Simplified CI/CD Workflows
- **Zero bash scripting**: Native Atmos commands replace custom CI glue code
- **Auto-generated matrices**: DAG-aware parallel component deployment
- **Built-in approvals**: Atmos Pro integration for deployment gates
- **Job summaries**: Rich deployment reports in CI UI automatically
- **Provider-agnostic**: Same workflow patterns across GitHub/GitLab/etc.

## Non-Goals

- **Not a CI replacement**: Atmos runs in CI but doesn't become a CI orchestrator
- **Not a new infrastructure primitive**: Rollouts always update existing Terraform/Helm/Lambda components
- **Not a test framework**: Teams bring their own test commands/frameworks
- **Not replacing stacks**: Deployments reference stacks, don't replace them
- **Not auto-inheritance**: Targets use explicit `context` for overrides, not automatic inheritance
- **Not workflow generation**: We provide native integration, not CI config file generators

## Architecture

### High-Level Design

```
Deployment Definition (YAML)
├── Metadata (name, labels, stacks)
├── Components
│   ├── Nixpack (nixpacks-based container builds)
│   ├── Terraform (infrastructure)
│   ├── Helmfile (Kubernetes)
│   └── Lambda (serverless)
├── Targets (dev/staging/prod)
│   ├── Labels (for binding)
│   └── Context (overrides)
├── Vendoring (JIT on-demand)
│   ├── Component Discovery
│   ├── Selective Pull
│   └── Deployment-Scoped Cache
└── Release Records
    ├── Image Digests
    ├── SBOM
    ├── Provenance
    └── Metadata
```

### File Structure

```
deployments/
├── api.yaml                    # Single app deployment
├── background-worker.yaml      # Another deployment
└── platform.yaml               # Shared infrastructure deployment

releases/
├── api/
│   ├── dev/
│   │   ├── release-abc123.yaml
│   │   └── release-def456.yaml
│   ├── staging/
│   └── prod/
└── worker/
    └── ...
```

## Implementation Phases

### Phase 1: Core Infrastructure (4-6 weeks)

**Deliverables**:
- Deployment YAML schema and validation
- Basic CLI commands (`list`, `describe`, `migrate`)
- JIT vendoring for deployments
- Vendor cache architecture
- Performance: only load stacks listed in deployment

**Success Criteria**:
- Can create deployment YAML manually
- `atmos deployment list` shows all deployments
- `atmos deployment describe` shows configuration
- JIT vendoring pulls only referenced components
- 10-20x performance improvement vs. scanning entire repo

### Phase 2: Build & Release (4-6 weeks)

**Deliverables**:
- Nixpack component type implementation
- `atmos deployment build` command
- `atmos deployment release` command
- Release record YAML generation
- Label-based component binding

**Success Criteria**:
- Can build container images via nixpacks
- Builds work identically locally and in CI
- Release records capture image digests + metadata
- Can list releases per deployment/target

### Phase 3: Rollout & SBOM (4-6 weeks)

**Deliverables**:
- `atmos deployment rollout` command
- Automatic image digest injection into Terraform/Helm
- SBOM generation (CycloneDX format)
- Component registry SBOM pattern
- Concurrent workspace execution

**Success Criteria**:
- Can rollout releases to update infrastructure
- Terraform components use correct image digests
- SBOM generated for each component type
- Components execute in parallel with `--parallelism`

### Phase 4: CI/CD Integration (4-6 weeks)

**Deliverables**:
- VCS provider abstraction (GitHub, GitLab)
- CI/CD provider abstraction (GitHub Actions, GitLab CI)
- Matrix generation for parallel deployment
- Atmos Pro approval integration
- Zero-bash GitHub Actions workflows

**Success Criteria**:
- Can deploy from GitHub Actions with no bash
- Matrix strategy parallelizes component deployment
- Atmos Pro approvals gate production rollouts
- Job summaries show deployment status

## Performance Impact

### Current Behavior

```
atmos terraform plan component -s stack
  → Load atmos.yaml
  → Scan ALL stacks/** (100s of files)
  → Process ALL imports recursively
  → Vendor ALL components (even unused ones)
  → Build component configuration
  → Execute terraform plan
```

**Performance**: 10-20 seconds for large repositories

### With Deployments + JIT Vendoring

```
atmos deployment rollout api --target dev
  → Load atmos.yaml
  → Load deployments/api.yaml
  → Scan ONLY deployment.stacks (5-10 files)
  → Process ONLY relevant imports
  → Vendor ONLY referenced components (JIT)
  → Build component configuration
  → Execute terraform plan
```

**Performance**: 1-2 seconds (10-20x improvement)

**Savings**:
- **File scanning**: 100+ files → 5-10 files
- **Vendoring**: All components → Only referenced components
- **Import processing**: Entire tree → Deployment-scoped tree

## See Also

- **[configuration.md](./configuration.md)** - Complete deployment YAML schema and examples
- **[vendoring.md](./vendoring.md)** - JIT vendoring strategy and vendor cache design
- **[nixpacks.md](./nixpacks.md)** - Nixpack component integration and build process
- **[sbom.md](./sbom.md)** - SBOM generation and component registry pattern
- **[concurrent-execution.md](./concurrent-execution.md)** - Workspace isolation and parallel execution
- **[cicd-integration.md](./cicd-integration.md)** - VCS/CI/CD provider abstraction and GitHub Actions
- **[cli-commands.md](./cli-commands.md)** - Complete command reference
