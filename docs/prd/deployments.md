# Atmos Deployments PRD

## Overview

Deployments are a new first-class concept in Atmos that enable reproducible, efficient application lifecycle management from build through production rollout. Like vendoring, deployments become a core primitive with their own configuration schema, CLI commands, and workflows.

A deployment defines an isolated subset of stacks and components that represent a complete application or service, enabling Atmos to process only the relevant configuration files rather than scanning the entire repository. This dramatically improves performance and creates clear boundaries for build, test, release, and rollout operations.

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

### Nice to Have (P2)
18. **CI Export**: Generate minimal CI stubs for popular platforms (GitHub Actions, GitLab CI)
19. **Dockerfile Escape**: Use Dockerfile when CNB doesn't meet requirements
20. **Multi-Component Deployments**: Coordinate releases across multiple nixpack components
21. **Deployment Status**: Track which releases are deployed to which targets
22. **Drift Detection**: Identify when infrastructure differs from expected release
23. **Vendor Garbage Collection**: Clean up unused vendored components

## Non-Goals

- **Not a CI replacement**: Atmos runs in CI but doesn't become a CI orchestrator
- **Not a new infrastructure primitive**: Rollouts always update existing Terraform/Helm/Lambda components
- **Not a test framework**: Teams bring their own test commands/frameworks
- **Not replacing stacks**: Deployments reference stacks, don't replace them
- **Not auto-inheritance**: Targets use explicit `context` for overrides, not automatic inheritance

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
    ├── refs/
    │   ├── github.com/
    │   │   └── cloudposse/
    │   │       └── terraform-aws-components/
    │   │           └── modules/
    │   │               └── ecs-service/
    │   │                   ├── 1.2.3 -> ../../../../objects/sha256/abc123...
    │   │                   ├── 1.2.4 -> ../../../../objects/sha256/def456...
    │   │                   └── latest -> 1.2.4
    │   └── registry.terraform.io/
    │       └── cloudposse/
    │           └── ecs-service/
    │               └── aws/
    │                   ├── 2.1.0 -> ../../../../objects/sha256/xyz789...
    │                   └── 2.1.5 -> ../../../../objects/sha256/aaa111...
    ├── deployments/
    │   ├── api/
    │   │   ├── dev/
    │   │   │   ├── terraform/
    │   │   │   │   └── ecs-service -> ../../../../refs/github.com/.../1.2.3
    │   │   │   └── .vendor-lock.yaml
    │   │   ├── staging/
    │   │   │   └── terraform/
    │   │   │       └── ecs-service -> ../../../../refs/github.com/.../1.2.4
    │   │   └── prod/
    │   │       └── terraform/
    │   │           └── ecs-service -> ../../../../refs/registry.terraform.io/.../2.1.5
    │   └── worker/
    │       └── dev/
    └── .cache-index.yaml  # Global cache metadata
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
atmos release create api --target dev --bundle
# Creates OCI artifact containing:
# - Built container images (digests)
# - Deployment configuration
# - Stack configurations
# - Vendored components (from JIT cache)
# - Release metadata

# Result: fully self-contained artifact for rollback
atmos rollout apply api --target prod --bundle oci://registry/api:release-abc123
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

### Build Phase

```bash
# Build all nixpack components in deployment
atmos build api --target dev

# Build specific component
atmos build api --component api --target dev

# Build with Git reference for provenance
atmos build api --target dev --ref $(git rev-parse HEAD)

# Build without publishing (local testing)
atmos build api --target dev --no-publish

# Build with custom nixpacks options
atmos build api --target dev \
  --pkgs ffmpeg,imagemagick \
  --install-cmd "npm ci" \
  --build-cmd "npm run build" \
  --start-cmd "npm start"

# Build with Dockerfile (escape hatch)
# If Dockerfile exists in component source, nixpacks uses it automatically
atmos build api --target dev --dockerfile ./services/api/Dockerfile
```

### Test Phase

```bash
# Run tests inside built container
atmos test api --target dev

# Run with custom test command
atmos test api --target dev -- command "go test ./... -v"

# Run with JUnit output
atmos test api --target dev -- command "go test ./..." --report ./out/junit.xml

# Run with volume mounts
atmos test api --target dev -- command "pytest -v" --mount ./tests:/app/tests

# Run integration tests
atmos test api --target dev --suite integration
```

### Release Phase

```bash
# Create release from latest build
atmos release create api --target dev

# Create release with annotations
atmos release create api --target dev \
  --annotate "description=Add user authentication" \
  --annotate "pr=#482" \
  --annotate "jira=PROJ-123"

# Create release with specific digest
atmos release create api --target dev --digest sha256:abc123...

# List releases
atmos release list api
atmos release list api --target dev
atmos release list api --target dev --limit 10

# Show release details
atmos release describe api abc123

# Create OCI bundle artifact (self-contained deployment)
atmos release create api --target dev --bundle
# Creates OCI artifact with:
# - Image digest
# - Deployment configuration
# - Stack configurations
# - Component configurations
# - Release metadata
```

### Rollout Phase

```bash
# Plan rollout (like terraform plan)
atmos rollout plan api --target dev

# Plan rollout for specific release
atmos rollout plan api --target dev --release abc123

# Apply rollout (like terraform apply)
atmos rollout apply api --target dev

# Apply specific release
atmos rollout apply api --target dev --release abc123

# Auto-approve (for CI)
atmos rollout apply api --target dev --auto-approve

# Promote release to another target (no rebuild)
atmos rollout apply api --target staging --release abc123
atmos rollout apply api --target prod --release abc123

# Rollback to previous release
atmos release list api --target prod --limit 5
atmos rollout apply api --target prod --release xyz789

# Show rollout status
atmos rollout status api
atmos rollout status api --target prod

# Detect drift (compare deployed vs expected)
atmos rollout drift api --target prod
```

### Complete Workflow Examples

**Development Workflow**:
```bash
# 1. Build locally
atmos build api --target dev --no-publish

# 2. Test locally
atmos test api --target dev

# 3. Build and publish
atmos build api --target dev

# 4. Create release
atmos release create api --target dev --annotate "pr=#482"

# 5. Rollout to dev
atmos rollout apply api --target dev --auto-approve
```

**CI/CD Workflow**:
```bash
# Pull request (build + test only)
atmos build api --target dev
atmos test api --target dev

# Merge to main (release + rollout dev)
atmos build api --target dev --ref $GITHUB_SHA
atmos test api --target dev
atmos release create api --target dev --annotate "sha=$GITHUB_SHA"
atmos rollout apply api --target dev --auto-approve

# Manual promotion to staging
RELEASE_ID=$(atmos release list api --target dev --limit 1 --format json | jq -r '.id')
atmos rollout apply api --target staging --release $RELEASE_ID

# Manual promotion to prod
atmos rollout apply api --target prod --release $RELEASE_ID
```

**Rollback Workflow**:
```bash
# 1. List recent releases
atmos release list api --target prod --limit 10

# 2. Inspect previous release
atmos release describe api xyz789

# 3. Plan rollback
atmos rollout plan api --target prod --release xyz789

# 4. Execute rollback
atmos rollout apply api --target prod --release xyz789
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
- [ ] `atmos build` command implementation
- [ ] Image digest capture and storage
- [ ] Local build (no publish) support
- [ ] CI build with provenance

**Week 3-4: Release Commands**
- [ ] Release record schema and storage
- [ ] `atmos release create` command
- [ ] `atmos release list` command
- [ ] `atmos release describe` command

**Week 4-6: Testing & Documentation**
- [ ] Build/release integration tests
- [ ] Nixpacks compatibility testing (Go, Node.js, Python, Rust)
- [ ] Documentation with examples
- [ ] CI workflow examples

### Phase 3: Test & Rollout (4-6 weeks)

**Week 1-2: Test Phase**
- [ ] Container test execution framework
- [ ] `atmos test` command implementation
- [ ] Test report collection (JUnit, etc.)
- [ ] Volume mount support for test data

**Week 2-4: Rollout Phase**
- [ ] Label-based component binding
- [ ] Digest injection into Terraform/Helm/Lambda
- [ ] `atmos rollout plan` command
- [ ] `atmos rollout apply` command

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

**Week 4-6: CI Export**
- [ ] GitHub Actions workflow generation
- [ ] GitLab CI pipeline generation
- [ ] Generic CI template system

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

### SBOM Generation
- Automatic SBOM generation for all builds
- Multiple formats: CycloneDX, SPDX
- Integrated with vulnerability scanning tools

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
