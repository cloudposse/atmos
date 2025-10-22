# Atmos Deployments PRD

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
- First-class support for Cloud Native Buildpacks (CNB) with Dockerfile escape hatch
- Reproducible builds locally and in CI with identical results
- Immutable release artifacts that can be promoted across environments
- Infrastructure-native rollouts that update existing Terraform/Helm/Lambda components
- Optional fully self-contained OCI artifacts for point-in-time deployments and rollbacks

## Goals

### Must Have (P0)
1. **Deployment Configuration**: YAML schema defining deployments with stacks, components, targets
2. **Performance Optimization**: Only load/process files listed in deployment stacks (not entire repo)
3. **JIT Vendoring**: Vendor only the components referenced by a deployment on-demand
4. **Build Phase**: Reproducible container builds via CNB Go SDK (no shelling out)
5. **Release Phase**: Create immutable release records with image digests, SBOM, provenance
6. **Rollout Phase**: Update infrastructure components (Terraform/Helm/Lambda) with new digests
7. **Migration Command**: Generate deployment definitions from existing top-level stacks
8. **Discovery Commands**: List deployments, show deployment details
9. **Target Support**: Environment-specific configuration (dev/staging/prod) within deployments

### Should Have (P1)
10. **Test Phase**: Run tests inside built containers for environment parity
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

## Goals

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

## Design

### Architecture

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

### Deployment Schema

```yaml
# deployments/api.yaml
deployment:
  name: api
  description: "REST API service for application backend"
  labels:
    service: api
    team: backend
    channel: stable

  # Only these stacks are loaded when processing this deployment
  stacks:
    - "platform/vpc"
    - "platform/eks"
    - "ecr"
    - "ecs"

  context:
    default_target: dev
    promote_by: digest  # or 'tag'

  # Vendor configuration with environment-specific versions
  vendor:
    components:
      # Dev/staging use latest (bleeding edge) for testing
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "1.3.0"  # Latest version
        targets: ["ecs/service-api"]
        labels:
          environment: ["dev", "staging"]

      # Production uses stable, battle-tested version
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "1.2.5"  # Stable version (2 releases behind)
        targets: ["ecs/service-api"]
        labels:
          environment: ["prod"]

      # All environments use same version of ECR component
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecr"
        version: "0.5.0"
        targets: ["ecr/api"]

    auto_discover: true

  components:
    nixpack:
      api:
        metadata:
          labels:
            service: api
            tier: backend
          depends_on:
            - terraform/ecr/api
        vars:
          source: "./services/api"
          # nixpacks auto-detects: Go, Node.js, Python, Rust, etc.
          # Optional overrides:
          install_cmd: "go mod download"  # optional
          build_cmd: "go build -o main ."  # optional
          start_cmd: "./main"              # optional
          pkgs: ["ffmpeg", "imagemagick"]  # additional Nix packages
          apt_pkgs: ["curl"]               # additional apt packages (if needed)
          image:
            registry: "123456789012.dkr.ecr.us-east-1.amazonaws.com"
            name: "api"
            tag: "{{ git.sha }}"  # advisory; rollout pins by digest
        settings:
          nixpack:
            publish: true
            dockerfile_escape: false  # use Dockerfile if present
            sbom:
              require: true
              formats: ["cyclonedx-json", "spdx-json"]

    terraform:
      ecr/api:
        metadata:
          labels:
            service: api
        vars:
          name: "api"
          image_scanning: true
          lifecycle_policy:
            keep_last: 10

      ecs/taskdef-api:
        metadata:
          labels:
            service: api  # binds to nixpack component
        vars:
          family: "api"
          cpu: 512
          memory: 1024
          container:
            name: "api"  # MUST match nixpack component name
            port: 8080
            env:
              - name: PORT
                value: "8080"
              - name: LOG_LEVEL
                value: "info"
            secrets:
              - name: DATABASE_URL
                valueFrom: "arn:aws:secretsmanager:...:secret:db-url"
            healthcheck:
              command: ["CMD-SHELL", "curl -f http://localhost:8080/health || exit 1"]
              interval: 10
              timeout: 5
              retries: 3
          roles:
            execution_role_arn: "arn:aws:iam::123:role/ecsExecutionRole"
            task_role_arn: "arn:aws:iam::123:role/ecsTaskRole"

      ecs/service-api:
        metadata:
          labels:
            service: api
        vars:
          cluster_name: "app-ecs"
          desired_count: 1
          launch_type: "FARGATE"
          network:
            subnets: ["subnet-aaa", "subnet-bbb"]
            security_groups: ["sg-xyz"]
          load_balancer:
            target_group_arn: "arn:aws:elasticloadbalancing:..."
            container_name: "api"
            container_port: 8080

  targets:
    dev:
      labels:
        environment: dev
      context:
        cpu: 256
        memory: 512
        replicas: 1
        log_level: "debug"

    staging:
      labels:
        environment: staging
      context:
        cpu: 512
        memory: 1024
        replicas: 2
        log_level: "info"

    prod:
      labels:
        environment: prod
      context:
        cpu: 1024
        memory: 2048
        replicas: 4
        autoscale:
          enabled: true
          min: 4
          max: 16
          cpu_target: 45
        log_level: "warn"
```

### Release Record Schema

```yaml
# releases/api/dev/release-abc123.yaml
release:
  id: "abc123"
  deployment: api
  target: dev
  created_at: "2025-01-15T10:30:00Z"
  created_by: "ci@example.com"
  git:
    sha: "abc123def456"
    branch: "main"
    tag: "v1.2.3"

  artifacts:
    api:
      type: nixpack
      digest: "sha256:1234567890abcdef..."
      registry: "123456789012.dkr.ecr.us-east-1.amazonaws.com"
      repository: "api"
      tag: "v1.2.3"  # advisory
      sbom:
        - format: "cyclonedx-json"
          digest: "sha256:sbom123..."
      provenance:
        builder: "nixpacks"
        nixpacks_version: "1.21.0"
        detected_provider: "go"
        nix_packages:
          - "go_1_21"
          - "ffmpeg"
          - "imagemagick"

  annotations:
    description: "Add user authentication feature"
    pr: "#482"
    jira: "PROJ-123"

  status: active  # active, superseded, rolled_back
```

### Component Binding Strategy

**Label-Based Binding (Primary)**:
1. Match `deployment.labels` with `component.metadata.labels`
2. Match `target.labels` with `component.metadata.labels`
3. If both match, component is bound to that nixpack output

**Name-Based Fallback**:
1. If `nixpack.vars.image.name` matches `terraform.vars.image.name`
2. Bind nixpack digest to that component

**Explicit Binding**:
```yaml
components:
  terraform:
    ecs/taskdef-api:
      vars:
        image: "{{ nixpack.api.digest }}"  # explicit reference
```

### Performance Optimization Strategy

**Current Behavior**:
```
atmos terraform plan component -s stack
  → Load atmos.yaml
  → Scan ALL stacks/** (100s of files)
  → Process ALL imports recursively
  → Vendor ALL components (even unused ones)
  → Build component configuration
  → Execute terraform plan
```

**With Deployments + JIT Vendoring**:
```
atmos rollout plan api --target dev
  → Load atmos.yaml
  → Load deployments/api.yaml
  → Scan ONLY deployment.stacks (5-10 files)
  → Process ONLY relevant imports
  → Vendor ONLY referenced components (JIT)
  → Build component configuration
  → Execute terraform plan
```

**Performance Impact**:
- Load time: 10-20s → 1-2s (10-20x improvement)
- Memory usage: 500MB → 50MB (10x reduction)
- Vendor time: 30-60s → 3-5s (10x improvement)
- Disk usage: vendor entire repo → vendor only deployment needs (10-50x reduction)
- Enables sub-second feedback for common operations

### Just-In-Time (JIT) Vendoring

JIT vendoring dramatically improves performance by vendoring only the components required by a specific deployment, rather than vendoring the entire component catalog.

**Key Concepts**:

1. **Deployment-Scoped Vendoring**: Each deployment declares its component dependencies. Atmos vendors only those components when the deployment is processed.

2. **Lazy Evaluation**: Components are vendored on first use, not during repository initialization.

3. **Vendor Cache**: Centralized cache (`.atmos/vendor-cache/`) stores vendored components, indexed by source URL and version/tag. Multiple deployments share cached components.

4. **Environment-Specific Vendoring**: Use labels/tags to vendor different component versions per environment (dev/staging/prod).

5. **Garbage Collection**: Unused vendored components can be cleaned up automatically based on age and usage.

**Vendoring Workflow**:

```
Traditional Vendoring:
  atmos vendor pull
    → Parse vendor.yaml
    → Pull ALL components (50-100+ components)
    → Write to components/**
    → 30-60 seconds

JIT Vendoring with Deployments:
  atmos rollout plan api --target dev
    → Load deployments/api.yaml
    → Identify referenced components (3-5 components)
    → Check vendor cache (.atmos/vendor-cache/deployments/api/)
    → Pull missing components only
    → Write to deployment-scoped vendor dir
    → 3-5 seconds
```

**Deployment Vendor Configuration**:

Deployments support **three vendor configuration strategies**:

**Option 1: Inline Vendor Configuration**

Declare vendor dependencies directly in the deployment file:

```yaml
# deployments/api.yaml
deployment:
  name: api
  stacks:
    - "platform/vpc"
    - "ecs"

  # Inline vendor configuration
  vendor:
    components:
      # Vendor from component catalog
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "1.2.3"
        targets: ["ecs/service-api"]
        labels:
          environment: ["dev", "staging"]  # Only vendor for dev/staging

      # Vendor from registry with environment-specific versions
      - source: "registry.terraform.io/cloudposse/ecs-service/aws"
        version: "~> 2.0"
        targets: ["ecs/taskdef-api"]
        labels:
          environment: ["dev", "staging"]

      # Production uses different (stable) version
      - source: "registry.terraform.io/cloudposse/ecs-service/aws"
        version: "2.1.5"  # pinned version for prod
        targets: ["ecs/taskdef-api"]
        labels:
          environment: ["prod"]

    # Or auto-discover from component references
    auto_discover: true  # default: true

    # Vendor cache configuration
    cache:
      enabled: true  # default: true
      dir: ".atmos/vendor-cache"  # centralized cache location
      ttl: 24h  # re-check sources after 24 hours
      strategy: "content-addressable"  # or "source-versioned"
```

**Option 2: External vendor.yaml Reference**

Reference an external vendor manifest file:

```yaml
# deployments/api.yaml
deployment:
  name: api
  stacks:
    - "platform/vpc"
    - "ecs"

  # Reference external vendor manifest
  vendor:
    manifest: "vendor/api-vendor.yaml"  # Path relative to repo root
    auto_discover: true
    cache:
      enabled: true
```

```yaml
# vendor/api-vendor.yaml
apiVersion: atmos/v1
kind: VendorManifest
metadata:
  name: api-components

spec:
  components:
    - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
      version: "1.2.3"
      targets: ["ecs/service-api"]
      labels:
        environment: ["dev", "staging"]

    - source: "registry.terraform.io/cloudposse/ecs-service/aws"
      version: "2.1.5"
      targets: ["ecs/taskdef-api"]
      labels:
        environment: ["prod"]
```

**Option 3: Repository-Wide vendor.yaml Fallback**

Use the existing repository-wide `vendor.yaml` as fallback:

```yaml
# deployments/api.yaml
deployment:
  name: api
  stacks:
    - "platform/vpc"
    - "ecs"

  # Use repository-wide vendor.yaml (no vendor config needed)
  # Atmos will automatically filter vendor.yaml entries based on:
  # 1. Components referenced by this deployment
  # 2. Labels matching deployment targets
```

```yaml
# vendor.yaml (repository root)
apiVersion: atmos/v1
kind: VendorManifest
metadata:
  name: repository-components

spec:
  components:
    # This will be auto-discovered if deployment references ecs-service
    - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
      version: "1.2.3"
      targets: ["components/terraform/ecs-service"]
      labels:
        environment: ["dev", "staging"]

    - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
      version: "2.1.5"
      targets: ["components/terraform/ecs-service"]
      labels:
        environment: ["prod"]

    # This won't be vendored (not used by api deployment)
    - source: "github.com/cloudposse/terraform-aws-components//modules/eks-cluster"
      version: "1.0.0"
      targets: ["components/terraform/eks"]
```

**Configuration Resolution Order**:

1. **Inline vendor configuration** in deployment file (highest priority)
2. **External vendor manifest** referenced by `vendor.manifest`
3. **Repository-wide vendor.yaml** (fallback, auto-filtered)
4. **Auto-discovery** from component references (if enabled)

**Mixing Strategies**:

You can combine strategies - inline config takes precedence:

```yaml
# deployments/api.yaml
deployment:
  name: api
  vendor:
    # Load base configuration from external manifest
    manifest: "vendor/base-components.yaml"

    # Override/extend with inline config (takes precedence)
    components:
      # This overrides matching entry from base-components.yaml
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "1.3.0-beta.1"  # Override version for testing
        labels:
          environment: ["dev"]

    auto_discover: true  # Still auto-discover missing components
```

**Auto-Discovery of Vendored Components**:

When `vendor.auto_discover: true` (default), Atmos scans the deployment's component definitions and automatically identifies external dependencies:

```yaml
components:
  terraform:
    ecs/service-api:
      component: "ecs-service"  # Auto-discovered: needs vendoring
      vars:
        cluster_name: "app-ecs"
```

Atmos checks:
1. Does `components/terraform/ecs-service/` exist locally?
2. If not, is it defined in `vendor.yaml` or deployment vendor config?
3. If yes, check vendor cache for matching content (by digest)
4. If not cached, pull and store in content-addressable cache
5. Create symlink/reference from deployment workspace to cached content

### Vendor Cache Architecture

The vendor cache is a **centralized, content-addressable store** that eliminates duplication and enables efficient sharing across deployments and environments.

**Cache Structure**:

**Note**: The cache uses **hard links on Unix/Mac and file copies on Windows** for cross-platform compatibility. Symlinks are NOT used due to Windows admin privilege requirements.

```
.atmos/
└── vendor-cache/
    ├── objects/
    │   └── sha256/
    │       ├── abc123.../          # Content-addressable storage
    │       │   ├── main.tf
    │       │   ├── variables.tf
    │       │   └── outputs.tf
    │       ├── def456.../
    │       └── xyz789.../
    ├── deployments/
    │   ├── api/
    │   │   ├── dev/
    │   │   │   ├── terraform/
    │   │   │   │   └── ecs-service/        # Hard link or copy from objects/
    │   │   │   │       ├── main.tf
    │   │   │   │       ├── variables.tf
    │   │   │   │       └── outputs.tf
    │   │   │   └── .vendor-lock.yaml
    │   │   ├── staging/
    │   │   │   └── terraform/
    │   │   │       └── ecs-service/        # Same content, shared via hard link
    │   │   └── prod/
    │   │       └── terraform/
    │   │           └── ecs-service/        # Different version, different content
    │   └── worker/
    │       └── dev/
    └── .cache-index.yaml  # Global cache metadata
```

**Cross-Platform File Sharing Strategy**:

1. **Unix/Linux/macOS**: Use hard links from `objects/sha256/<digest>/` to deployment workspace
   - Space efficient (same inode, zero duplication)
   - Transparent to tools (looks like regular files)
   - No special privileges required

2. **Windows**: Copy files from cache to deployment workspace
   - Fallback when hard links fail
   - Slight disk usage increase, but still manageable
   - Cache cleanup reclaims space

3. **Implementation**: Detect OS and attempt hard link first, fall back to copy
   ```go
   // Try hard link first (Unix/Mac/Windows NTFS)
   err := os.Link(sourcePath, destPath)
   if err != nil {
       // Fall back to copy (Windows FAT32, network drives)
       err = copyFile(sourcePath, destPath)
   }
   ```

**Benefits of Content-Addressable Cache**:

1. **Deduplication**: Same component version stored once, referenced by multiple deployments
2. **Environment Isolation**: Dev/staging/prod can use different versions without conflicts
3. **Fast Switching**: Changing versions is instant (symlink update)
4. **Disk Efficiency**: 100 deployments using same component = 1x storage
5. **Garbage Collection**: Easy to identify unreferenced objects

**Cache Index**:

```yaml
# .atmos/vendor-cache/.cache-index.yaml
version: 1
generated_at: "2025-01-15T10:30:00Z"

objects:
  sha256:abc123...:
    size: 45678
    refs:
      - github.com/cloudposse/terraform-aws-components//modules/ecs-service@1.2.3
    deployments:
      - api/dev
      - api/staging
      - worker/dev
    last_accessed: "2025-01-15T10:30:00Z"
    created_at: "2025-01-10T08:00:00Z"

  sha256:def456...:
    size: 50123
    refs:
      - github.com/cloudposse/terraform-aws-components//modules/ecs-service@1.2.4
    deployments:
      - api/staging
    last_accessed: "2025-01-14T15:20:00Z"
    created_at: "2025-01-12T09:00:00Z"

refs:
  github.com/cloudposse/terraform-aws-components//modules/ecs-service:
    versions:
      1.2.3: sha256:abc123...
      1.2.4: sha256:def456...
      latest: 1.2.4
```

**Deployment Lock File**:

```yaml
# .atmos/vendor-cache/deployments/api/dev/.vendor-lock.yaml
deployment: api
target: dev
generated_at: "2025-01-15T10:30:00Z"

components:
  terraform/ecs-service:
    source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
    version: "1.2.3"
    digest: "sha256:abc123..."
    cache_path: ".atmos/vendor-cache/objects/sha256/abc123..."
    pulled_at: "2025-01-15T10:30:00Z"
    labels:
      environment: dev

  terraform/ecs-taskdef:
    source: "registry.terraform.io/cloudposse/ecs-service/aws"
    version: "2.1.0"
    digest: "sha256:def456..."
    cache_path: ".atmos/vendor-cache/objects/sha256/def456..."
    pulled_at: "2025-01-15T10:30:00Z"
    labels:
      environment: dev
```

**CLI Commands for JIT Vendoring**:

```bash
# Vendor all components for a deployment (all targets)
atmos vendor pull --deployment api
# Output:
# Discovering components for deployment 'api'...
# Found 3 components across 3 targets (dev, staging, prod)
# ✓ terraform/ecs-service@1.2.3 (dev, staging) → sha256:abc123... [cached]
# ✓ terraform/ecs-service@2.1.5 (prod) → sha256:xyz789... [pulled]
# ✓ terraform/ecs-taskdef@2.1.0 (dev, staging) → sha256:def456... [cached]
# ✓ terraform/ecs-taskdef@2.1.5 (prod) → sha256:aaa111... [cached]
# Cache usage: 180 MB (3 unique objects, 6 deployment refs)

# Vendor for specific target (environment)
atmos vendor pull --deployment api --target dev
# Output:
# Discovering components for deployment 'api' target 'dev'...
# Found 2 components
# ✓ terraform/ecs-service@1.2.3 → sha256:abc123... [cached]
# ✓ terraform/ecs-taskdef@2.1.0 → sha256:def456... [cached]
# Created workspace: .atmos/vendor-cache/deployments/api/dev/

# Show vendor status for deployment (all targets)
atmos vendor status --deployment api
# Output:
# TARGET    COMPONENT                  VERSION   DIGEST      STATUS    AGE
# dev       terraform/ecs-service      1.2.3     abc123...   cached    2h
# dev       terraform/ecs-taskdef      2.1.0     def456...   cached    2h
# staging   terraform/ecs-service      1.2.4     ghi789...   cached    1d
# staging   terraform/ecs-taskdef      2.1.0     def456...   cached    1d
# prod      terraform/ecs-service      2.1.5     xyz789...   cached    7d
# prod      terraform/ecs-taskdef      2.1.5     aaa111...   outdated  30d (2.1.6 available)

# Show vendor status for specific target
atmos vendor status --deployment api --target prod

# Show cache statistics
atmos vendor cache stats
# Output:
# Vendor Cache Statistics
#
# Total size: 2.3 GB
# Unique objects: 145
# Deployment workspaces: 23
# Total references: 378
#
# Top 5 components by size:
#   1. eks-cluster (250 MB, 8 refs)
#   2. rds-instance (180 MB, 12 refs)
#   3. vpc (120 MB, 15 refs)
#   4. ecs-service (90 MB, 25 refs)
#   5. lambda-function (60 MB, 18 refs)
#
# Deduplication savings: 8.7 GB (78% reduction)

# Update vendored components for specific target
atmos vendor pull --deployment api --target prod --update

# Clean unused vendor cache (automatic GC)
atmos vendor clean
# Output:
# Analyzing vendor cache...
#
# Unreferenced objects (not used by any deployment):
#   sha256:old123... (45 MB, last used 180d ago)
#   sha256:old456... (60 MB, last used 200d ago)
#
# Stale deployment workspaces (deployment no longer exists):
#   old-service/dev (80 MB)
#   old-service/prod (80 MB)
#
# Total reclaimable: 265 MB
# Clean? (y/N)

# Force clean specific deployment cache (all targets)
atmos vendor clean --deployment old-service --force

# Clean specific target only
atmos vendor clean --deployment api --target staging --force

# Prune unreferenced objects older than 30 days
atmos vendor clean --prune --older-than 30d
```

**Integration with Existing Vendoring**:

JIT vendoring is **opt-in per deployment**. Existing `atmos vendor pull` continues to work for repository-wide vendoring:

```bash
# Traditional: vendor everything from vendor.yaml
atmos vendor pull

# New: vendor only for specific deployment
atmos vendor pull --deployment api

# Vendor specific deployment + target
atmos vendor pull --deployment api --target prod
```

**Backwards Compatibility**:

1. **Existing vendor.yaml works unchanged**:
   - Repository-wide `vendor.yaml` continues to function as before
   - Deployments without vendor config automatically use `vendor.yaml` entries (filtered by component references)
   - `atmos vendor pull` (without `--deployment`) behaves identically to current behavior

2. **Gradual migration path**:
   ```yaml
   # Phase 1: Use existing vendor.yaml (no changes needed)
   deployment:
     name: api
     # No vendor config - falls back to vendor.yaml

   # Phase 2: Add deployment-specific vendor manifest
   deployment:
     name: api
     vendor:
       manifest: "vendor/api-vendor.yaml"  # Split from monolithic vendor.yaml

   # Phase 3: Add environment-specific versions
   deployment:
     name: api
     vendor:
       manifest: "vendor/api-vendor.yaml"
       components:  # Override for specific environments
         - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
           version: "1.3.0-beta.1"
           labels:
             environment: ["dev"]
   ```

3. **Vendor cache is shared**:
   - Traditional `atmos vendor pull` writes to `components/**` (unchanged)
   - Deployment vendoring writes to `.atmos/vendor-cache/` (new, content-addressable)
   - Both can coexist without conflicts
   - Teams can use both approaches simultaneously during migration

4. **Component resolution order**:
   ```
   When resolving component "ecs-service":
   1. Check local components/terraform/ecs-service/ (committed to repo)
   2. Check deployment vendor cache (.atmos/vendor-cache/deployments/api/dev/)
   3. Check traditional vendor cache (components/terraform/ecs-service/ vendored globally)
   4. If not found, attempt auto-discovery and JIT vendor
   ```

**Environment-Specific Vendoring Use Cases**:

JIT vendoring with labels enables sophisticated environment-specific component versioning strategies:

**Use Case 1: Progressive Rollout**
```yaml
vendor:
  components:
    # Dev/staging use latest (unstable) version for testing
    - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
      version: "1.3.0-beta.1"
      labels:
        environment: ["dev", "staging"]

    # Production uses stable version
    - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
      version: "1.2.5"
      labels:
        environment: ["prod"]
```

**Use Case 2: Cost Optimization in Non-Prod**
```yaml
vendor:
  components:
    # Dev uses lightweight component variant
    - source: "github.com/internal/rds-cluster-dev"
      version: "1.0.0"
      labels:
        environment: ["dev"]

    # Production uses full-featured component
    - source: "github.com/internal/rds-cluster-prod"
      version: "2.1.0"
      labels:
        environment: ["prod"]
```

**Use Case 3: Multi-Region with Regional Variations**
```yaml
vendor:
  components:
    # US region component
    - source: "github.com/internal/compliance-us"
      version: "1.0.0"
      labels:
        region: ["us-east-1", "us-west-2"]

    # EU region component (GDPR compliance)
    - source: "github.com/internal/compliance-eu"
      version: "1.0.0"
      labels:
        region: ["eu-west-1", "eu-central-1"]
```

**Use Case 4: Feature Flags via Labels**
```yaml
vendor:
  components:
    # Feature preview for canary deployments
    - source: "github.com/internal/api-gateway"
      version: "2.0.0"
      labels:
        feature: "new-auth-system"
        environment: ["dev"]

    # Stable version for production
    - source: "github.com/internal/api-gateway"
      version: "1.5.0"
      labels:
        environment: ["prod"]
```

**Performance Comparison**:

```
Scenario: Monorepo with 100 components, deployment uses 5, 3 environments

Traditional Vendoring:
  Initial:     atmos vendor pull → 60s, 500 MB
  Subsequent:  Cached, but 500 MB on disk
  Per-env:     N/A (same components for all environments)

JIT Vendoring with Environment-Specific Versions:
  Initial:     atmos vendor pull --deployment api → 8s, 40 MB
  Subsequent:  Cached (content-addressable deduplication)
  dev:         5 components @ v1.3.x → 25 MB (latest/unstable)
  staging:     5 components @ v1.3.x → 25 MB (shared with dev, 0 MB additional)
  prod:        5 components @ v1.2.x → 22 MB (stable, different digest)
  Total:       47 MB (3 unique versions, 6 deployment refs)

Multi-Deployment Scenario (10 deployments × 3 environments):
  Traditional: 500 MB total (shared, all envs use same versions)
  JIT Content-Addressable:
    - Unique component versions: ~50 (mix of stable/latest per env)
    - Total storage: ~300 MB
    - Deployment refs: 150 (10 deployments × 3 targets × 5 components avg)
    - Deduplication savings: 70% (would be 1 GB without content-addressing)
```

**OCI Bundle Integration**:

When creating OCI bundle artifacts, vendored components are included:

```bash
atmos deployment release api --target dev --bundle
# Creates OCI artifact containing:
# - Built container images (digests)
# - Deployment configuration
# - Stack configurations
# - Vendored components (from JIT cache)
# - Release metadata

# Result: fully self-contained artifact for rollback
atmos deployment rollout api --target prod --release abc123 --bundle oci://registry/api:release-abc123
```

**Benefits of JIT Vendoring with Content-Addressable Cache**:

1. **Speed**: 10x faster vendoring by pulling only what's needed
2. **Disk Space**: 10-50x reduction through content-addressable deduplication
3. **Environment Isolation**: Dev/staging/prod use different component versions via labels
4. **Reproducibility**: Lock files ensure consistent component versions per environment
5. **Parallelization**: Multiple deployments can vendor concurrently
6. **Cleanup**: Automatic garbage collection of unreferenced objects
7. **Monorepo-Friendly**: Scale to hundreds of components without performance degradation
8. **Version Flexibility**: Progressive rollout strategies (beta in dev, stable in prod)
9. **Cache Sharing**: Same component version used by multiple deployments = 1x storage

## Concurrent Deployment Architecture

### Dependency DAG and Parallelism

Deployments support **concurrent component processing** using the existing Atmos dependency DAG (`depends_on` field). Components are executed in topological order with configurable parallelism.

**Key Concepts**:

1. **Dependency DAG**: Components declare dependencies via `metadata.depends_on`
2. **Topological Ordering**: DAG ensures dependencies are processed before dependents
3. **Concurrent Execution**: Multiple independent components run in parallel
4. **Workspace Isolation**: Each concurrent component gets isolated temporary workspace
5. **Automatic Cleanup**: Temporary workspaces cleaned up after execution

**Dependency Graph Example**:

```yaml
# deployments/payment-service.yaml
deployment:
  name: payment-service
  components:
    terraform:
      vpc:
        # No dependencies - can run immediately
        vars:
          cidr: "10.0.0.0/16"

      security-group:
        metadata:
          depends_on:
            - terraform/vpc  # Requires VPC outputs
        vars:
          vpc_id: "{{ terraform.output('vpc', 'vpc_id') }}"

      rds/payment-db:
        metadata:
          depends_on:
            - terraform/vpc
            - terraform/security-group
        vars:
          vpc_id: "{{ terraform.output('vpc', 'vpc_id') }}"
          security_group_ids: ["{{ terraform.output('security-group', 'id') }}"]

      ecs/cluster:
        metadata:
          depends_on:
            - terraform/vpc
        vars:
          vpc_id: "{{ terraform.output('vpc', 'vpc_id') }}"

    nixpack:
      payment-api:
        metadata:
          depends_on:
            - terraform/ecr  # Requires ECR repository
        vars:
          source: "./services/payment-api"

    terraform:
      ecs/payment-api:
        metadata:
          depends_on:
            - terraform/ecs/cluster
            - terraform/rds/payment-db
            - nixpack/payment-api  # Requires container image digest
        vars:
          cluster_id: "{{ terraform.output('ecs/cluster', 'cluster_id') }}"
          image_digest: "{{ nixpack.payment-api.digest }}"
          db_endpoint: "{{ terraform.output('rds/payment-db', 'endpoint') }}"
```

**Execution DAG** (with `--parallelism 3`):

```
Wave 1 (parallel):
  terraform/vpc

Wave 2 (parallel - up to 3):
  terraform/security-group  (depends: vpc)
  terraform/ecs/cluster     (depends: vpc)
  terraform/ecr             (no deps, queued from wave 1)

Wave 3 (parallel - up to 3):
  terraform/rds/payment-db  (depends: vpc, security-group)
  nixpack/payment-api       (depends: ecr)

Wave 4:
  terraform/ecs/payment-api (depends: cluster, db, nixpack)
```

### Workspace Isolation Strategy

**Problem**: Concurrent component execution requires isolated working directories to prevent conflicts (e.g., Terraform state operations, vendored files, generated configs).

**Solution**: Temporary workspace cloning with automatic cleanup.

**Workspace Structure**:

```
.atmos/
└── workspaces/
    └── deployment-{uuid}/          # Unique per rollout execution
        ├── terraform/
        │   ├── vpc/                # Isolated workspace for vpc component
        │   │   ├── main.tf         # Hard link/copy from vendor cache
        │   │   ├── variables.tf
        │   │   ├── backend.tf      # Generated backend config
        │   │   ├── terraform.tfvars # Generated from component vars
        │   │   └── .terraform/     # Terraform init artifacts
        │   ├── security-group/     # Isolated workspace
        │   └── rds-payment-db/
        ├── nixpack/
        │   └── payment-api/
        └── .cleanup                # Marker for cleanup on exit
```

**Workspace Lifecycle**:

1. **Initialization** (`atmos rollout apply`):
   ```
   - Generate unique deployment UUID
   - Create .atmos/workspaces/deployment-{uuid}/
   - Register cleanup handler (defer)
   ```

2. **Component Execution** (per component, concurrent):
   ```
   - Create component workspace: .atmos/workspaces/{uuid}/{type}/{component}/
   - Hard link/copy vendored files from cache to workspace
   - Generate component-specific files (backend.tf, tfvars, etc.)
   - Execute component (terraform apply, docker build, etc.)
   - Capture outputs for downstream dependencies
   ```

3. **Cleanup** (on success or failure):
   ```
   - Remove entire .atmos/workspaces/deployment-{uuid}/ tree
   - Keep release records and SBOMs (.atmos/sboms/, releases/)
   - Log cleanup errors but don't fail
   ```

**Implementation**:

```go
// pkg/deployment/workspace.go
package deployment

import (
    "context"
    "fmt"
    "os"
    "path/filepath"

    "github.com/google/uuid"
    "github.com/cloudposse/atmos/pkg/schema"
)

// Workspace manages isolated component execution environments.
type Workspace struct {
    ID          string
    RootPath    string
    Deployment  string
    Target      string
    cleanupDone bool
}

// NewWorkspace creates isolated workspace for deployment execution.
func NewWorkspace(deployment, target string) (*Workspace, error) {
    id := uuid.New().String()
    rootPath := filepath.Join(".atmos", "workspaces", fmt.Sprintf("deployment-%s", id))

    if err := os.MkdirAll(rootPath, 0755); err != nil {
        return nil, fmt.Errorf("failed to create workspace: %w", err)
    }

    ws := &Workspace{
        ID:         id,
        RootPath:   rootPath,
        Deployment: deployment,
        Target:     target,
    }

    return ws, nil
}

// GetComponentWorkspace returns isolated workspace path for component.
func (w *Workspace) GetComponentWorkspace(componentType, componentName string) (string, error) {
    workspacePath := filepath.Join(w.RootPath, componentType, componentName)
    if err := os.MkdirAll(workspacePath, 0755); err != nil {
        return "", fmt.Errorf("failed to create component workspace: %w", err)
    }
    return workspacePath, nil
}

// Cleanup removes workspace directory tree.
func (w *Workspace) Cleanup() error {
    if w.cleanupDone {
        return nil
    }

    if err := os.RemoveAll(w.RootPath); err != nil {
        // Log error but don't fail - cleanup is best-effort
        log.Warn("Failed to cleanup workspace", "workspace", w.ID, "error", err)
        return err
    }

    w.cleanupDone = true
    log.Debug("Cleaned up workspace", "workspace", w.ID)
    return nil
}

// PrepareComponentFiles copies/links files from vendor cache to component workspace.
func (w *Workspace) PrepareComponentFiles(
    componentType string,
    componentName string,
    vendorCachePath string,
) error {
    workspacePath, err := w.GetComponentWorkspace(componentType, componentName)
    if err != nil {
        return err
    }

    // Copy/hard-link files from vendor cache to workspace
    return copyOrLinkDirectory(vendorCachePath, workspacePath)
}

// copyOrLinkDirectory uses hard links on Unix/Mac, copies on Windows.
func copyOrLinkDirectory(src, dst string) error {
    return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        // Compute destination path
        relPath, err := filepath.Rel(src, path)
        if err != nil {
            return err
        }
        dstPath := filepath.Join(dst, relPath)

        if info.IsDir() {
            return os.MkdirAll(dstPath, info.Mode())
        }

        // Try hard link first, fall back to copy
        if err := os.Link(path, dstPath); err != nil {
            // Hard link failed (Windows FAT32, cross-device, etc.)
            return copyFile(path, dstPath)
        }

        return nil
    })
}
```

### DAG-Based Concurrent Execution

**Executor** manages parallel component execution respecting dependencies:

```go
// pkg/deployment/executor.go
package deployment

import (
    "context"
    "fmt"
    "sync"

    "golang.org/x/sync/errgroup"
    "github.com/cloudposse/atmos/pkg/component"
)

// Executor manages concurrent component execution with DAG-based ordering.
type Executor struct {
    workspace   *Workspace
    parallelism int
    components  map[string]component.Component
    dag         *DependencyDAG
}

// DependencyDAG represents component dependency graph.
type DependencyDAG struct {
    nodes map[string]*DAGNode
}

type DAGNode struct {
    Name         string
    Component    component.Component
    Dependencies []string
    Dependents   []string
    completed    bool
    mu           sync.Mutex
}

// Execute runs components in topological order with parallelism limit.
func (e *Executor) Execute(ctx context.Context) error {
    // Build dependency graph
    if err := e.dag.Build(e.components); err != nil {
        return fmt.Errorf("failed to build dependency DAG: %w", err)
    }

    // Topological sort to get execution waves
    waves, err := e.dag.TopologicalSort()
    if err != nil {
        return fmt.Errorf("dependency cycle detected: %w", err)
    }

    // Execute waves with parallelism
    for waveNum, wave := range waves {
        log.Info("Executing wave", "wave", waveNum+1, "components", len(wave))

        if err := e.executeWave(ctx, wave); err != nil {
            return fmt.Errorf("wave %d failed: %w", waveNum+1, err)
        }
    }

    return nil
}

// executeWave runs components in parallel with parallelism limit.
func (e *Executor) executeWave(ctx context.Context, components []string) error {
    // Use errgroup with parallelism limit
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(e.parallelism)

    for _, compName := range components {
        compName := compName // Capture for goroutine

        g.Go(func() error {
            return e.executeComponent(ctx, compName)
        })
    }

    return g.Wait()
}

// executeComponent executes single component in isolated workspace.
func (e *Executor) executeComponent(ctx context.Context, name string) error {
    comp := e.components[name]
    node := e.dag.nodes[name]

    log.Info("Executing component", "component", name, "type", comp.GetType())

    // Get isolated workspace for this component
    workspacePath, err := e.workspace.GetComponentWorkspace(comp.GetType(), name)
    if err != nil {
        return err
    }

    // Prepare component files (copy from vendor cache)
    if err := e.workspace.PrepareComponentFiles(
        comp.GetType(),
        name,
        comp.GetVendorCachePath(),
    ); err != nil {
        return fmt.Errorf("failed to prepare workspace: %w", err)
    }

    // Execute component in isolated workspace
    componentCtx := context.WithValue(ctx, "workspace", workspacePath)
    if err := comp.Execute(componentCtx); err != nil {
        return fmt.Errorf("component execution failed: %w", err)
    }

    // Mark as completed
    node.mu.Lock()
    node.completed = true
    node.mu.Unlock()

    log.Info("Component completed", "component", name)
    return nil
}

// Build constructs dependency graph from components.
func (dag *DependencyDAG) Build(components map[string]component.Component) error {
    dag.nodes = make(map[string]*DAGNode)

    // Create nodes
    for name, comp := range components {
        dag.nodes[name] = &DAGNode{
            Name:         name,
            Component:    comp,
            Dependencies: comp.GetDependencies(),
            Dependents:   []string{},
        }
    }

    // Build edges (reverse dependencies)
    for name, node := range dag.nodes {
        for _, depName := range node.Dependencies {
            if depNode, exists := dag.nodes[depName]; exists {
                depNode.Dependents = append(depNode.Dependents, name)
            } else {
                return fmt.Errorf("component %s depends on non-existent component %s", name, depName)
            }
        }
    }

    return nil
}

// TopologicalSort returns execution waves (groups of independent components).
func (dag *DependencyDAG) TopologicalSort() ([][]string, error) {
    waves := [][]string{}
    remaining := make(map[string]*DAGNode)
    inDegree := make(map[string]int)

    // Initialize
    for name, node := range dag.nodes {
        remaining[name] = node
        inDegree[name] = len(node.Dependencies)
    }

    // Extract waves
    for len(remaining) > 0 {
        wave := []string{}

        // Find components with no remaining dependencies
        for name, degree := range inDegree {
            if degree == 0 {
                if _, exists := remaining[name]; exists {
                    wave = append(wave, name)
                }
            }
        }

        if len(wave) == 0 {
            // No progress - cycle detected
            return nil, fmt.Errorf("dependency cycle detected among: %v", remaining)
        }

        // Process wave
        for _, name := range wave {
            delete(remaining, name)
            delete(inDegree, name)

            // Decrement in-degree for dependents
            for _, depName := range dag.nodes[name].Dependents {
                inDegree[depName]--
            }
        }

        waves = append(waves, wave)
    }

    return waves, nil
}
```

**CLI Usage**:

```bash
# Default: sequential execution (parallelism=1)
atmos deployment rollout payment-service --target prod

# Parallel execution: up to 5 components at once
atmos deployment rollout payment-service --target prod --parallelism 5

# Maximum parallelism: unlimited (use with caution)
atmos deployment rollout payment-service --target prod --parallelism 0

# With workspace debugging (don't cleanup on error)
atmos deployment rollout payment-service --target prod --parallelism 5 --keep-workspace
```

**Output Example**:

```
→ Creating deployment workspace: deployment-abc123-def456
→ Building dependency DAG...
  ✓ Found 8 components with 12 dependencies
  ✓ Validated DAG (no cycles)
  ✓ Generated 4 execution waves

→ Wave 1/4: Executing 2 components (parallelism: 5)
  ⠿ terraform/vpc                 [executing]
  ⠿ terraform/ecr                 [executing]
  ✓ terraform/vpc                 [completed in 45s]
  ✓ terraform/ecr                 [completed in 23s]

→ Wave 2/4: Executing 3 components (parallelism: 5)
  ⠿ terraform/security-group      [executing]
  ⠿ terraform/ecs/cluster         [executing]
  ⠿ nixpack/payment-api           [waiting for ecr]
  ✓ terraform/security-group      [completed in 12s]
  ⠿ nixpack/payment-api           [executing]
  ✓ terraform/ecs/cluster         [completed in 34s]
  ✓ nixpack/payment-api           [completed in 67s]

→ Wave 3/4: Executing 2 components (parallelism: 5)
  ⠿ terraform/rds/payment-db      [executing]
  ✓ terraform/rds/payment-db      [completed in 123s]

→ Wave 4/4: Executing 1 component (parallelism: 5)
  ⠿ terraform/ecs/payment-api     [executing]
  ✓ terraform/ecs/payment-api     [completed in 34s]

✓ Deployment completed successfully
  Total: 338s (5m 38s)
  Components: 8
  Waves: 4
  Peak parallelism: 3

→ Cleaning up workspace: deployment-abc123-def456
✓ Workspace cleaned
```

### Benefits

1. **Performance**: 3-5x faster for deployments with independent components
2. **Safety**: DAG prevents race conditions and ensures correct ordering
3. **Isolation**: Temporary workspaces prevent conflicts
4. **Existing Pattern**: Reuses Atmos `depends_on` - no new concepts
5. **Configurable**: Tune parallelism for CI resources (small = 2, large = 10+)
6. **Cross-Platform**: Works on Linux, macOS, Windows

## VCS and CI/CD Provider Abstraction

### Overview

Atmos deployments include **dual provider abstractions** (inspired by `tools/gotcha/pkg/vcs`):

1. **VCS Provider** (`pkg/vcs/`) - Version control operations (PR comments, commit status, releases)
2. **CI/CD Provider** (`pkg/cicd/`) - Automation operations (matrix generation, approvals, job summaries)

These are **separate abstractions** because:
- Some platforms provide both (GitHub = VCS + Actions, GitLab = VCS + CI)
- Some provide only CI/CD (CircleCI, Jenkins, Buildkite, Spacelift)
- Some provide only VCS (Gitea without Actions)
- Allows graceful degradation and specialized integrations

**Success Criteria**: Install Atmos → Run `atmos deployment rollout` directly in CI → Zero bash glue code required.

### VCS Provider Interface

**Core interface definition** (`pkg/vcs/interface.go`, matching Gotcha pattern):

```go
// Platform represents a VCS platform type.
type Platform string

const (
    PlatformGitHub      Platform = "github"
    PlatformGitLab      Platform = "gitlab"
    PlatformBitbucket   Platform = "bitbucket"
    PlatformAzureDevOps Platform = "azuredevops"
    PlatformGitea       Platform = "gitea"
    PlatformUnknown     Platform = "unknown"
)

// Provider is the main VCS provider interface.
type Provider interface {
    // Core functionality
    DetectContext() (Context, error)
    CreateCommentManager(ctx Context) CommentManager

    // Optional capabilities - return nil if not supported
    GetCommitStatusWriter() CommitStatusWriter
    GetReleasePublisher() ReleasePublisher

    // Metadata
    GetPlatform() Platform
    IsAvailable() bool
}

// Context provides VCS-specific context information.
type Context interface {
    GetOwner() string        // GitHub org/user, GitLab namespace
    GetRepo() string         // Repository name
    GetPRNumber() int        // PR/MR number (0 if not PR)
    GetBranch() string       // Current branch
    GetCommitSHA() string    // Current commit
    GetEventName() string    // Event type (push, pull_request, etc.)
    GetPlatform() Platform
}

// CommentManager handles VCS comment operations (PR/MR comments).
type CommentManager interface {
    PostOrUpdateComment(ctx context.Context, content string) error
    FindExistingComment(ctx context.Context, uuid string) (interface{}, error)
}

// CommitStatusWriter updates commit status checks (optional).
type CommitStatusWriter interface {
    SetCommitStatus(ctx context.Context, status CommitStatus) error
    IsCommitStatusSupported() bool
}

type CommitStatus struct {
    State       string // success, failure, pending
    Context     string // "atmos/deployment"
    Description string
    TargetURL   string
}

// ReleasePublisher creates VCS releases (optional).
type ReleasePublisher interface {
    CreateRelease(ctx context.Context, release Release) error
    IsReleaseSupported() bool
}

type Release struct {
    Tag         string
    Name        string
    Body        string
    Draft       bool
    Prerelease  bool
}
```

**VCS providers are for**:
- PR/MR comments (deployment status, SBOM links)
- Commit status checks (✓ Deployment successful)
- Creating releases (tagging release records)

### CI/CD Provider Interface

**Core interface definition** (`pkg/cicd/interface.go`):

```go
// Platform represents a CI/CD platform type.
type Platform string

const (
    PlatformGitHubActions   Platform = "github-actions"
    PlatformGitLabCI        Platform = "gitlab-ci"
    PlatformCircleCI        Platform = "circleci"
    PlatformJenkins         Platform = "jenkins"
    PlatformBuildkite       Platform = "buildkite"
    PlatformSpacelift       Platform = "spacelift"
    PlatformAzurePipelines  Platform = "azure-pipelines"
    PlatformUnknown         Platform = "unknown"
)

// Provider is the main CI/CD provider interface.
type Provider interface {
    // Core functionality
    DetectContext() (Context, error)
    CreateApprovalManager(ctx Context) (ApprovalManager, error)
    CreateMatrixStrategy() MatrixStrategy

    // Optional capabilities - return nil if not supported
    GetWorkflowDispatcher() WorkflowDispatcher
    GetJobSummaryWriter() JobSummaryWriter
    GetArtifactPublisher() ArtifactPublisher

    // Metadata
    GetPlatform() Platform
    IsAvailable() bool
}

// Context provides CI/CD-specific context information.
type Context interface {
    GetRunID() string        // Workflow/pipeline/build run ID
    GetJobID() string        // Current job/step ID
    GetBuildNumber() int     // Build number (Jenkins, CircleCI)
    GetWorkflowName() string // Workflow/pipeline name
    IsCI() bool              // Running in CI environment
    GetPlatform() Platform
    GetEnvironment() map[string]string // Platform-specific env vars
}

// ApprovalManager handles deployment approval workflows (Atmos Pro integration).
type ApprovalManager interface {
    // RequestApproval blocks until approved/rejected
    RequestApproval(ctx context.Context, req ApprovalRequest) (*ApprovalResponse, error)

    // IsApprovalSupported returns true if approval workflow available
    IsApprovalSupported() bool
}

// MatrixStrategy generates CI matrix configuration for parallel rollouts.
type MatrixStrategy interface {
    // GenerateMatrix creates matrix for all components
    GenerateMatrix(deployment, target string) (*Matrix, error)

    // GenerateMatrixForWaves creates wave-based matrix (respects DAG)
    GenerateMatrixForWaves(deployment, target string) (*WaveMatrix, error)
}

// WorkflowDispatcher triggers workflows programmatically (optional).
type WorkflowDispatcher interface {
    TriggerWorkflow(ctx context.Context, req WorkflowDispatchRequest) error
    IsWorkflowDispatchSupported() bool
}

// JobSummaryWriter writes CI job summary (optional).
type JobSummaryWriter interface {
    WriteJobSummary(content string) error
    IsJobSummarySupported() bool
}

// ArtifactPublisher publishes CI artifacts (optional).
type ArtifactPublisher interface {
    PublishArtifact(name string, paths []string) error
    IsArtifactSupported() bool
}
```

**CI/CD providers are for**:
- Matrix generation (parallel component deployment)
- Approval workflows (Atmos Pro integration)
- Job summaries (deployment reports in CI UI)
- Workflow dispatch (trigger deployments programmatically)
- Artifact publishing (SBOMs, test reports)

**Key Design Decisions**:

1. **Separate VCS and CI/CD**: Allows mixing (GitHub VCS + CircleCI, GitLab VCS + Jenkins, etc.)
2. **Optional Capabilities**: Return `nil` for unsupported features (graceful degradation)
3. **Atmos Pro Integration**: Approval manager integrates with Atmos Pro API
4. **DAG-Aware**: Matrix strategies understand component dependencies
5. **Auto-Detection**: Environment variable-based provider discovery

### Dual Provider Pattern

**GitHub (implements both)**:
```go
// pkg/vcs/github/provider.go
func init() {
    vcs.RegisterProvider(vcs.PlatformGitHub, NewGitHubVCSProvider)
}

// pkg/cicd/github/provider.go
func init() {
    cicd.RegisterProvider(cicd.PlatformGitHubActions, NewGitHubCICDProvider)
}
```

**CircleCI (implements only CI/CD)**:
```go
// pkg/cicd/circleci/provider.go
func init() {
    cicd.RegisterProvider(cicd.PlatformCircleCI, NewCircleCIProvider)
}
// No VCS provider - CircleCI doesn't manage repos
```

**Gitea (implements only VCS)**:
```go
// pkg/vcs/gitea/provider.go
func init() {
    vcs.RegisterProvider(vcs.PlatformGitea, NewGiteaProvider)
}
// No CI/CD provider (yet) - Gitea Actions is separate
```

**Usage in Atmos**:
```go
// Detect both providers independently
vcsProvider := vcs.DetectProvider()
cicdProvider := cicd.DetectProvider()

// Use VCS for PR comments
if vcsProvider != nil {
    commentMgr := vcsProvider.CreateCommentManager(ctx)
    commentMgr.PostOrUpdateComment(ctx, "Deployment started...")
}

// Use CI/CD for matrix generation
if cicdProvider != nil {
    matrixStrategy := cicdProvider.CreateMatrixStrategy()
    matrix, _ := matrixStrategy.GenerateMatrix("payment-service", "prod")
}
```

### GitHub Actions Provider

**Implementation** (`pkg/cicd/github/provider.go`):

```go
func init() {
    cicd.RegisterProvider(cicd.PlatformGitHubActions, NewGitHubActionsProvider)
}

type GitHubActionsProvider struct{}

func (p *GitHubActionsProvider) IsAvailable() bool {
    return os.Getenv("GITHUB_ACTIONS") == "true"
}

func (p *GitHubActionsProvider) DetectContext() (cicd.Context, error) {
    return &GitHubContext{
        Owner:     os.Getenv("GITHUB_REPOSITORY_OWNER"),
        Repo:      os.Getenv("GITHUB_REPOSITORY"),
        Branch:    os.Getenv("GITHUB_REF_NAME"),
        CommitSHA: os.Getenv("GITHUB_SHA"),
        RunID:     os.Getenv("GITHUB_RUN_ID"),
        Token:     os.Getenv("GITHUB_TOKEN"),
        EventName: os.Getenv("GITHUB_EVENT_NAME"),
        PRNumber:  parsePRNumber(os.Getenv("GITHUB_REF")),
    }, nil
}

func (p *GitHubActionsProvider) CreateApprovalManager(ctx cicd.Context) (cicd.ApprovalManager, error) {
    return NewGitHubApprovalManager(ctx), nil
}

func (p *GitHubActionsProvider) CreateMatrixStrategy() cicd.MatrixStrategy {
    return &GitHubMatrixStrategy{}
}

func (p *GitHubActionsProvider) GetJobSummaryWriter() cicd.JobSummaryWriter {
    if os.Getenv("GITHUB_STEP_SUMMARY") != "" {
        return &GitHubJobSummaryWriter{}
    }
    return nil // Not supported (e.g., running locally)
}
```

### Atmos Pro Approval Integration

**Approval manager** integrates deployment approvals with Atmos Pro:

```go
// pkg/cicd/github/approval.go
type GitHubApprovalManager struct {
    ctx    cicd.Context
    client *pro.Client
}

func (m *GitHubApprovalManager) RequestApproval(
    ctx context.Context,
    req cicd.ApprovalRequest,
) (*cicd.ApprovalResponse, error) {
    // Create Atmos Pro approval request
    approval, err := m.client.CreateDeploymentApproval(ctx, pro.ApprovalRequest{
        Deployment:     req.Deployment,
        Target:         req.Target,
        Release:        req.Release,
        Component:      req.Component, // Optional: component-level approval
        Repository:     fmt.Sprintf("%s/%s", m.ctx.GetOwner(), m.ctx.GetRepo()),
        RunID:          m.ctx.GetRunID(),
        CommitSHA:      m.ctx.GetCommitSHA(),
        RequiredTeams:  req.RequiredTeams,
        RequiredUsers:  req.RequiredUsers,
        TimeoutMinutes: req.TimeoutMinutes,
    })

    // Poll for approval (Atmos Pro sends webhook when approved)
    return m.pollForApproval(ctx, approval.ID, req.TimeoutMinutes)
}

func (m *GitHubApprovalManager) IsApprovalSupported() bool {
    return m.client.IsConfigured() && os.Getenv("ATMOS_PRO_TOKEN") != ""
}
```

### Matrix Generation

**GitHub matrix strategy** for parallel deployments:

```go
// pkg/cicd/github/matrix.go
type GitHubMatrixStrategy struct{}

func (s *GitHubMatrixStrategy) GenerateMatrix(
    deploymentName, target string,
) (*cicd.Matrix, error) {
    deploy, err := deployment.Load(deploymentName)
    if err != nil {
        return nil, err
    }

    matrix := &cicd.Matrix{Include: []cicd.MatrixEntry{}}

    for _, comp := range deploy.GetComponents(target) {
        matrix.Include = append(matrix.Include, cicd.MatrixEntry{
            Deployment: deploymentName,
            Target:     target,
            Component:  comp.Name,
        })
    }

    return matrix, nil
}

func (s *GitHubMatrixStrategy) GenerateMatrixForWaves(
    deploymentName, target string,
) (*cicd.WaveMatrix, error) {
    // Build DAG, topological sort, generate wave-based matrices
    // (See Concurrent Deployment Architecture section for DAG details)
}
```

### CLI Usage in CI/CD

**Deployment with approval**:

```bash
# GitHub Actions environment automatically detected
atmos deployment rollout payment-service --target prod --approve
# → Detects GitHub Actions provider
# → Creates Atmos Pro approval request
# → Prints approval URL
# → Blocks until approved/rejected
# → Continues rollout on approval
```

**Matrix generation**:

```bash
# Generate GitHub Actions matrix JSON
atmos deployment matrix payment-service --target prod --format json
# Output:
# {
#   "include": [
#     {"deployment": "payment-service", "target": "prod", "component": "vpc"},
#     {"deployment": "payment-service", "target": "prod", "component": "rds"},
#     {"deployment": "payment-service", "target": "prod", "component": "ecs"}
#   ]
# }

# Generate wave-based matrix (respects dependencies)
atmos deployment matrix payment-service --target prod --waves --format json
# Output:
# {
#   "waves": [
#     {"include": [{"deployment": "payment-service", "target": "prod", "component": "vpc", "wave": 1}]},
#     {"include": [{"deployment": "payment-service", "target": "prod", "component": "rds", "wave": 2}]},
#     {"include": [{"deployment": "payment-service", "target": "prod", "component": "ecs", "wave": 3}]}
#   ]
# }
```

### GitHub Actions Workflow Examples

**Simple deployment** (zero bash required):

```yaml
name: Deploy to Production

on:
  workflow_dispatch:
    inputs:
      deployment:
        required: true
      target:
        required: true

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write

    steps:
      - uses: actions/checkout@v4
      - uses: cloudposse/github-action-setup-atmos@v2

      - name: Deploy
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          ATMOS_PRO_TOKEN: ${{ secrets.ATMOS_PRO_TOKEN }}
        run: |
          atmos deployment rollout ${{ inputs.deployment }} \
            --target ${{ inputs.target }} \
            --approve \
            --auto-approve
```

**Matrix-based parallel deployment**:

```yaml
name: Parallel Deployment

jobs:
  matrix:
    runs-on: ubuntu-latest
    outputs:
      matrix: ${{ steps.gen.outputs.matrix }}
    steps:
      - uses: actions/checkout@v4
      - uses: cloudposse/github-action-setup-atmos@v2
      - id: gen
        run: |
          MATRIX=$(atmos deployment matrix payment-service --target prod --format json)
          echo "matrix=$MATRIX" >> $GITHUB_OUTPUT

  deploy:
    needs: matrix
    runs-on: ubuntu-latest
    strategy:
      matrix: ${{ fromJson(needs.matrix.outputs.matrix) }}
      max-parallel: 5

    steps:
      - uses: actions/checkout@v4
      - uses: cloudposse/github-action-setup-atmos@v2

      - name: Deploy Component
        run: |
          atmos deployment rollout ${{ matrix.deployment }} \
            --target ${{ matrix.target }} \
            --component ${{ matrix.component }}
```

**Component-specific workflow dispatch**:

```yaml
name: Deploy Single Component

on:
  workflow_dispatch:
    inputs:
      deployment:
        required: true
      target:
        required: true
      component:
        required: true

jobs:
  deploy-component:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: cloudposse/github-action-setup-atmos@v2

      - name: Deploy Component
        env:
          ATMOS_PRO_TOKEN: ${{ secrets.ATMOS_PRO_TOKEN }}
        run: |
          atmos deployment rollout ${{ inputs.deployment }} \
            --target ${{ inputs.target }} \
            --component ${{ inputs.component }} \
            --approve
```

### Provider Registry Pattern

```go
// pkg/cicd/factory.go
package cicd

type ProviderFactory func() Provider

var providers = map[Platform]ProviderFactory{}

// DetectProvider automatically detects CI/CD provider.
func DetectProvider() Provider {
    orderedPlatforms := []Platform{
        PlatformGitHubActions,
        PlatformGitLabCI,
        // Add others as implemented
    }

    for _, platform := range orderedPlatforms {
        if factory, ok := providers[platform]; ok {
            provider := factory()
            if provider.IsAvailable() {
                return provider
            }
        }
    }

    return nil // No CI/CD provider (running locally)
}

// RegisterProvider registers provider factory (called in init()).
func RegisterProvider(platform Platform, factory ProviderFactory) {
    providers[platform] = factory
}
```

### Benefits

1. **Zero Bash Required**: Native Atmos commands work directly in CI
2. **Dual Abstraction**: VCS and CI/CD separate, allows mixing providers
3. **Provider Agnostic**: Same commands on GitHub, GitLab, CircleCI, etc.
4. **Atmos Pro Integration**: Native approval workflows via CI/CD provider
5. **Matrix Support**: Auto-generate CI matrix configs with DAG awareness
6. **Job Summaries**: Rich deployment reports in CI UI
7. **PR/MR Comments**: Deployment status via VCS provider
8. **Commit Status**: CI checks integration via VCS provider
9. **Extensible**: Add new providers via interface implementation
10. **Graceful Degradation**: Unsupported features return `nil`, not errors
11. **Mix and Match**: GitHub VCS + CircleCI, GitLab VCS + Jenkins, etc.

### Future Provider Implementations

**VCS Providers**:
- **GitLab** (`pkg/vcs/gitlab`): MR comments, commit status, releases
- **Bitbucket** (`pkg/vcs/bitbucket`): PR comments, commit status
- **Azure DevOps** (`pkg/vcs/azuredevops`): PR comments, work item linking
- **Gitea** (`pkg/vcs/gitea`): PR comments, releases

**CI/CD Providers**:
- **GitLab CI** (`pkg/cicd/gitlab`): Approval via API, pipeline triggers, job artifacts
- **CircleCI** (`pkg/cicd/circleci`): Approval jobs, dynamic config, artifacts
- **Jenkins** (`pkg/cicd/jenkins`): Approval input steps, parameterized builds
- **Buildkite** (`pkg/cicd/buildkite`): Pipeline generation, annotations
- **Spacelift** (`pkg/cicd/spacelift`): Stack-based deployments, approval policies
- **Azure Pipelines** (`pkg/cicd/azurepipelines`): Approval gates, pipeline runs

## CLI Design

### Discovery & Management

```bash
# List all deployments
atmos deployments list
# Output:
# NAME              COMPONENTS    STACKS    TARGETS
# api               3             4         dev, staging, prod
# background-worker 2             3         dev, prod
# platform          15            8         dev, staging, prod

# Show deployment details
atmos deployments describe api
# Output: Full deployment configuration with resolved values

# Validate deployment configuration
atmos deployments validate api
atmos deployments validate --all

# Create deployment from existing stacks (migration helper)
atmos deployments migrate \
  --stack-pattern "platform/*" \
  --output deployments/platform.yaml

# Generate deployment from components
atmos deployments init api \
  --components terraform/ecs/taskdef-api,terraform/ecs/service-api \
  --output deployments/api.yaml
```

### Build Stage

**Purpose**: Compile source code into container images with reproducible digests.

```bash
# Build all nixpack components in deployment
atmos deployment build api --target dev

# Build specific component only
atmos deployment build api --target dev --component payment-api

# Build with Git reference for provenance
atmos deployment build api --target dev --ref $(git rev-parse HEAD)

# Build without publishing (local testing)
atmos deployment build api --target dev --no-publish

# Build with custom nixpacks options
atmos deployment build api --target dev \
  --pkgs ffmpeg,imagemagick \
  --install-cmd "npm ci" \
  --build-cmd "npm run build" \
  --start-cmd "npm start"

# Build with Dockerfile (escape hatch)
# If Dockerfile exists in component source, nixpacks uses it automatically
atmos deployment build api --target dev --dockerfile ./services/api/Dockerfile
```

### Test Stage

**Purpose**: Execute tests inside built containers, capture results.

```bash
# Run tests inside built container
atmos deployment test api --target dev

# Run with custom test command
atmos deployment test api --target dev --command "go test ./... -v"

# Run with JUnit output
atmos deployment test api --target dev --command "go test ./..." --report ./out/junit.xml

# Run with volume mounts
atmos deployment test api --target dev --command "pytest -v" --mount ./tests:/app/tests

# Run integration tests
atmos deployment test api --target dev --suite integration
```

### Release Stage

**Purpose**: Capture immutable release record with image digests, SBOM, provenance.

```bash
# Create release from latest build
atmos deployment release api --target dev

# Create release with annotations
atmos deployment release api --target dev \
  --annotate "description=Add user authentication" \
  --annotate "pr=#482" \
  --annotate "jira=PROJ-123"

# Create release with specific digest
atmos deployment release api --target dev --digest sha256:abc123...

# List releases for deployment
atmos deployment releases api
atmos deployment releases api --target dev
atmos deployment releases api --target dev --limit 10

# Show release details
atmos deployment releases api --id abc123

# Create OCI bundle artifact (self-contained deployment)
atmos deployment release api --target dev --bundle
# Creates OCI artifact with:
# - Image digest
# - Deployment configuration
# - Stack configurations
# - Component configurations
# - Release metadata
```

### Rollout Stage

**Purpose**: Update infrastructure components (Terraform/Helm/Lambda) to use release.

```bash
# Rollout latest release to target (default behavior)
atmos deployment rollout api --target dev

# Rollout specific release
atmos deployment rollout api --target dev --release abc123

# Auto-approve (for CI)
atmos deployment rollout api --target dev --auto-approve

# Parallel rollout with DAG-based concurrency
atmos deployment rollout api --target dev --parallelism 5
# Respects component dependencies (depends_on)
# Processes up to 5 components concurrently
# Components with no dependencies run first
# Blocks downstream components until dependencies complete

# Promote release to another target (no rebuild)
atmos deployment rollout api --target staging --release abc123
atmos deployment rollout api --target prod --release abc123

# Rollback to previous release
atmos deployment releases api --target prod --limit 5
atmos deployment rollout api --target prod --release xyz789

# Show deployment status for target
atmos deployment status api --target prod

# Detect drift (compare deployed vs expected)
atmos deployment drift api --target prod
```

### Complete Workflow Examples

**Development Workflow**:
```bash
# 1. Build locally
atmos deployment build api --target dev --no-publish

# 2. Test locally
atmos deployment test api --target dev

# 3. Build and publish
atmos deployment build api --target dev

# 4. Create release
atmos deployment release api --target dev --annotate "pr=#482"

# 5. Rollout to dev
atmos deployment rollout api --target dev --auto-approve
```

**CI/CD Workflow**:
```bash
# Pull request (build + test only)
atmos deployment build api --target dev
atmos deployment test api --target dev

# Merge to main (release + rollout dev)
atmos deployment build api --target dev --ref $GITHUB_SHA
atmos deployment test api --target dev
atmos deployment release api --target dev --annotate "sha=$GITHUB_SHA"
atmos deployment rollout api --target dev --auto-approve

# Manual promotion to staging
RELEASE_ID=$(atmos deployment releases api --target dev --limit 1 --format json | jq -r '.id')
atmos deployment rollout api --target staging --release $RELEASE_ID

# Manual promotion to prod
atmos deployment rollout api --target prod --release $RELEASE_ID
```

**Rollback Workflow**:
```bash
# 1. List recent releases
atmos deployment releases api --target prod --limit 10

# 2. Inspect previous release
atmos deployment releases api --id xyz789

# 3. Rollback to previous release
atmos deployment rollout api --target prod --release xyz789
```

## Implementation Plan

### Phase 1: Core Infrastructure (4-6 weeks)

**Week 1-2: Schema & Configuration**
- [ ] Define deployment YAML schema in `pkg/datafetcher/schema/deployment/`
- [ ] Add deployment discovery in `pkg/config/`
- [ ] Implement deployment validation
- [ ] Update `atmos.yaml` schema to reference deployments directory
- [ ] Vendor manifest schema support (inline, external, vendor.yaml fallback)

**Week 2-3: Performance Optimization & Vendor Cache**
- [ ] Modify stack processor to accept deployment context
- [ ] Filter stack loading based on `deployment.stacks`
- [ ] Implement content-addressable vendor cache (`.atmos/vendor-cache/`)
  - [ ] Cross-platform file linking: hard links (Unix/Mac) + copy fallback (Windows)
  - [ ] No symlinks (Windows admin privilege requirement)
  - [ ] Test on all platforms: Linux, macOS, Windows
- [ ] Cache index and lock file generation
- [ ] Benchmark performance improvements
- [ ] Add deployment-scoped component resolution

**Week 3-4: JIT Vendoring**
- [ ] Vendor configuration resolution (inline → manifest → vendor.yaml)
- [ ] Label-based component filtering for environments
- [ ] `atmos vendor pull --deployment` implementation
- [ ] `atmos vendor status --deployment` implementation
- [ ] `atmos vendor cache stats` implementation
- [ ] `atmos vendor clean` with GC support

**Week 4-5: CLI Commands - Discovery**
- [ ] `atmos deployments list` command
- [ ] `atmos deployments describe` command
- [ ] `atmos deployments validate` command
- [ ] `atmos deployments migrate` command (generate from stacks)

**Week 5-6: Testing & Documentation**
- [ ] Unit tests for deployment loading/validation
- [ ] Unit tests for vendor cache and JIT vendoring
- [ ] Integration tests with test fixtures
- [ ] Docusaurus documentation for deployment concept
- [ ] Vendor cache architecture documentation
- [ ] Migration guide from existing stacks

### Phase 2: Build & Release (4-6 weeks)

**Week 1-2: Nixpack Component Type**
- [ ] Integrate nixpacks (shell out or Go bindings if available)
- [ ] Implement nixpack component in `pkg/component/nixpack.go`
- [ ] Add Dockerfile escape hatch support
- [ ] SBOM generation integration
- [ ] Coordinate with in-flight nixpacks PR

**Week 2-3: Build Commands**
- [ ] `atmos deployment build` command implementation
- [ ] Image digest capture and storage
- [ ] Local build (no publish) support
- [ ] CI build with provenance

**Week 3-4: Release Commands**
- [ ] Release record schema and storage
- [ ] `atmos deployment release` command
- [ ] `atmos deployment releases` command (list/describe)
- [ ] Release promotion workflow

**Week 4-6: Testing & Documentation**
- [ ] Build/release integration tests
- [ ] Nixpacks compatibility testing (Go, Node.js, Python, Rust)
- [ ] Documentation with examples
- [ ] CI workflow examples

### Phase 3: Test & Rollout Stages (4-6 weeks)

**Week 1-2: Test Stage Implementation**
- [ ] Container test execution framework
- [ ] `atmos deployment test` command implementation
- [ ] Test report collection (JUnit, etc.)
- [ ] Volume mount support for test data

**Week 2-4: Rollout Stage Implementation**
- [ ] Label-based component binding
- [ ] Digest injection into Terraform/Helm/Lambda
- [ ] `atmos deployment rollout` command
- [ ] `atmos deployment status` command
- [ ] `atmos deployment drift` command
- [ ] Concurrent deployment architecture:
  - [ ] Workspace isolation (`pkg/deployment/workspace.go`)
  - [ ] Dependency DAG builder (reuse existing `depends_on`)
  - [ ] Topological sort for execution waves
  - [ ] Parallel executor with `--parallelism` flag
  - [ ] `golang.org/x/sync/errgroup` for bounded concurrency
  - [ ] Automatic workspace cleanup (defer)
  - [ ] `--keep-workspace` flag for debugging
- [ ] Test parallelism on all platforms (Linux, macOS, Windows)

**Week 4-6: Testing & Documentation**
- [ ] End-to-end deployment tests
- [ ] Rollback testing
- [ ] Complete workflow documentation
- [ ] Video tutorials

### Phase 4: Advanced Features (6-8 weeks)

**Week 1-2: OCI Bundle Artifacts**
- [ ] Bundle deployment context into OCI artifact
- [ ] Self-contained release artifacts
- [ ] Bundle extraction for rollback

**Week 2-3: Status & Drift**
- [ ] `atmos rollout status` command
- [ ] `atmos rollout drift` command
- [ ] Deployment state tracking

**Week 3-4: Multi-Component Coordination**
- [ ] Parallel component builds
- [ ] Dependency-aware rollouts
- [ ] Atomic multi-component updates

**Week 4-6: VCS and CI/CD Provider Abstraction**
- [ ] Create `pkg/vcs/interface.go` (matching `tools/gotcha/pkg/vcs` pattern)
  - [ ] `Provider`, `Context`, `CommentManager` interfaces
  - [ ] `CommitStatusWriter`, `ReleasePublisher` (optional)
  - [ ] Provider registry: `RegisterProvider()`, `DetectProvider()`
- [ ] Create `pkg/cicd/interface.go` (separate from VCS)
  - [ ] `Provider`, `Context`, `ApprovalManager`, `MatrixStrategy` interfaces
  - [ ] `WorkflowDispatcher`, `JobSummaryWriter`, `ArtifactPublisher` (optional)
  - [ ] Provider registry: `RegisterProvider()`, `DetectProvider()`
- [ ] Implement GitHub VCS provider (`pkg/vcs/github/`)
  - [ ] PR comment management
  - [ ] Commit status updates (optional)
  - [ ] Release publishing (optional)
- [ ] Implement GitHub Actions CI/CD provider (`pkg/cicd/github/`)
  - [ ] Provider registration and auto-detection via `GITHUB_ACTIONS` env
  - [ ] Context detection (run ID, job ID, workflow name)
  - [ ] Matrix strategy (DAG-aware component matrices)
  - [ ] Atmos Pro approval manager integration
  - [ ] Job summary writer (`$GITHUB_STEP_SUMMARY`)
- [ ] Add `atmos deployment matrix` command
  - [ ] `--format json` output for GitHub Actions matrix
  - [ ] `--waves` flag for DAG-based wave matrices
- [ ] Add `--approve` flag to `atmos deployment rollout`
  - [ ] Detects CI/CD provider automatically
  - [ ] Blocks until Atmos Pro approval received
  - [ ] Prints approval URL to logs
- [ ] Integration tests with mocked VCS/CI environments
- [ ] Test zero-bash workflows in actual GitHub Actions
- [ ] Document dual-provider pattern (VCS + CI/CD separate)

**Week 6-8: Polish & Documentation**
- [ ] Performance optimization
- [ ] Error message improvements
- [ ] Comprehensive documentation
- [ ] Blog post and changelog

## Success Metrics

### Performance
- Stack loading time: 10-20x improvement (10-20s → 1-2s)
- Memory usage: 10x reduction (500MB → 50MB)
- Build reproducibility: 100% (same input → same digest)

### Adoption
- 50% of teams using deployments within 6 months
- 80% of new projects start with deployments
- 10+ community-contributed deployment templates

### Quality
- 90%+ test coverage for deployment code
- Zero critical bugs in first 3 months
- <5min to create first deployment (via migrate command)

## Security & Compliance

### SBOM (Software Bill of Materials) Generation

Atmos deployments generate comprehensive SBOMs for both **container images** (nixpack builds) and **vendored infrastructure components** (Terraform modules, Helmfile charts).

#### **SBOM Formats Supported**

- **CycloneDX** (primary) - OWASP standard, JSON/XML
- **SPDX** (secondary) - Linux Foundation standard, JSON/YAML
- **Syft JSON** (optional) - Anchore format for tooling integration

#### **What Goes in the SBOM**

**1. Container Image SBOM** (from nixpack builds):
```json
{
  "bomFormat": "CycloneDX",
  "specVersion": "1.5",
  "serialNumber": "urn:uuid:...",
  "version": 1,
  "metadata": {
    "component": {
      "type": "container",
      "name": "payment-api",
      "version": "sha256:abc123...",
      "purl": "pkg:oci/payment-api@sha256:abc123..."
    },
    "tools": [
      {
        "vendor": "Atmos",
        "name": "atmos-nixpack-builder",
        "version": "1.0.0"
      }
    ]
  },
  "components": [
    {
      "type": "library",
      "name": "golang.org/x/crypto",
      "version": "v0.14.0",
      "purl": "pkg:golang/golang.org/x/crypto@v0.14.0",
      "hashes": [
        {
          "alg": "SHA-256",
          "content": "abc123..."
        }
      ],
      "licenses": [
        {
          "license": {
            "id": "BSD-3-Clause"
          }
        }
      ]
    },
    {
      "type": "library",
      "name": "github.com/gin-gonic/gin",
      "version": "v1.9.1",
      "purl": "pkg:golang/github.com/gin-gonic/gin@v1.9.1"
    },
    {
      "type": "operating-system",
      "name": "ubuntu",
      "version": "22.04",
      "purl": "pkg:deb/ubuntu/ubuntu@22.04"
    }
  ],
  "dependencies": [
    {
      "ref": "pkg:golang/golang.org/x/crypto@v0.14.0",
      "dependsOn": []
    }
  ]
}
```

**2. Vendored Component SBOM** (Terraform/Helmfile):

This example shows the complete dependency chain including:
- Vendored Terraform modules (from GitHub/registry)
- Terraform providers used by those modules
- Nested module dependencies
- Vendor source metadata (Git URL, commit SHA, cache path)

```json
{
  "bomFormat": "CycloneDX",
  "specVersion": "1.5",
  "metadata": {
    "component": {
      "type": "application",
      "name": "payment-service-prod-vendor",
      "version": "release-abc123"
    },
    "timestamp": "2025-01-15T10:30:00Z"
  },
  "components": [
    {
      "type": "library",
      "group": "terraform-modules",
      "name": "cloudposse/terraform-aws-components/ecs-service",
      "version": "1.8.2",
      "purl": "pkg:github/cloudposse/terraform-aws-components@1.8.2#modules/ecs-service",
      "externalReferences": [
        {
          "type": "vcs",
          "url": "https://github.com/cloudposse/terraform-aws-components/tree/1.8.2/modules/ecs-service"
        },
        {
          "type": "distribution",
          "url": "https://github.com/cloudposse/terraform-aws-components/archive/refs/tags/1.8.2.tar.gz"
        }
      ],
      "hashes": [
        {
          "alg": "SHA-256",
          "content": "mno345abc789def456..."
        }
      ],
      "licenses": [
        {
          "license": {
            "id": "Apache-2.0"
          }
        }
      ],
      "properties": [
        {
          "name": "atmos:deployment",
          "value": "payment-service"
        },
        {
          "name": "atmos:target",
          "value": "prod"
        },
        {
          "name": "atmos:component",
          "value": "ecs/payment-api"
        },
        {
          "name": "atmos:vendor_source",
          "value": "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        },
        {
          "name": "atmos:vendor_method",
          "value": "git"
        },
        {
          "name": "atmos:git_commit",
          "value": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0"
        },
        {
          "name": "atmos:cache_path",
          "value": ".atmos/vendor-cache/objects/sha256/mno345..."
        },
        {
          "name": "atmos:pulled_at",
          "value": "2025-01-15T09:00:00Z"
        }
      ]
    },
    {
      "type": "library",
      "group": "terraform-modules",
      "name": "cloudposse/terraform-aws-components/rds",
      "version": "1.3.5",
      "purl": "pkg:github/cloudposse/terraform-aws-components@1.3.5#modules/rds",
      "externalReferences": [
        {
          "type": "vcs",
          "url": "https://github.com/cloudposse/terraform-aws-components/tree/1.3.5/modules/rds"
        }
      ],
      "hashes": [
        {
          "alg": "SHA-256",
          "content": "jkl012uvw345xyz678..."
        }
      ],
      "licenses": [
        {
          "license": {
            "id": "Apache-2.0"
          }
        }
      ],
      "properties": [
        {
          "name": "atmos:deployment",
          "value": "payment-service"
        },
        {
          "name": "atmos:target",
          "value": "prod"
        },
        {
          "name": "atmos:component",
          "value": "rds/payment-db"
        },
        {
          "name": "atmos:vendor_source",
          "value": "github.com/cloudposse/terraform-aws-components//modules/rds"
        },
        {
          "name": "atmos:vendor_method",
          "value": "git"
        }
      ]
    },
    {
      "type": "library",
      "group": "terraform-providers",
      "name": "hashicorp/aws",
      "version": "5.31.0",
      "purl": "pkg:terraform/hashicorp/aws@5.31.0",
      "externalReferences": [
        {
          "type": "distribution",
          "url": "https://registry.terraform.io/providers/hashicorp/aws/5.31.0"
        }
      ],
      "hashes": [
        {
          "alg": "SHA-256",
          "content": "provider-sha256-abc123..."
        }
      ],
      "licenses": [
        {
          "license": {
            "id": "MPL-2.0"
          }
        }
      ],
      "properties": [
        {
          "name": "terraform:provider_type",
          "value": "official"
        },
        {
          "name": "terraform:registry_url",
          "value": "https://registry.terraform.io/providers/hashicorp/aws/5.31.0"
        }
      ]
    },
    {
      "type": "library",
      "group": "terraform-providers",
      "name": "hashicorp/random",
      "version": "3.6.0",
      "purl": "pkg:terraform/hashicorp/random@3.6.0",
      "licenses": [
        {
          "license": {
            "id": "MPL-2.0"
          }
        }
      ]
    },
    {
      "type": "library",
      "group": "terraform-modules",
      "name": "cloudposse/label/null",
      "version": "0.25.0",
      "purl": "pkg:terraform/cloudposse/label@0.25.0",
      "externalReferences": [
        {
          "type": "distribution",
          "url": "https://registry.terraform.io/modules/cloudposse/label/null/0.25.0"
        }
      ],
      "hashes": [
        {
          "alg": "SHA-256",
          "content": "nested-module-sha256..."
        }
      ],
      "licenses": [
        {
          "license": {
            "id": "Apache-2.0"
          }
        }
      ],
      "properties": [
        {
          "name": "atmos:nested_dependency",
          "value": "true"
        },
        {
          "name": "atmos:parent_module",
          "value": "cloudposse/terraform-aws-components/ecs-service"
        },
        {
          "name": "terraform:module_source",
          "value": "cloudposse/label/null"
        }
      ]
    },
    {
      "type": "library",
      "group": "terraform-modules",
      "name": "cloudposse/ecs-alb-service-task/aws",
      "version": "0.73.0",
      "purl": "pkg:terraform/cloudposse/ecs-alb-service-task@0.73.0",
      "properties": [
        {
          "name": "atmos:nested_dependency",
          "value": "true"
        },
        {
          "name": "atmos:parent_module",
          "value": "cloudposse/terraform-aws-components/ecs-service"
        }
      ]
    }
  ],
  "dependencies": [
    {
      "ref": "pkg:github/cloudposse/terraform-aws-components@1.8.2#modules/ecs-service",
      "dependsOn": [
        "pkg:terraform/hashicorp/aws@5.31.0",
        "pkg:terraform/hashicorp/random@3.6.0",
        "pkg:terraform/cloudposse/label@0.25.0",
        "pkg:terraform/cloudposse/ecs-alb-service-task@0.73.0"
      ]
    },
    {
      "ref": "pkg:github/cloudposse/terraform-aws-components@1.3.5#modules/rds",
      "dependsOn": [
        "pkg:terraform/hashicorp/aws@5.31.0",
        "pkg:terraform/cloudposse/label@0.25.0"
      ]
    },
    {
      "ref": "pkg:terraform/cloudposse/ecs-alb-service-task@0.73.0",
      "dependsOn": [
        "pkg:terraform/cloudposse/label@0.25.0"
      ]
    }
  ]
}
```

**3. Combined Deployment SBOM** (everything):
```json
{
  "bomFormat": "CycloneDX",
  "specVersion": "1.5",
  "metadata": {
    "component": {
      "type": "application",
      "name": "payment-service-deployment",
      "version": "release-abc123"
    }
  },
  "components": [
    {
      "type": "container",
      "name": "payment-api",
      "version": "sha256:abc123...",
      "components": []  // Nested: application dependencies
    },
    {
      "type": "library",
      "group": "terraform-modules",
      "name": "cloudposse/terraform-aws-components/ecs-service",
      "version": "1.8.2"
    },
    {
      "type": "application",
      "name": "atmos-cli",
      "version": "1.85.0",
      "purl": "pkg:golang/github.com/cloudposse/atmos@v1.85.0"
    }
  ]
}
```

#### **Component Registry Pattern for SBOM Generation**

**Architecture**: Each component type is responsible for generating its own SBOM. The component registry is extended with an SBOM generation interface.

**Component Interface Extension**:

```go
// pkg/component/interface.go
package component

import (
    cdx "github.com/CycloneDX/cyclonedx-go"
)

// Component interface (existing)
type Component interface {
    GetName() string
    GetType() string
    Validate() error
    Execute(ctx context.Context) error
}

// SBOMGenerator interface (new) - components opt-in by implementing this
type SBOMGenerator interface {
    // GenerateSBOM returns a CycloneDX BOM for this component
    GenerateSBOM(ctx context.Context) (*cdx.BOM, error)

    // SupportsSBOM returns true if this component can generate SBOMs
    SupportsSBOM() bool
}

// SBOM-aware component combines both interfaces
type SBOMAwareComponent interface {
    Component
    SBOMGenerator
}
```

**Component Implementation Examples**:

```go
// pkg/component/nixpack/sbom.go
package nixpack

import (
    "context"

    cdx "github.com/CycloneDX/cyclonedx-go"
    purl "github.com/package-url/packageurl-go"
)

// NixpackComponent implements SBOMGenerator
func (n *NixpackComponent) SupportsSBOM() bool {
    return true
}

func (n *NixpackComponent) GenerateSBOM(ctx context.Context) (*cdx.BOM, error) {
    bom := cdx.NewBOM()

    // Container image metadata
    bom.Metadata = &cdx.Metadata{
        Component: &cdx.Component{
            Type:    cdx.ComponentTypeContainer,
            Name:    n.imageName,
            Version: n.imageDigest,
            PackageURL: fmt.Sprintf("pkg:oci/%s@%s", n.imageName, n.imageDigest),
        },
        Tools: []cdx.Tool{
            {Vendor: "Atmos", Name: "atmos-nixpack", Version: schema.VERSION},
            {Vendor: "Nixpacks", Name: "nixpacks", Version: n.nixpacksVersion},
        },
    }

    components := []cdx.Component{}

    // Add Nix packages
    for _, pkg := range n.nixPackages {
        components = append(components, cdx.Component{
            Type:       cdx.ComponentTypeLibrary,
            Name:       pkg,
            PackageURL: purl.NewPackageURL("nix", "", pkg, "", nil, "").String(),
        })
    }

    // Add application dependencies (language-specific)
    // Delegate to language-specific scanners
    appDeps, err := n.scanApplicationDependencies(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to scan app dependencies: %w", err)
    }
    components = append(components, appDeps...)

    bom.Components = &components
    return bom, nil
}

// scanApplicationDependencies detects language and scans dependencies
func (n *NixpackComponent) scanApplicationDependencies(ctx context.Context) ([]cdx.Component, error) {
    // Detect language from nixpacks provider
    switch n.detectedProvider {
    case "go":
        return n.scanGoModules()
    case "node":
        return n.scanNpmPackages()
    case "python":
        return n.scanPipPackages()
    default:
        return []cdx.Component{}, nil
    }
}
```

```go
// pkg/component/terraform/sbom.go
package terraform

import (
    "context"

    cdx "github.com/CycloneDX/cyclonedx-go"
    "github.com/cloudposse/atmos/pkg/sbom"
)

// TerraformComponent implements SBOMGenerator
func (t *TerraformComponent) SupportsSBOM() bool {
    return true
}

func (t *TerraformComponent) GenerateSBOM(ctx context.Context) (*cdx.BOM, error) {
    // Parse Terraform configuration using terraform-config-inspect
    moduleInfo, err := sbom.ParseTerraformModule(t.componentPath)
    if err != nil {
        return nil, err
    }

    // Parse lock file for provider versions
    lockFile := filepath.Join(t.componentPath, ".terraform.lock.hcl")
    providerLocks, _ := sbom.ParseTerraformLockFile(lockFile)

    bom := cdx.NewBOM()
    bom.Metadata = &cdx.Metadata{
        Component: &cdx.Component{
            Type: cdx.ComponentTypeApplication,
            Name: t.componentName,
        },
    }

    components := []cdx.Component{}

    // Add providers from lock file
    for _, provider := range providerLocks {
        components = append(components, sbom.CreateProviderComponent(provider))
    }

    // Add module dependencies
    for _, modCall := range moduleInfo.ModuleCalls {
        if !sbom.IsLocalModule(modCall.Source) {
            components = append(components, sbom.CreateModuleComponent(modCall))
        }
    }

    bom.Components = &components
    return bom, nil
}
```

```go
// pkg/component/helmfile/sbom.go
package helmfile

// HelmfileComponent implements SBOMGenerator
func (h *HelmfileComponent) SupportsSBOM() bool {
    return true // Can scan Helm charts
}

func (h *HelmfileComponent) GenerateSBOM(ctx context.Context) (*cdx.BOM, error) {
    // Parse helmfile.yaml for chart dependencies
    // Extract chart versions, repositories
    // Return BOM with Helm charts as components
    // ...
}
```

**Component Registry Integration**:

```go
// pkg/component/registry.go
package component

import (
    "context"

    cdx "github.com/CycloneDX/cyclonedx-go"
)

type Registry struct {
    components map[string]Component
}

// GenerateDeploymentSBOM aggregates SBOMs from all components in deployment
func (r *Registry) GenerateDeploymentSBOM(
    ctx context.Context,
    deployment string,
    target string,
) (*cdx.BOM, error) {
    bom := cdx.NewBOM()
    bom.Metadata = &cdx.Metadata{
        Component: &cdx.Component{
            Type:    cdx.ComponentTypeApplication,
            Name:    fmt.Sprintf("%s-%s", deployment, target),
        },
    }

    allComponents := []cdx.Component{}

    // Iterate through all components in deployment
    for name, comp := range r.components {
        // Check if component supports SBOM generation
        if sbomGen, ok := comp.(SBOMGenerator); ok && sbomGen.SupportsSBOM() {
            componentBOM, err := sbomGen.GenerateSBOM(ctx)
            if err != nil {
                // Log warning but continue with other components
                log.Warn("Failed to generate SBOM for component", "component", name, "error", err)
                continue
            }

            // Merge component BOM into deployment BOM
            if componentBOM.Components != nil {
                allComponents = append(allComponents, *componentBOM.Components...)
            }
        }
    }

    bom.Components = &allComponents
    return bom, nil
}
```

**Go Libraries** (shared across component types):

1. **github.com/CycloneDX/cyclonedx-go** (Primary)
   - Official CycloneDX Go library
   - Supports CycloneDX 1.5 spec
   - JSON/XML encoding
   - License: Apache 2.0

2. **github.com/spdx/tools-golang** (SPDX support)
   - Official SPDX Go tools
   - SPDX 2.3 spec
   - JSON/YAML/RDF support
   - License: Apache 2.0

3. **github.com/anchore/syft** (Container/dependency scanning)
   - Can call syft as library or CLI
   - Excellent container scanning
   - Supports multiple output formats
   - License: Apache 2.0

4. **github.com/package-url/packageurl-go** (PURL generation)
   - Generate Package URLs
   - Required for CycloneDX/SPDX
   - License: MIT

#### **Extracting Terraform Module and Provider Information from HCL**

To build comprehensive SBOMs for Terraform infrastructure, Atmos must programmatically extract module sources, versions, and provider requirements from HCL files. HashiCorp provides official Go libraries for this purpose.

**Recommended Libraries:**

1. **`github.com/hashicorp/terraform-config-inspect`** - High-level Terraform module inspection
   - Parses module calls and their sources
   - Extracts provider requirements
   - Handles Terraform Registry references
   - Works without Terraform CLI installation
   - License: MPL-2.0

2. **`github.com/hashicorp/hcl/v2`** - Low-level HCL parsing (if needed)
   - For custom parsing needs beyond module inspection
   - Direct AST access
   - License: MPL-2.0

3. **`.terraform.lock.hcl` parsing** - Extract provider checksums and exact versions
   - Use standard HCL parser for lock file
   - Contains authoritative provider versions and checksums

**Implementation Strategy:**

```go
// pkg/sbom/terraform_parser.go
package sbom

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/hashicorp/hcl/v2"
    "github.com/hashicorp/hcl/v2/hclparse"
    "github.com/hashicorp/terraform-config-inspect/tfconfig"
)

// TerraformModuleInfo contains parsed module metadata.
type TerraformModuleInfo struct {
    Path              string
    ModuleCalls       []ModuleCall
    RequiredProviders []ProviderRequirement
    NestedModules     []TerraformModuleInfo
}

// ModuleCall represents a module {} block.
type ModuleCall struct {
    Name    string
    Source  string
    Version string // from version constraint
}

// ProviderRequirement from required_providers block.
type ProviderRequirement struct {
    Name      string
    Source    string // e.g., "hashicorp/aws"
    Version   string // constraint like ">= 5.0"
}

// ParseTerraformModule uses terraform-config-inspect to extract module and provider info.
func ParseTerraformModule(modulePath string) (*TerraformModuleInfo, error) {
    // Load module configuration
    module, diags := tfconfig.LoadModule(modulePath)
    if diags.HasErrors() {
        return nil, fmt.Errorf("failed to load module: %w", diags.Err())
    }

    info := &TerraformModuleInfo{
        Path:              modulePath,
        ModuleCalls:       []ModuleCall{},
        RequiredProviders: []ProviderRequirement{},
        NestedModules:     []TerraformModuleInfo{},
    }

    // Extract module calls
    for name, modCall := range module.ModuleCalls {
        call := ModuleCall{
            Name:    name,
            Source:  modCall.Source,
            Version: modCall.Version, // May be empty if no version constraint
        }
        info.ModuleCalls = append(info.ModuleCalls, call)

        // Recursively parse nested modules (if local)
        if isLocalModule(modCall.Source) {
            nestedPath := filepath.Join(modulePath, modCall.Source)
            if nested, err := ParseTerraformModule(nestedPath); err == nil {
                info.NestedModules = append(info.NestedModules, *nested)
            }
        }
    }

    // Extract required providers
    for name, provider := range module.RequiredProviders {
        req := ProviderRequirement{
            Name:    name,
            Source:  provider.Source,
            Version: joinVersionConstraints(provider.VersionConstraints),
        }
        info.RequiredProviders = append(info.RequiredProviders, req)
    }

    return info, nil
}

// ParseTerraformLockFile extracts exact provider versions and checksums.
func ParseTerraformLockFile(lockFilePath string) ([]ProviderLockEntry, error) {
    parser := hclparse.NewParser()
    f, diags := parser.ParseHCLFile(lockFilePath)
    if diags.HasErrors() {
        return nil, fmt.Errorf("failed to parse lock file: %w", diags.Errs())
    }

    var providers []ProviderLockEntry

    // Lock file structure:
    // provider "registry.terraform.io/hashicorp/aws" {
    //   version     = "5.31.0"
    //   constraints = ">= 5.0"
    //   hashes = [
    //     "h1:abc123...",
    //     "zh:def456...",
    //   ]
    // }

    content, _, diags := f.Body.PartialContent(&hcl.BodySchema{
        Blocks: []hcl.BlockHeaderSchema{
            {Type: "provider", LabelNames: []string{"source"}},
        },
    })

    if diags.HasErrors() {
        return nil, fmt.Errorf("failed to read lock file content: %w", diags.Errs())
    }

    for _, block := range content.Blocks {
        provider := ProviderLockEntry{
            Source: block.Labels[0],
        }

        attrs, diags := block.Body.JustAttributes()
        if diags.HasErrors() {
            continue
        }

        if version, exists := attrs["version"]; exists {
            val, _ := version.Expr.Value(nil)
            provider.Version = val.AsString()
        }

        if constraints, exists := attrs["constraints"]; exists {
            val, _ := constraints.Expr.Value(nil)
            provider.Constraints = val.AsString()
        }

        if hashes, exists := attrs["hashes"]; exists {
            val, _ := hashes.Expr.Value(nil)
            if val.Type().IsListType() {
                for _, hash := range val.AsValueSlice() {
                    provider.Hashes = append(provider.Hashes, hash.AsString())
                }
            }
        }

        providers = append(providers, provider)
    }

    return providers, nil
}

// ProviderLockEntry from .terraform.lock.hcl
type ProviderLockEntry struct {
    Source      string   // "registry.terraform.io/hashicorp/aws"
    Version     string   // "5.31.0"
    Constraints string   // ">= 5.0"
    Hashes      []string // ["h1:...", "zh:..."]
}

// Helper: check if module source is local path
func isLocalModule(source string) bool {
    return filepath.IsAbs(source) ||
           source == "." ||
           source[:2] == "./" ||
           source[:3] == "../"
}

// Helper: join version constraints
func joinVersionConstraints(constraints []string) string {
    if len(constraints) == 0 {
        return ""
    }
    return constraints[0] // Simplified; could join multiple constraints
}

// ParseVendoredComponent combines module info + lock file for complete SBOM.
func ParseVendoredComponent(componentPath string) (*VendoredComponentSBOM, error) {
    // 1. Parse module configuration
    moduleInfo, err := ParseTerraformModule(componentPath)
    if err != nil {
        return nil, fmt.Errorf("failed to parse module: %w", err)
    }

    // 2. Parse lock file (if exists)
    lockFilePath := filepath.Join(componentPath, ".terraform.lock.hcl")
    var providerLocks []ProviderLockEntry
    if _, err := os.Stat(lockFilePath); err == nil {
        providerLocks, err = ParseTerraformLockFile(lockFilePath)
        if err != nil {
            // Lock file is optional; log warning but continue
            fmt.Fprintf(os.Stderr, "Warning: failed to parse lock file: %v\n", err)
        }
    }

    // 3. Combine into SBOM data structure
    sbom := &VendoredComponentSBOM{
        ModuleInfo:    moduleInfo,
        ProviderLocks: providerLocks,
    }

    return sbom, nil
}

// VendoredComponentSBOM combines module and provider data for SBOM generation.
type VendoredComponentSBOM struct {
    ModuleInfo    *TerraformModuleInfo
    ProviderLocks []ProviderLockEntry
}

// Example usage in vendor SBOM generation:
func (g *Generator) GenerateVendorSBOMFromParsedData(
    deployment string,
    target string,
    vendoredComponents map[string]*VendoredComponentSBOM,
) (*cdx.BOM, error) {
    bom := cdx.NewBOM()

    components := []cdx.Component{}

    for componentName, sbomData := range vendoredComponents {
        // Add main module component
        moduleComponent := cdx.Component{
            Type:  cdx.ComponentTypeLibrary,
            Group: "terraform-modules",
            Name:  componentName,
        }
        components = append(components, moduleComponent)

        // Add providers from lock file (authoritative versions)
        for _, providerLock := range sbomData.ProviderLocks {
            providerComponent := g.createProviderComponentFromLock(providerLock)
            components = append(components, providerComponent)
        }

        // Add nested module dependencies
        for _, nestedModule := range flattenNestedModules(sbomData.ModuleInfo) {
            for _, modCall := range nestedModule.ModuleCalls {
                if !isLocalModule(modCall.Source) {
                    // External module dependency
                    depComponent := g.createModuleDependencyComponent(modCall)
                    components = append(components, depComponent)
                }
            }
        }
    }

    bom.Components = &components
    return bom, nil
}

func (g *Generator) createProviderComponentFromLock(lock ProviderLockEntry) cdx.Component {
    // Parse source to extract namespace/name
    // "registry.terraform.io/hashicorp/aws" → namespace=hashicorp, name=aws
    parts := parseProviderSource(lock.Source)

    return cdx.Component{
        Type:    cdx.ComponentTypeLibrary,
        Group:   "terraform-providers",
        Name:    fmt.Sprintf("%s/%s", parts.Namespace, parts.Name),
        Version: lock.Version,
        PackageURL: fmt.Sprintf("pkg:terraform/%s/%s@%s",
            parts.Namespace,
            parts.Name,
            lock.Version,
        ),
        Hashes: convertHashesToCDX(lock.Hashes),
        ExternalReferences: []cdx.ExternalReference{
            {
                Type: cdx.ERTypeDistribution,
                URL: fmt.Sprintf("https://registry.terraform.io/providers/%s/%s/%s",
                    parts.Namespace, parts.Name, lock.Version),
            },
        },
        Properties: []cdx.Property{
            {Name: "terraform:constraints", Value: lock.Constraints},
            {Name: "terraform:lock_hashes", Value: joinStrings(lock.Hashes, ",")},
        },
    }
}

func (g *Generator) createModuleDependencyComponent(modCall ModuleCall) cdx.Component {
    return cdx.Component{
        Type:    cdx.ComponentTypeLibrary,
        Group:   "terraform-modules",
        Name:    modCall.Source,
        Version: modCall.Version,
        PackageURL: generateModulePURL(modCall.Source, modCall.Version),
        Properties: []cdx.Property{
            {Name: "terraform:module_name", Value: modCall.Name},
            {Name: "terraform:version_constraint", Value: modCall.Version},
        },
    }
}

// Helper: flatten nested modules for dependency tracking
func flattenNestedModules(module *TerraformModuleInfo) []*TerraformModuleInfo {
    result := []*TerraformModuleInfo{module}
    for _, nested := range module.NestedModules {
        result = append(result, flattenNestedModules(&nested)...)
    }
    return result
}

type ProviderSourceParts struct {
    Registry  string // "registry.terraform.io"
    Namespace string // "hashicorp"
    Name      string // "aws"
}

func parseProviderSource(source string) ProviderSourceParts {
    // Parse "registry.terraform.io/hashicorp/aws" → {hashicorp, aws}
    // Simplified implementation
    parts := strings.Split(source, "/")
    if len(parts) == 3 {
        return ProviderSourceParts{
            Registry:  parts[0],
            Namespace: parts[1],
            Name:      parts[2],
        }
    }
    return ProviderSourceParts{}
}

func convertHashesToCDX(hashes []string) []cdx.Hash {
    var result []cdx.Hash
    for _, h := range hashes {
        // Terraform lock hashes are in format "h1:base64..." or "zh:base64..."
        // Convert to CycloneDX format
        if strings.HasPrefix(h, "h1:") {
            result = append(result, cdx.Hash{
                Algorithm: cdx.HashAlgoSHA256,
                Value:     h[3:], // Strip "h1:" prefix
            })
        }
        // zh: hashes are legacy, can include if needed
    }
    return result
}

func joinStrings(items []string, sep string) string {
    return strings.Join(items, sep)
}
```

**Complete Workflow: From Vendored Component to SBOM**

```go
// pkg/vendor/sbom_integration.go
package vendor

import (
    "github.com/cloudposse/atmos/pkg/sbom"
)

// GenerateSBOMForDeployment creates SBOM for all vendored components in a deployment.
func GenerateSBOMForDeployment(deployment string, target string) error {
    // 1. Get vendored components for this deployment/target
    vendorCache := ".atmos/vendor-cache/deployments/" + deployment + "/" + target

    components, err := discoverVendoredComponents(vendorCache)
    if err != nil {
        return err
    }

    // 2. Parse each component's Terraform configuration
    sbomData := make(map[string]*sbom.VendoredComponentSBOM)
    for name, path := range components {
        parsed, err := sbom.ParseVendoredComponent(path)
        if err != nil {
            return fmt.Errorf("failed to parse component %s: %w", name, err)
        }
        sbomData[name] = parsed
    }

    // 3. Generate CycloneDX SBOM
    generator := &sbom.Generator{Format: "cyclonedx"}
    bom, err := generator.GenerateVendorSBOMFromParsedData(deployment, target, sbomData)
    if err != nil {
        return err
    }

    // 4. Write SBOM to file
    sbomPath := fmt.Sprintf(".atmos/sboms/%s-%s-vendor.cdx.json", deployment, target)
    return writeBOMToFile(bom, sbomPath)
}
```

**Key Benefits of This Approach:**

1. **No Terraform CLI Required**: Uses Go libraries that parse HCL directly
2. **Comprehensive Dependency Graph**: Tracks modules, providers, and nested dependencies
3. **Authoritative Version Info**: Lock file provides exact provider versions and checksums
4. **Multi-Source Support**: Handles GitHub, GitLab, Terraform Registry, local modules
5. **SBOM Standard Compliance**: Generates CycloneDX/SPDX with proper PURLs and metadata

**Integration Approach:**

```go
// pkg/sbom/generator.go
package sbom

import (
    cdx "github.com/CycloneDX/cyclonedx-go"
    purl "github.com/package-url/packageurl-go"
    "github.com/cloudposse/atmos/pkg/schema"
)

type Generator struct {
    format string // cyclonedx, spdx, syft
}

// Generate SBOM for nixpack build
func (g *Generator) GenerateNixpackSBOM(
    componentName string,
    imageDigest string,
    nixPackages []string,
    appDependencies []Dependency,
) (*cdx.BOM, error) {
    bom := cdx.NewBOM()

    // Add nixpack build metadata
    bom.Metadata = &cdx.Metadata{
        Component: &cdx.Component{
            Type:    cdx.ComponentTypeContainer,
            Name:    componentName,
            Version: imageDigest,
        },
        Tools: []cdx.Tool{
            {
                Vendor:  "Atmos",
                Name:    "atmos-build",
                Version: schema.VERSION,
            },
            {
                Vendor:  "Nixpacks",
                Name:    "nixpacks",
                Version: getNixpacksVersion(),
            },
        },
    }

    components := []cdx.Component{}

    // Add Nix packages
    for _, pkg := range nixPackages {
        components = append(components, cdx.Component{
            Type:    cdx.ComponentTypeLibrary,
            Name:    pkg,
            PackageURL: purl.NewPackageURL("nix", "", pkg, "", nil, "").String(),
        })
    }

    // Add application dependencies (Go modules, npm packages, etc.)
    for _, dep := range appDependencies {
        components = append(components, cdx.Component{
            Type:       cdx.ComponentTypeLibrary,
            Name:       dep.Name,
            Version:    dep.Version,
            PackageURL: dep.PURL,
            Licenses:   parseLicenses(dep.License),
        })
    }

    bom.Components = &components
    return bom, nil
}

// Generate SBOM for vendored components
func (g *Generator) GenerateVendorSBOM(
    deployment string,
    target string,
    vendoredComponents map[string]VendoredComponent,
) (*cdx.BOM, error) {
    bom := cdx.NewBOM()

    bom.Metadata = &cdx.Metadata{
        Component: &cdx.Component{
            Type:    cdx.ComponentTypeApplication,
            Name:    fmt.Sprintf("%s-%s-vendor", deployment, target),
        },
    }

    components := []cdx.Component{}

    for name, comp := range vendoredComponents {
        // Generate component for the vendored module
        moduleComponent := g.createModuleComponent(name, comp, deployment, target)
        components = append(components, moduleComponent)

        // Also track Terraform providers used by this module
        if comp.TerraformProviders != nil {
            for _, provider := range comp.TerraformProviders {
                providerComponent := g.createProviderComponent(provider)
                components = append(components, providerComponent)
            }
        }

        // Track nested module dependencies
        if comp.ModuleDependencies != nil {
            for _, dep := range comp.ModuleDependencies {
                depComponent := g.createNestedModuleComponent(dep)
                components = append(components, depComponent)
            }
        }
    }

    bom.Components = &components
    return bom, nil
}

func (g *Generator) createModuleComponent(
    name string,
    comp VendoredComponent,
    deployment string,
    target string,
) cdx.Component {
    purl := generateVendorPURL(comp)

    component := cdx.Component{
        Type:    cdx.ComponentTypeLibrary,
        Group:   "terraform-modules",
        Name:    comp.Source,
        Version: comp.Version,
        PackageURL: purl,
        Hashes: []cdx.Hash{
            {
                Algorithm: cdx.HashAlgoSHA256,
                Value:     comp.Digest,
            },
        },
        ExternalReferences: []cdx.ExternalReference{
            {
                Type: cdx.ERTypeVCS,
                URL:  comp.SourceURL,
            },
        },
        Properties: []cdx.Property{
            {Name: "atmos:deployment", Value: deployment},
            {Name: "atmos:target", Value: target},
            {Name: "atmos:component", Value: name},
            {Name: "atmos:vendor_source", Value: comp.Source},
            {Name: "atmos:vendor_method", Value: comp.VendorMethod}, // git, http, local
            {Name: "atmos:cache_path", Value: comp.CachePath},
        },
    }

    // Add license if detected from module metadata
    if comp.License != "" {
        component.Licenses = &cdx.Licenses{
            {License: &cdx.License{ID: comp.License}},
        }
    }

    return component
}

func (g *Generator) createProviderComponent(provider TerraformProvider) cdx.Component {
    return cdx.Component{
        Type:    cdx.ComponentTypeLibrary,
        Group:   "terraform-providers",
        Name:    provider.Name,
        Version: provider.Version,
        PackageURL: fmt.Sprintf("pkg:terraform/%s/%s@%s",
            provider.Namespace,
            provider.Name,
            provider.Version,
        ),
        Hashes: []cdx.Hash{
            {
                Algorithm: cdx.HashAlgoSHA256,
                Value:     provider.SHA256Sum,
            },
        },
        Licenses: &cdx.Licenses{
            {License: &cdx.License{ID: provider.License}},
        },
        ExternalReferences: []cdx.ExternalReference{
            {
                Type: cdx.ERTypeDistribution,
                URL:  fmt.Sprintf("https://registry.terraform.io/providers/%s/%s/%s",
                    provider.Namespace, provider.Name, provider.Version),
            },
        },
    }
}

func (g *Generator) createNestedModuleComponent(dep ModuleDependency) cdx.Component {
    return cdx.Component{
        Type:    cdx.ComponentTypeLibrary,
        Group:   "terraform-modules",
        Name:    dep.Source,
        Version: dep.Version,
        PackageURL: generateModulePURL(dep.Source, dep.Version),
        Properties: []cdx.Property{
            {Name: "atmos:nested_dependency", Value: "true"},
            {Name: "atmos:parent_module", Value: dep.ParentModule},
        },
    }
}

func generateVendorPURL(comp VendoredComponent) string {
    // For GitHub sources
    if strings.Contains(comp.Source, "github.com") {
        parts := parseGitHubURL(comp.Source)
        return purl.NewPackageURL(
            "github",
            parts.Owner,
            parts.Repo,
            comp.Version,
            nil,
            parts.Subpath,
        ).String()
        // Result: pkg:github/cloudposse/terraform-aws-components@1.8.2#modules/ecs-service
    }

    // For Terraform registry
    if strings.Contains(comp.Source, "registry.terraform.io") {
        parts := parseTerraformRegistry(comp.Source)
        return purl.NewPackageURL(
            "terraform",
            parts.Namespace,
            parts.Name,
            comp.Version,
            nil,
            "",
        ).String()
        // Result: pkg:terraform/cloudposse/ecs-service@1.8.2
    }

    // For GitLab sources
    if strings.Contains(comp.Source, "gitlab.com") {
        parts := parseGitLabURL(comp.Source)
        return purl.NewPackageURL(
            "gitlab",
            parts.Namespace,
            parts.Project,
            comp.Version,
            nil,
            parts.Subpath,
        ).String()
    }

    // For Bitbucket sources
    if strings.Contains(comp.Source, "bitbucket.org") {
        parts := parseBitbucketURL(comp.Source)
        return purl.NewPackageURL(
            "bitbucket",
            parts.Workspace,
            parts.Repo,
            comp.Version,
            nil,
            parts.Subpath,
        ).String()
    }

    // For generic Git sources
    if strings.HasPrefix(comp.Source, "git::") {
        return fmt.Sprintf("pkg:generic/%s@%s", sanitizeName(comp.Source), comp.Version)
    }

    return ""
}
```

#### **CLI Integration**

**Component-Level SBOM Generation** (automatic during build/release):

```bash
# Generate SBOM during build (nixpack component auto-generates SBOM)
atmos deployment build payment-api --target prod --sbom
# Creates:
# - .atmos/sboms/nixpack-payment-api-prod-sha256:abc123.cdx.json
# Nixpack component generates SBOM with container + app dependencies

# Generate SBOM for specific component
atmos describe component ecs/payment-api -s prod --sbom
# Creates:
# - .atmos/sboms/terraform-ecs-payment-api-prod.cdx.json
# Terraform component generates SBOM with modules + providers

# Generate combined deployment SBOM (aggregates all components)
atmos deployment release payment-service --target prod --sbom
# Component registry calls GenerateSBOM() on each component
# Creates:
# - releases/payment-service/prod/release-abc123-sbom.cdx.json
# Includes: nixpack SBOMs + terraform SBOMs + helmfile SBOMs

# Export SBOM in different format
atmos deployment sbom export payment-service --release abc123 --format spdx --output /tmp/sbom.spdx.json

# Validate SBOM
atmos deployment sbom validate payment-service --release abc123

# Scan SBOM for vulnerabilities (integration with Grype/Trivy)
atmos deployment sbom scan payment-service --release abc123 --scanner grype
```

**Component Registry Workflow**:

1. **Build Phase**: Nixpack component generates container SBOM
2. **Release Phase**: Component registry aggregates SBOMs from all components
3. **Rollout Phase**: Terraform components generate infrastructure SBOMs on-demand

Each component type decides:
- What to include in its SBOM
- How to scan dependencies
- Whether SBOM generation is supported (`SupportsSBOM()` returns false = skip)

#### **SBOM Storage in Releases**

```yaml
# releases/payment-service/prod/release-abc123.yaml
release:
  id: "abc123"
  deployment: payment-service
  target: prod

  artifacts:
    payment-api:
      type: nixpack
      digest: "sha256:1234567890abcdef..."
      sbom:
        - format: "cyclonedx-json"
          digest: "sha256:sbom123..."
          path: ".atmos/sboms/payment-api-prod-sha256:1234567890abcdef.cdx.json"
          url: "oci://registry/payment-api@sha256:sbom123..."
        - format: "spdx-json"
          digest: "sha256:sbom456..."
          path: ".atmos/sboms/payment-api-prod-sha256:1234567890abcdef.spdx.json"

  vendor_sbom:
    format: "cyclonedx-json"
    digest: "sha256:vendor789..."
    path: ".atmos/sboms/payment-service-prod-vendor.cdx.json"
    components: 15

  combined_sbom:
    format: "cyclonedx-json"
    digest: "sha256:combined012..."
    path: "releases/payment-service/prod/release-abc123-sbom.cdx.json"
    total_components: 287
```

#### **Vulnerability Scanning Integration**

```go
// Integration with Grype/Trivy
func ScanSBOM(sbomPath string, scanner string) (*ScanResult, error) {
    switch scanner {
    case "grype":
        return scanWithGrype(sbomPath)
    case "trivy":
        return scanWithTrivy(sbomPath)
    default:
        return nil, fmt.Errorf("unknown scanner: %s", scanner)
    }
}

func scanWithGrype(sbomPath string) (*ScanResult, error) {
    cmd := exec.Command("grype", "sbom:"+sbomPath, "-o", "json")
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }

    var result ScanResult
    json.Unmarshal(output, &result)
    return &result, nil
}
```

#### **SBOM Policy Enforcement**

```yaml
# atmos.yaml
settings:
  sbom:
    enabled: true
    formats: ["cyclonedx-json", "spdx-json"]

    # Require SBOM for production releases
    require_for_targets: ["prod", "staging"]

    # Vulnerability scanning
    scan:
      enabled: true
      scanner: "grype"  # or "trivy"
      fail_on_severity: "high"  # critical, high, medium, low

    # License compliance
    licenses:
      allowed:
        - "Apache-2.0"
        - "MIT"
        - "BSD-3-Clause"
        - "MPL-2.0"
      denied:
        - "AGPL-3.0"
        - "GPL-3.0"
```

#### **Implementation Tasks**

Add to Phase 2 implementation plan:

```markdown
**Week 3-4: SBOM Generation (Component Registry Pattern)**
- [ ] Extend component interface with `SBOMGenerator` interface
  - [ ] `GenerateSBOM(ctx) (*cdx.BOM, error)` method
  - [ ] `SupportsSBOM() bool` method
- [ ] Integrate CycloneDX Go library (shared across components)
- [ ] Implement SBOM generation per component type:
  - [ ] Nixpack component: container image + app dependencies
  - [ ] Terraform component: modules + providers (using terraform-config-inspect)
  - [ ] Helmfile component: Helm charts + dependencies
- [ ] Component registry aggregation method: `GenerateDeploymentSBOM()`
- [ ] SBOM storage in release records
- [ ] CLI commands: `atmos sbom export/validate/scan`
- [ ] Integration with Grype/Trivy for vulnerability scanning
- [ ] License compliance checking
```

### Provenance Tracking
- Capture nixpacks version, detected provider, Nix packages
- Link releases to Git commits, PRs, CI runs
- Immutable audit trail

### Image Signing
- Optional integration with cosign/sigstore
- Verify signed images before rollout
- Policy enforcement via OPA

### Access Control
- Release creation requires appropriate IAM permissions
- Rollout requires infrastructure write permissions
- Audit log for all deployment operations

## Migration Strategy

### Existing Atmos Users

**Step 1: Identify Top-Level Stacks**
```bash
# List all stacks that don't have parent stacks
atmos list stacks --filter "import==null"
```

**Step 2: Generate Deployment**
```bash
# Create deployment from stack
atmos deployments migrate \
  --stack platform/dev/us-east-1 \
  --output deployments/platform-dev.yaml
```

**Step 3: Review & Adjust**
- Review generated deployment configuration
- Add nixpack components if applicable (for container workloads)
- Define targets (dev/staging/prod)
- Add labels for component binding

**Step 4: Validate**
```bash
atmos deployments validate platform-dev
```

**Step 5: Gradual Rollout**
- Use new deployment commands alongside existing stack commands
- No breaking changes to existing workflows
- Opt-in adoption

### New Atmos Users
- Start with deployment-first approach
- Use `atmos deployments init` to scaffold
- Follow deployment patterns from documentation

## Open Questions

1. **Nixpacks Integration**: How should in-flight nixpacks PR integrate with deployment components?
   - Option A: Separate component type (`nixpack`)
   - Option B: Unified component type (`build`) with driver selection (nixpacks, cnb, dockerfile)
   - **Decision**: Option A - use `nixpack` component type for now, unified `build` type in future release

2. **Vendor Manifest Format**: Should we extend existing vendor.yaml format or create new deployment-specific format?
   - Option A: Extend existing vendor.yaml with `labels` field (backwards compatible)
   - Option B: New VendorManifest kind for deployment-specific files
   - **Decision**: Option A for backwards compatibility - add `labels` as optional field to existing schema

3. **OCI Bundle Format**: What should bundled artifact structure look like?
   - Include entire stack hierarchy or flatten?
   - How to handle external dependencies (vendored components)?
   - Recommendation: Hierarchical with vendored deps included

4. **Vendor Cache Location**: Should `.atmos/vendor-cache/` be gitignored by default?
   - Option A: Always gitignore (ephemeral cache, rebuild on CI)
   - Option B: Optionally commit lock files only (not cached objects)
   - Recommendation: Option B - gitignore objects, commit lock files for reproducibility

5. **Release Retention**: Default policy for release records?
   - Keep last N releases per target?
   - Time-based retention (90 days)?
   - Recommendation: Keep last 10 per target, configurable

6. **Multi-Region Deployments**: How to handle deployments across regions?
   - Single deployment with multi-region targets?
   - Separate deployments per region?
   - Recommendation: Single deployment, targets can specify region

7. **Deployment Dependencies**: Should deployments depend on other deployments?
   - Example: app deployment depends on platform deployment
   - Recommendation: Yes, via `depends_on` field

## Risks & Mitigations

### Risk: Performance regression for non-deployment users
**Mitigation**: Deployments are opt-in. Default behavior unchanged. Benchmark both paths.

### Risk: Nixpacks provider compatibility issues
**Mitigation**: Dockerfile escape hatch. Test common language ecosystems (Go, Node.js, Python, Rust). Clear error messages. Community can contribute custom providers.

### Risk: Complex migration for large repositories
**Mitigation**: `migrate` command generates 80% of config. Comprehensive documentation. Community support.

### Risk: OCI bundle size bloat
**Mitigation**: Bundle only referenced files. Compress artifacts. Make bundling optional.

### Risk: Integration with existing CI/CD pipelines
**Mitigation**: Atmos runs as CLI tool in any CI. Export feature generates starter workflows. Don't replace CI.

## Alternatives Considered

### Alternative 1: Deployment as Stack Import
Use stack imports to define deployment boundaries.

**Pros**: Reuses existing mechanism
**Cons**: Stacks still load all files, no performance benefit, mixing concepts

**Decision**: Rejected. Deployments are first-class for clear separation and performance.

### Alternative 2: External Deployment Tool
Build separate tool that wraps Atmos.

**Pros**: No changes to Atmos core
**Cons**: Fragmented experience, duplication, maintenance burden

**Decision**: Rejected. Deployments are core to modern cloud native workflows.

### Alternative 3: Component-Level Deployments
Deployments map 1:1 with components.

**Pros**: Simpler model
**Cons**: No multi-component orchestration, no shared infrastructure

**Decision**: Rejected. Real deployments span multiple components and stacks.

## References

- [Nixpacks](https://nixpacks.com/)
- [Nixpacks GitHub](https://github.com/railwayapp/nixpacks)
- [OCI Specification](https://github.com/opencontainers/image-spec)
- [SBOM Formats](https://cyclonedx.org/)
- [SLSA Provenance](https://slsa.dev/provenance/)
- [Atmos Stack Configuration](https://atmos.tools/core-concepts/stacks/)
- [Atmos Component Documentation](https://atmos.tools/core-concepts/components/)
- [Cloud Native Buildpacks](https://buildpacks.io/) - Future alternative to nixpacks

## Appendix A: Example Deployments

### Simple API Deployment
```yaml
deployment:
  name: api
  stacks: ["platform", "ecs"]
  components:
    nixpack:
      api:
        vars:
          source: "./services/api"
          # nixpacks auto-detects language/framework
    terraform:
      ecs/api: {...}
  targets:
    dev: {...}
    prod: {...}
```

### Multi-Service Deployment
```yaml
deployment:
  name: microservices
  components:
    nixpack:
      api: {...}
      worker: {...}
      scheduler: {...}
    terraform:
      ecs/api: {...}
      ecs/worker: {...}
      ecs/scheduler: {...}
  targets:
    dev: {...}
    prod: {...}
```

### Lambda Deployment
```yaml
deployment:
  name: lambda-functions
  components:
    nixpack:
      user-handler: {...}
      order-handler: {...}
    terraform:
      lambda/user-handler: {...}
      lambda/order-handler: {...}
  targets:
    dev: {...}
    prod: {...}
```

### Environment-Specific Component Versions (Complete Example)

This example demonstrates pinning different component versions to different environments - dev uses bleeding edge, prod uses stable:

```yaml
# deployments/payment-service.yaml
deployment:
  name: payment-service
  description: "Payment processing microservice with PCI compliance"
  labels:
    service: payment
    team: payments
    compliance: pci-dss

  stacks:
    - "platform/vpc"
    - "platform/security"
    - "rds"
    - "ecs"

  # Environment-specific vendor versions
  vendor:
    components:
      # Dev uses latest RDS component (1.5.0) for testing new features
      - source: "github.com/cloudposse/terraform-aws-components//modules/rds"
        version: "1.5.0"  # Latest - includes new backup features
        targets: ["rds/payment-db"]
        labels:
          environment: ["dev"]

      # Staging uses release candidate (1.4.0) for pre-prod validation
      - source: "github.com/cloudposse/terraform-aws-components//modules/rds"
        version: "1.4.0"  # RC - tested in dev, ready for staging
        targets: ["rds/payment-db"]
        labels:
          environment: ["staging"]

      # Production uses stable version (1.3.5) - battle-tested
      - source: "github.com/cloudposse/terraform-aws-components//modules/rds"
        version: "1.3.5"  # Stable - proven in production
        targets: ["rds/payment-db"]
        labels:
          environment: ["prod"]

      # Dev/staging use beta ECS service with new ALB features
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "2.0.0-beta.3"  # Beta - testing new load balancer config
        targets: ["ecs/payment-api"]
        labels:
          environment: ["dev", "staging"]

      # Production uses stable ECS service
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "1.8.2"  # Stable - current production version
        targets: ["ecs/payment-api"]
        labels:
          environment: ["prod"]

      # Security groups use same version across all environments (compliance)
      - source: "github.com/cloudposse/terraform-aws-components//modules/security-group"
        version: "0.3.0"  # Pinned - compliance requirement
        targets: ["security/payment-sg"]
        # No labels = applies to all environments

    auto_discover: true
    cache:
      enabled: true
      ttl: 12h

  components:
    nixpack:
      payment-api:
        metadata:
          labels:
            service: payment
            tier: api
        vars:
          source: "./services/payment-api"
          image:
            registry: "123456789012.dkr.ecr.us-east-1.amazonaws.com"
            name: "payment-api"
            tag: "{{ git.sha }}"

    terraform:
      rds/payment-db:
        metadata:
          labels:
            service: payment
            data: primary
        vars:
          identifier: "payment-db"
          engine: "postgres"
          engine_version: "15.3"
          # Version-specific features available based on vendored component
          # v1.5.0 (dev): supports new snapshot_retention_days
          # v1.3.5 (prod): uses older backup_retention_period

      ecs/payment-api:
        metadata:
          labels:
            service: payment
        vars:
          cluster_name: "payment-cluster"
          # Version-specific features
          # v2.0.0-beta.3 (dev/staging): new ALB target group deregistration_delay
          # v1.8.2 (prod): stable ALB configuration

      security/payment-sg:
        metadata:
          labels:
            service: payment
            compliance: pci-dss
        vars:
          name: "payment-sg"
          # Same version across all environments for compliance audit trail

  targets:
    dev:
      labels:
        environment: dev
      context:
        db_instance_class: "db.t3.small"  # Cost-optimized
        db_backup_retention: 1
        ecs_cpu: 256
        ecs_memory: 512
        ecs_desired_count: 1

    staging:
      labels:
        environment: staging
      context:
        db_instance_class: "db.t3.medium"
        db_backup_retention: 7
        ecs_cpu: 512
        ecs_memory: 1024
        ecs_desired_count: 2

    prod:
      labels:
        environment: prod
      context:
        db_instance_class: "db.r6g.xlarge"  # Production-grade
        db_backup_retention: 30  # Compliance requirement
        db_multi_az: true
        ecs_cpu: 1024
        ecs_memory: 2048
        ecs_desired_count: 4
        ecs_autoscaling:
          enabled: true
          min_capacity: 4
          max_capacity: 20
```

**Workflow Example: Progressive Version Rollout**

```bash
# Week 1: Test new RDS component (1.5.0) in dev
atmos vendor pull --deployment payment-service --target dev
atmos rollout plan payment-service --target dev
atmos rollout apply payment-service --target dev

# Week 2: Promote to staging (1.4.0 RC)
atmos vendor pull --deployment payment-service --target staging
atmos rollout plan payment-service --target staging
atmos rollout apply payment-service --target staging

# Week 4: After validation, promote stable (1.3.5) to prod
# (Version 1.4.0 becomes 1.3.5 after testing)
atmos vendor pull --deployment payment-service --target prod
atmos rollout plan payment-service --target prod
atmos rollout apply payment-service --target prod

# Check what versions are actually deployed
atmos vendor status --deployment payment-service
# Output:
# TARGET    COMPONENT           VERSION        DIGEST      STATUS
# dev       rds/payment-db      1.5.0          abc123...   cached
# dev       ecs/payment-api     2.0.0-beta.3   def456...   cached
# staging   rds/payment-db      1.4.0          ghi789...   cached
# staging   ecs/payment-api     2.0.0-beta.3   def456...   cached (shared)
# prod      rds/payment-db      1.3.5          jkl012...   cached
# prod      ecs/payment-api     1.8.2          mno345...   cached
```

**Content-Addressable Cache Benefits:**

```
Vendor cache structure for payment-service:

.atmos/vendor-cache/
├── objects/
│   └── sha256/
│       ├── abc123.../  (RDS 1.5.0)     → 12 MB
│       ├── ghi789.../  (RDS 1.4.0)     → 12 MB
│       ├── jkl012.../  (RDS 1.3.5)     → 11 MB
│       ├── def456.../  (ECS 2.0.0-β)   → 8 MB
│       └── mno345.../  (ECS 1.8.2)     → 7 MB
├── deployments/
│   └── payment-service/
│       ├── dev/
│       │   └── terraform/
│       │       ├── rds/              # Hard link/copy from objects/abc123...
│       │       └── ecs-service/      # Hard link/copy from objects/def456...
│       ├── staging/
│       │   └── terraform/
│       │       ├── rds/              # Hard link/copy from objects/ghi789...
│       │       └── ecs-service/      # Hard link from objects/def456... (shared!)
│       └── prod/
│           └── terraform/
│               ├── rds/              # Hard link/copy from objects/jkl012...
│               └── ecs-service/      # Hard link/copy from objects/mno345...

Total storage (Unix/Mac with hard links): 50 MB (5 unique component versions)
Total storage (Windows with copies):      75 MB (all files copied per deployment)
Without content-addressable cache:        150 MB (no deduplication at all)

Deduplication savings:
- Unix/Mac: 67% savings (hard links)
- Windows:  50% savings (copies but shared cache objects)
```

## Appendix B: JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Atmos Deployment",
  "type": "object",
  "required": ["deployment"],
  "properties": {
    "deployment": {
      "type": "object",
      "required": ["name", "stacks"],
      "properties": {
        "name": {
          "type": "string",
          "pattern": "^[a-z0-9][a-z0-9-]*[a-z0-9]$"
        },
        "description": {
          "type": "string"
        },
        "labels": {
          "type": "object",
          "additionalProperties": {
            "type": "string"
          }
        },
        "stacks": {
          "type": "array",
          "items": {
            "type": "string"
          },
          "minItems": 1
        },
        "context": {
          "type": "object",
          "properties": {
            "default_target": {
              "type": "string"
            },
            "promote_by": {
              "type": "string",
              "enum": ["digest", "tag"]
            }
          }
        },
        "components": {
          "type": "object",
          "properties": {
            "nixpack": {
              "type": "object",
              "additionalProperties": {
                "type": "object"
              }
            },
            "terraform": {
              "type": "object",
              "additionalProperties": {
                "type": "object"
              }
            },
            "helmfile": {
              "type": "object",
              "additionalProperties": {
                "type": "object"
              }
            }
          }
        },
        "targets": {
          "type": "object",
          "additionalProperties": {
            "type": "object",
            "properties": {
              "labels": {
                "type": "object",
                "additionalProperties": {
                  "type": "string"
                }
              },
              "context": {
                "type": "object"
              }
            }
          }
        }
      }
    }
  }
}
```
