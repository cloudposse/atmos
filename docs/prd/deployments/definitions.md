# Atmos Deployments - Definitions

This document defines all core concepts in the Atmos Deployments system. These terms are used consistently throughout the PRD and implementation.

## Core Concepts

### Deployment

**Definition:** An explicit YAML configuration file that defines an isolated subset of stacks and components representing a complete application or service.

**Purpose:**
- Scopes what Atmos processes (10-20x faster than full repository scan)
- Declares which stacks to load, which components to manage
- Defines targets (environments) for the application

**Location:** `deployments/<deployment-name>.yaml`

**Example:**
```yaml
# deployments/payment-service.yaml
deployment:
  name: payment-service
  description: Payment processing REST API

  stacks:
    - "platform/vpc"
    - "platform/eks"
    - "ecr"
    - "ecs"

  components:
    nixpack:
      api:
        vars:
          source: "./services/api"
    terraform:
      ecr/api:
        vars:
          name: "payment-api"
      ecs/service-api:
        vars:
          cluster_name: "app-cluster"

  targets:
    dev: { cpu: 256, memory: 512 }
    staging: { cpu: 512, memory: 1024 }
    prod: { cpu: 2048, memory: 4096 }
```

**Key characteristics:**
- Explicit scope (no heuristics)
- Self-contained (declares dependencies)
- Environment-agnostic (targets provide environment config)

### Target

**Definition:** An environment-specific variant of a deployment (e.g., `dev`, `staging`, `prod`).

**Purpose:**
- Provides environment-specific configuration overrides
- Maps to SDLC stages (development, staging, production)
- Enables promotion paths (dev → staging → prod)

**Characteristics:**
- Same components, different configuration
- Different resource allocations (CPU, memory, replicas)
- Can have different component versions (via vendoring)

**Example:**
```yaml
targets:
  dev:
    labels:
      environment: dev
    context:
      cpu: 256
      memory: 512
      replicas: 1
      log_level: "debug"

  prod:
    labels:
      environment: prod
    context:
      cpu: 2048
      memory: 4096
      replicas: 8
      autoscale:
        enabled: true
        min: 8
        max: 32
      log_level: "warn"
```

**Not to be confused with:**
- **Stacks:** Targets reference stacks but are not stacks themselves
- **Stages:** Targets are environments (nouns), stages are phases (verbs)

### Component

**Definition:** A unit of infrastructure managed by Atmos (Terraform module, Helm chart, Nixpack container, etc.).

**Types:**
- **`terraform`** - Terraform/OpenTofu modules
- **`nixpack`** - Container images built from source code
- **`helmfile`** - Helm charts
- **`lambda`** - AWS Lambda functions (future)

**Relationship to deployments:**
- Deployments **orchestrate** components
- Components are still developed independently
- Same component can be used in multiple deployments

**Example:**
```yaml
components:
  nixpack:
    api:
      vars:
        source: "./services/api"
      metadata:
        depends_on: ["terraform/ecr/api"]

  terraform:
    ecr/api:
      vars:
        name: "payment-api"
        image_scanning: true

    ecs/service-api:
      vars:
        cluster_name: "app-cluster"
        task_definition: "payment-api"
```

### Stack

**Definition:** An Atmos YAML file that defines component configurations, imports other stacks, and applies inheritance.

**Relationship to deployments:**
- Deployments **reference** stacks (via `stacks:` field)
- Stacks provide component configuration
- Same stacks can be used by multiple deployments
- Deployments scope which stacks to load

**Example:**
```yaml
# deployments/payment-service.yaml
stacks:
  - "platform/vpc"      # Load only these stacks
  - "platform/eks"
  - "ecr"
  - "ecs"

# Does NOT load:
# - Other services' stacks
# - Unrelated platform stacks
# - Other regions
```

**Key insight:** Deployments make stack loading explicit and scoped.

### Build

**Definition:** The process of creating container images from source code.

**Applies to:** Nixpack components (auto-detects Go, Node.js, Python, Rust, etc.)

**Command:** `atmos deployment build <deployment> --target <target>`

**Input:**
- Source code directory
- Nixpack configuration (optional)
- Target context (CPU, memory, environment variables)

**Output:**
- Container image pushed to registry
- Image digest (e.g., `sha256:abc123...`)
- SBOM (Software Bill of Materials) as Git note

**Example:**
```bash
atmos deployment build payment-service --target dev

# → Builds nixpack component: api
# → Pushes to: registry.example.com/payment-api:sha256-abc123
# → Generates SBOM
# → Attaches SBOM as git note: refs/notes/atmos/payment-service/sbom
```

**Not applicable to:**
- Terraform components (no build phase)
- Pre-built container images (already built)
- Helmfile components (charts are already packaged)

### Build Record

**Definition:** Metadata about a container build, stored in Git.

**Storage:** Git refs and tags track build records

**Contents:**
- Image digest (immutable reference)
- Build timestamp
- Git SHA (source code commit)
- Builder information
- Target environment

**Purpose:**
- Track which builds exist
- Enable promotion of specific builds
- Audit trail for security compliance

### Release (Future Concept)

**Definition:** An immutable snapshot of a deployment at a specific point in time.

**Note:** This concept is from the original FULL_REFERENCE.md but has been deprioritized. In the current Git-based tracking approach, we use **deployment tags** instead of separate release records.

**Original concept:**
- Stored as `releases/<deployment>/<target>/release-<id>.yaml`
- Contained image digests, Git SHA, SBOM, provenance

**Current approach:**
- Git annotated tags serve as release records
- Tags contain all deployment metadata in YAML format
- No separate release files needed

**If we bring back releases:**
- They would complement Git tags
- Could bundle additional artifacts (lock files, generated configs)
- Would enable OCI bundle format

### Rollout

**Definition:** The act of deploying infrastructure components to a target environment.

**Command:** `atmos deployment rollout <deployment> --target <target>`

**What it does:**
1. Validates promotion path (if promoting)
2. Ensures containers are built (or uses promoted build)
3. Runs `terraform apply` on all Terraform components in dependency order
4. Updates Git refs to track deployment state
5. Creates Git tags with deployment metadata

**Example:**
```bash
# Deploy to dev (builds + applies)
atmos deployment rollout payment-service --target dev

# Promote from dev to staging (no rebuild)
atmos deployment rollout payment-service --target staging --promote-from dev

# Rollback prod to previous commit
atmos deployment rollout payment-service --target prod --git-sha abc123def
```

**Behind the scenes:**
```bash
# For each component in dependency order:
terraform init
terraform plan
terraform apply

# Updates Git tracking:
git update-ref refs/atmos/deployments/payment-service/prod/api <sha>
git tag -a atmos/deployments/payment-service/prod/api/2025-01-22T12-00-00Z
git notes --ref=atmos/payment-service/approvals add <approval-data>
```

### Promotion

**Definition:** Deploying a build from one target to another without rebuilding.

**Purpose:**
- Same container images across environments (immutable)
- Faster deployments (no rebuild)
- Validated binaries (tested in lower environments)
- Auditable promotion path

**Example:**
```bash
# Build in dev
atmos deployment build payment-service --target dev
# → Image: sha256:abc123...

# Deploy to dev
atmos deployment rollout payment-service --target dev
# → Uses image: sha256:abc123...

# Promote to staging (same image)
atmos deployment rollout payment-service --target staging --promote-from dev
# → Uses same image: sha256:abc123...
# → New Terraform config (staging context)

# Promote to prod (requires approval in CI/CD)
atmos deployment rollout payment-service --target prod --promote-from staging
# → Uses same image: sha256:abc123...
# → New Terraform config (prod context)
```

**Promotion paths are validated:**
```yaml
# deployments/payment-service.yaml
targets:
  staging:
    promotion:
      from: dev

  prod:
    promotion:
      from: staging
      requires_approval: true
```

### Promotion Path

**Definition:** The allowed progression of deployments between targets.

**Purpose:**
- Enforce quality gates
- Prevent accidental production deployments
- Require approval for sensitive environments

**Configuration:**
```yaml
targets:
  dev:
    # No promotion config = entry point

  staging:
    promotion:
      from: dev  # Can only promote from dev

  prod:
    promotion:
      from: staging  # Must come from staging
      requires_approval: true  # Needs CI/CD approval
```

**Validation:**
```bash
# Valid: Following promotion path
atmos deployment rollout api --target staging --promote-from dev
# ✓ Allowed: dev → staging

# Invalid: Skipping staging
atmos deployment rollout api --target prod --promote-from dev
# ✗ Error: Cannot promote directly from dev to prod
```

**Visual representation:**
```
dev → staging → prod
        ↓
       QA (parallel path)
```

### Stage vs Target (Disambiguation)

**Problem:** "Stage" has two meanings in software delivery.

**Stage as Environment (Deprecated Term):**
```
dev stage → staging stage → prod stage
```
We call these **targets** to avoid confusion.

**Stage as Deployment Phase (Verb):**
```
build stage → test stage → deploy stage → validate stage
```
We use **verbs** for these: `build`, `test`, `rollout`, `validate`.

**Clear terminology:**
- **Target** = Environment (noun): `dev`, `staging`, `prod`
- **Verb** = Phase (action): `build`, `rollout`, `promote`

**Commands follow this pattern:**
```bash
atmos deployment <verb> <deployment> --target <environment>
                   ↑                            ↑
                  Phase                    Environment

atmos deployment build payment-service --target dev
atmos deployment rollout payment-service --target prod
```

### SBOM (Software Bill of Materials)

**Definition:** A comprehensive inventory of all software components and dependencies in a deployment.

**Format:** CycloneDX JSON (industry standard)

**Storage:** Git notes (per commit)

**Generated for:**
- Nixpack components (container + application dependencies)
- Terraform components (modules + providers)
- Complete deployments (aggregated from all components)

**Example:**
```bash
# SBOM generated during build
atmos deployment build payment-service --target dev
# → Attaches SBOM as git note: refs/notes/atmos/payment-service/sbom

# View SBOM for deployed version
atmos deployment sbom payment-service --target prod
# → Retrieves SBOM from git note for deployed commit

# Output:
# Components: 3
#   payment-api (container)
#   └── golang.org/x/net v0.17.0
#   └── github.com/gin-gonic/gin v1.9.1
#   ecs-service (terraform)
#   └── terraform-aws-modules/ecs v5.2.0
#   ecr (terraform)
#   └── terraform-aws-modules/ecr v2.0.0
```

### Working Directory

**Definition:** An isolated copy of a component directory used for a single operation.

**Purpose:** Enables concurrent operations on the same component.

**Implementation:**
```
.atmos/workdir/
  <deployment>-<target>-<component>-<uuid>/
    # Copy of component directory
    # Generated backend.tf
    # Generated providers.tf
    # Temporary .terraform/
```

**Lifecycle:**
1. Created before `terraform init`
2. Used for `terraform plan` and `terraform apply`
3. Cleaned up after operation completes
4. Retained on error for debugging

**Benefits:**
- Multiple targets can be deployed simultaneously
- Local dev + CI/CD can run concurrently
- No file conflicts between operations

### Vendor Cache

**Definition:** Content-addressable storage for vendored components.

**Purpose:**
- Just-in-time vendoring per deployment + target
- Multiple versions of same component coexist
- Fast lookups (keyed by source + version + digest)

**Structure:**
```
.atmos/vendor-cache/
  <sha256-hash>/          # Content-addressable directory
    main.tf
    variables.tf
    outputs.tf
  .cache-index.yaml       # Maps source+version to hash
```

**Workflow:**
```bash
# 1. Check cache
lookup(source="github.com/.../ecs-service", version="1.5.0")
# → Cache hit: .atmos/vendor-cache/abc123...

# 2. Use cached version (or vendor if miss)
# 3. Copy to working directory
# 4. Execute terraform
```

**Benefits:**
- Dev uses v1.5.0, prod uses v1.3.5 (no folder duplication)
- Instant vendoring (cache hit)
- Deterministic (content-addressable)

### Git Ref (Deployment Tracking)

**Definition:** A lightweight Git reference pointing to the currently deployed commit for a target/component.

**Format:** `refs/atmos/deployments/<deployment>/<target>/<component>`

**Purpose:**
- Track "what's deployed now" for each environment
- Fast queries (Git show-ref)
- Mutable (updates in place)

**Example:**
```bash
# Current deployment state
refs/atmos/deployments/payment-service/prod/api → abc123def
refs/atmos/deployments/payment-service/prod/worker → abc123def

# Query current state
git show-ref refs/atmos/deployments/payment-service/prod/api
# Output: abc123def refs/atmos/deployments/payment-service/prod/api
```

**User never sees this:** Accessed via `atmos deployment status` commands.

### Git Tag (Deployment History)

**Definition:** An immutable Git annotated tag containing deployment metadata.

**Format:** `atmos/deployments/<deployment>/<target>/<component>/<timestamp>`

**Purpose:**
- Immutable audit trail
- Deployment history
- Metadata storage (who, when, why)

**Tag message format:**
```yaml
atmos.tools/v1alpha1
kind: Deployment
---
deployment: payment-service
target: prod
component: api
deployed_by: ci@example.com
timestamp: 2025-01-22T12:00:00Z
status: success
promoted_from: staging
metadata:
  pr: "#482"
  approver: alice@example.com
```

**User never sees this:** Accessed via `atmos deployment history` commands.

### Git Note (Per-Commit Metadata)

**Definition:** Additional metadata attached to a Git commit without modifying the commit.

**Format:** `refs/notes/atmos/<deployment>/<namespace>`

**Purpose:**
- Attach SBOM to commit
- Store test results
- Record approvals
- Security scan results

**Example namespaces:**
```
refs/notes/atmos/payment-service/sbom
refs/notes/atmos/payment-service/test-results
refs/notes/atmos/payment-service/approvals
refs/notes/atmos/payment-service/scan-results
```

**User never sees this:** Accessed via `atmos deployment sbom` and similar commands.

## Terminology Guidelines

### Prefer

- **Target** instead of "environment" or "stage"
- **Rollout** instead of "deploy" or "apply"
- **Promotion** instead of "copy" or "move"
- **Build** instead of "compile" or "package"
- **Component** instead of "module" or "service"

### Avoid

- **Stage** (ambiguous - environment or phase?)
- **Deploy** (vague - what kind of deployment?)
- **Stack** when you mean "deployment" (stacks are lower-level)
- **Environment** (we use "target" for clarity)

### Verb Patterns

All deployment commands follow this pattern:

```bash
atmos deployment <verb> <deployment> --target <target> [flags]
                   ↑                            ↑
                  Action                    Environment

# Examples:
atmos deployment build payment-service --target dev
atmos deployment rollout payment-service --target prod
atmos deployment status payment-service --target staging
atmos deployment history payment-service --target prod
atmos deployment diff payment-service --from-target prod --to-target staging
```

## See Also

- **[problem-statement.md](./problem-statement.md)** - Why we need deployments
- **[overview.md](./overview.md)** - High-level architecture
- **[configuration.md](./configuration.md)** - Complete YAML schema reference
