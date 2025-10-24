# Comparing Stacks with Deployments

This document clarifies the relationship between Atmos Stacks and Atmos Deployments, helping you understand when to use each and how they work together.

## TL;DR

- **Stacks** = Configuration layer (YAML files defining component settings)
- **Deployments** = Orchestration layer (grouping components into deployable applications)
- You still use stacks. Deployments reference them.
- Deployments don't replace stacks—they organize them.

## What Are Stacks?

**Stacks are Atmos's configuration layer.** They define:
- Component configurations (variables, settings)
- Inheritance hierarchies (imports, merging)
- Environment-specific overrides
- Template-based configuration

**Example stack:**
```yaml
# stacks/prod/us-east-1.yaml
import:
  - orgs/acme/platform/networking/_defaults
  - orgs/acme/platform/ecs/_defaults

components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.0.0.0/16"

    ecs/api:
      vars:
        cpu: 2048
        memory: 4096
        desired_count: 8
```

**What stacks do well:**
- ✅ Define component configuration
- ✅ Share configuration via inheritance
- ✅ Override values per environment
- ✅ Template complex configurations

**What stacks struggle with at scale:**
- ❌ Implicit deployment scope (which components = one app?)
- ❌ Must process entire repository to deploy one component
- ❌ No built-in deployment tracking
- ❌ No isolation for concurrent operations

## What Are Deployments?

**Deployments are Atmos's orchestration layer.** They define:
- Which components comprise an application
- Which stacks to load (scoped processing)
- Which targets (environments) exist
- Deployment-specific vendoring

**Example deployment:**
```yaml
# deployments/api.yaml
deployment:
  name: api

  # Explicit scope: Only load these stacks
  stacks:
    - "platform/vpc"
    - "platform/ecs"
    - "ecr"

  # Explicit grouping: These components = this app
  components:
    nixpack:
      api:
        vars:
          source: "./services/api"

    terraform:
      ecr/api:
        vars:
          name: "api"
      ecs/service-api:
        vars:
          cluster_name: "app-ecs"

  # Explicit targets: These are the environments
  targets:
    dev:
      context: { cpu: 256, memory: 512 }
    staging:
      context: { cpu: 512, memory: 1024 }
    prod:
      context: { cpu: 2048, memory: 4096 }
```

**What deployments do well:**
- ✅ Explicit application boundaries
- ✅ Fast (only process declared stacks)
- ✅ Built-in deployment tracking (Git-based)
- ✅ Concurrent operations (isolated working directories)
- ✅ Just-in-time vendoring (per deployment + target)

## How They Work Together

**Deployments orchestrate components that are configured by stacks.**

```
┌─────────────────────────────────────────────────────────┐
│ Deployment: api                                         │
│                                                         │
│  stacks: ["platform/vpc", "platform/ecs", "ecr"]       │
│                                                         │
│  ┌───────────────────────────────────────────┐         │
│  │ Stack: platform/ecs                       │         │
│  │                                           │         │
│  │ components:                               │         │
│  │   terraform:                              │         │
│  │     ecs/service-api:                      │         │
│  │       vars:                               │         │
│  │         cluster_name: "app-ecs"           │         │
│  │         cpu: 512                          │         │
│  └───────────────────────────────────────────┘         │
│                          ↓                              │
│              Deployment references stack                │
│              Stack provides configuration               │
│                          ↓                              │
│  ┌───────────────────────────────────────────┐         │
│  │ Component: ecs/service-api                │         │
│  │                                           │         │
│  │ Deployed with configuration from stack   │         │
│  └───────────────────────────────────────────┘         │
└─────────────────────────────────────────────────────────┘
```

**Flow:**
1. Deployment declares: "Load these stacks"
2. Atmos loads only those stacks (fast, scoped)
3. Stacks provide component configuration
4. Deployment deploys components with that config

## Comparison Table

| Aspect | Stacks | Deployments |
|--------|--------|-------------|
| **Purpose** | Configuration | Orchestration |
| **Defines** | Component settings | Application grouping |
| **Scope** | Repository-wide | Explicit, scoped |
| **Performance** | Processes everything | Processes only what's declared |
| **Inheritance** | YAML imports | Stack references |
| **Tracking** | None built-in | Git-based (refs, tags, notes) |
| **Concurrency** | File conflicts | Isolated working directories |
| **Vendoring** | Global (folder-based) | Per-deployment + target (JIT) |
| **When to use** | Defining component config | Deploying applications |

## Development Workflows

### Stack-Based Workflow (Still Works)

**Use case:** Developing and testing individual components.

```bash
# Edit component configuration
vim stacks/dev/us-east-1.yaml

# Plan component using stack
atmos terraform plan ecs/api -s dev

# Apply component using stack
atmos terraform apply ecs/api -s dev
```

**When to use:**
- Testing individual components
- Iterating on component configuration
- Quick prototyping
- Component development

### Deployment-Based Workflow (New)

**Use case:** Deploying complete applications across environments.

```bash
# Build entire application
atmos deployment build api --target dev

# Deploy entire application
atmos deployment rollout api --target dev

# Promote to production
atmos deployment rollout api --target prod --promote-from staging

# Check deployment status
atmos deployment status api --target prod
```

**When to use:**
- Deploying complete applications
- Promoting between environments
- Tracking deployment history
- Enterprise-scale infrastructure

### Combined Workflow (Recommended)

**Local development:**
```bash
# Develop component with stacks (fast iteration)
atmos terraform plan ecs/api -s dev
atmos terraform apply ecs/api -s dev
```

**CI/CD deployment:**
```bash
# Deploy application with deployments (orchestration)
atmos deployment build api --target dev
atmos deployment rollout api --target dev
```

## Key Differences Explained

### 1. Scope Definition

**Stacks (Implicit):**
```yaml
# stacks/prod/us-east-1.yaml
import:
  - catalog/networking
  - catalog/ecs
  - catalog/rds
  - catalog/redis
  - catalog/s3
  # ... dozens more imports

# Which components = "the application"? Unclear.
```

**Deployments (Explicit):**
```yaml
# deployments/api.yaml
stacks:
  - "platform/vpc"
  - "platform/ecs"
  - "ecr"

# Clear: Only these stacks are part of this deployment
```

### 2. Performance

**Stacks:**
```bash
atmos terraform plan ecs/api -s prod
# Processes: ALL stacks (1000+ files)
# Time: 45 seconds
```

**Deployments:**
```bash
atmos deployment rollout api --target prod
# Processes: Only declared stacks (3 files)
# Time: 2 seconds
```

### 3. Deployment Tracking

**Stacks:**
```bash
# How do I know what's deployed to prod?
# → Check Terraform state files
# → Manual tracking
# → No built-in answer
```

**Deployments:**
```bash
atmos deployment status api --target prod
# → Instant answer from Git refs
# → Complete history in Git tags
# → SBOM attached as Git notes
```

### 4. Concurrency

**Stacks:**
```bash
# Terminal 1
atmos terraform plan ecs/api -s prod
# Generates: components/terraform/ecs-api/backend.tf

# Terminal 2 (conflict!)
atmos terraform plan ecs/api -s staging
# Overwrites: components/terraform/ecs-api/backend.tf
```

**Deployments:**
```bash
# Terminal 1
atmos deployment rollout api --target prod
# Working dir: .atmos/workdir/api-prod-ecs-service-uuid1/

# Terminal 2 (no conflict)
atmos deployment rollout api --target staging
# Working dir: .atmos/workdir/api-staging-ecs-service-uuid2/
```

## When to Use What

### Use Stacks When:
- ✅ Defining component configuration
- ✅ Developing individual components
- ✅ Testing configuration changes
- ✅ Quick iteration on settings
- ✅ Managing configuration inheritance

### Use Deployments When:
- ✅ Deploying complete applications
- ✅ Promoting between environments
- ✅ Tracking deployment history
- ✅ Need concurrent deployments
- ✅ Enterprise-scale infrastructure (1000+ components)
- ✅ Different component versions per environment
- ✅ Compliance/audit requirements

### Use Both When:
- ✅ Develop components with stacks
- ✅ Deploy applications with deployments
- ✅ This is the recommended approach

## Migration Path

You don't need to migrate stacks. Deployments reference them.

**Before (stack-only):**
```bash
# Deploy components individually
atmos terraform apply ecr/api -s prod
atmos terraform apply ecs/taskdef-api -s prod
atmos terraform apply ecs/service-api -s prod
```

**After (with deployments):**
```yaml
# Create deployment that references existing stacks
# deployments/api.yaml
deployment:
  name: api

  stacks:
    - "prod/us-east-1"  # Reference existing stack

  components:
    terraform:
      ecr/api: {}       # Already configured in stack
      ecs/taskdef-api: {}
      ecs/service-api: {}
```

```bash
# Deploy all at once
atmos deployment rollout api --target prod
```

**Your stacks don't change.** You just add a deployment manifest that references them.

## Common Misconceptions

### ❌ "Deployments replace stacks"

**No.** Deployments orchestrate components that are configured by stacks.

### ❌ "I need to rewrite my stacks to use deployments"

**No.** Create a deployment manifest that references your existing stacks.

### ❌ "Deployments are just another way to organize stacks"

**No.** Deployments add orchestration, tracking, and scoping that stacks don't provide.

### ❌ "I can't use `atmos terraform` commands anymore"

**No.** `atmos terraform` commands still work. Deployments are additive.

### ❌ "Small teams don't need deployments"

**Maybe.** If you have <10 components and one environment, stacks alone are fine. But deployments still provide value (tracking, promotion paths, clearer organization).

## The Mental Model

Think of it like this:

**Stacks = Configuration files**
- Like `application.yaml` or `.env` files
- Define settings for components
- Provide inheritance and templating

**Deployments = Application manifests**
- Like `docker-compose.yaml` or `package.json`
- Define what comprises an application
- Orchestrate deployment across environments

**Together:**
- Stacks configure components
- Deployments deploy them as cohesive applications

## Examples

### Small Application

**Stacks:**
```yaml
# stacks/dev.yaml
components:
  terraform:
    app:
      vars:
        instance_type: "t3.small"
```

**Deployment (optional but helpful):**
```yaml
# deployments/app.yaml
deployment:
  name: app
  stacks: ["dev"]
  components:
    terraform:
      app: {}
  targets:
    dev:
      context: { instance_type: "t3.small" }
```

**Benefit:** Explicit definition, deployment tracking.

### Large Enterprise Platform

**Stacks (hundreds of files):**
```
stacks/
  orgs/acme/
    platform/
      networking/
      compute/
      storage/
      databases/
      security/
    services/
      api/
      worker/
      frontend/
  regions/
    us-east-1/
    us-west-2/
    eu-west-1/
```

**Deployments (clear boundaries):**
```yaml
# deployments/api.yaml - Only loads what it needs
deployment:
  name: api
  stacks:
    - "orgs/acme/platform/networking"
    - "orgs/acme/platform/compute"
    - "orgs/acme/services/api"
  # Fast: Only processes 3 stacks, not hundreds
```

**Benefit:** 10-20x faster processing, clear scope, deployment tracking.

## See Also

- **[problem-statement.md](./problem-statement.md)** - Why deployments solve enterprise-scale challenges
- **[definitions.md](./definitions.md)** - Terminology reference
- **[adoption-guide.md](./adoption-guide.md)** - How to adopt deployments in existing repos
- **[configuration.md](./configuration.md)** - Complete deployment schema
