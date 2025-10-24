# Atmos Deployments PRD

## Introduction

**Software delivery is hard—and it should be simple.** Whether you're deploying your first service or managing thousands of components across hundreds of environments, you face the same fundamental questions: *What is this application? How do I deploy it safely? What's running in production right now?*

Atmos Deployments solves these universal challenges with explicit, Git-tracked deployment orchestration. **No external databases. No complex state management. No deployment drift.** Just clear definitions, safe promotions, and complete visibility into what's deployed where.

From day-one clarity for newcomers to enterprise-scale performance for thousands of components, deployments make software delivery reliable and repeatable for everyone. **[See problem-statement.md](./problem-statement.md) for the complete problem analysis.**

## Proposed Solution

**Atmos Deployments** introduces explicit, scoped application orchestration built on Git-based tracking—no external databases required. A deployment is a YAML manifest that declares exactly which components comprise an application, which stacks to load, and how to deploy across environments (targets). This enables:

- **Explicit scope** - Process only declared stacks and components (10-20x faster than full repository processing)
- **Git-based tracking** - Track deployments using Git refs, annotated tags, and notes (what/when/who deployed)
- **Concurrent execution** - Isolated working directories per operation eliminate file conflicts
- **JIT vendoring** - Scope vendoring to deployment + target, enabling different component versions per environment
- **Promotion workflows** - Validated paths through environments (dev → staging → prod) with approval gates
- **Artifact generation** - Complete deployable artifacts including SBOMs, backends, providers
- **Application-level orchestration** - Build containers (Nixpacks), deploy Terraform, track dependencies—all in one workflow

Deployments complement stacks: stacks provide configuration composability (DRY, imports, inheritance), deployments provide orchestration and explicit scope. **[See comparing-stacks-deployments.md](./comparing-stacks-deployments.md) for the relationship.**

## User Experience

**Deploy applications with confidence.** Atmos Deployments orchestrates your Terraform components and container builds across environments—without external databases, complex state management, or deployment drift.

### The Experience

#### 1. Define Your Deployment Once

A deployment orchestrates multiple Atmos components (Terraform, containers, etc.) as a cohesive application:

```yaml
# deployments/api.yaml
deployment:
  name: api

  # Which stacks to load
  stacks:
    - "platform/vpc"
    - "platform/eks"
    - "ecr"
    - "ecs"

  components:
    nixpack:
      api:
        vars:
          source: "./services/api"  # Auto-builds Go/Node/Python/Rust
        metadata:
          depends_on: ["terraform/ecr/api"]

    terraform:
      ecr/api:
        vars:
          name: "api"
      ecs/service-api:
        vars:
          cluster_name: "app-ecs"

  targets:
    dev:
      context: { cpu: 256, memory: 512, replicas: 1 }
    staging:
      context: { cpu: 512, memory: 1024, replicas: 2 }
    prod:
      context: { cpu: 1024, memory: 2048, replicas: 4 }
```

You're still developing **components** (Terraform modules, services). Deployments just **orchestrate** them together.

#### 2. Develop Components Normally

Work on components like you always do:

```bash
# Develop a Terraform component
cd components/terraform/ecs-service
# ... make changes ...

# Test the component in a stack
atmos terraform plan ecs/service-api -s dev
atmos terraform apply ecs/service-api -s dev

# Develop your service code
cd services/api
# ... make changes ...
```

#### 3. Deploy Everything Together

When ready, deploy the whole application:

```bash
atmos deployment build api --target dev
```

```
Building api for target dev

  ✓ Detecting project type... Go
  ✓ Installing dependencies... go mod download
  ✓ Building application... go build -o main .
  ✓ Building container... sha256:abc123d
  ✓ Pushing to registry... registry.example.com/api:abc123d
  ✓ Generating SBOM... 47 dependencies
  ✓ Attaching to commit... abc123def

  Built: sha256:abc123d (2.3s)
```

```bash
atmos deployment rollout api --target dev
```

```
Rolling out api to dev

  Components (3):
    ✓ terraform/ecr/api (0.8s)
    ✓ terraform/ecs/taskdef-api (1.2s)
    ✓ terraform/ecs/service-api (2.1s)

  Deployment complete:
    → refs/atmos/deployments/api/dev/api → abc123def
    → atmos/deployments/api/dev/api/2025-01-23T15-30-00Z

  Deployed in 4.1s
```

#### 4. Promote Safely Through Environments

```bash
atmos deployment rollout api --target staging --promote-from dev
```

```
Promoting api from dev to staging

  ✓ Validating promotion path... dev → staging
  ✓ Loading dev deployment... abc123def

  Components (3):
    ✓ terraform/ecr/api (0.5s, no changes)
    ✓ terraform/ecs/taskdef-api (1.1s, config updated)
    ✓ terraform/ecs/service-api (1.8s, config updated)

  Deployment complete:
    → refs/atmos/deployments/api/staging/api → abc123def
    → atmos/deployments/api/staging/api/2025-01-23T15-45-00Z

  Promoted in 3.4s
```

```bash
atmos deployment rollout api --target prod --promote-from staging
```

```
Promoting api from staging to prod

  ✓ Validating promotion path... staging → prod
  ℹ Promotion requires approval (configure in CI/CD)
  ✓ Loading staging deployment... abc123def

  [CI/CD job pauses here for GitHub Environment approval]

  Components (3):
    ✓ terraform/ecr/api (0.4s, no changes)
    ✓ terraform/ecs/taskdef-api (1.3s, config updated)
    ✓ terraform/ecs/service-api (2.2s, config updated)

  Deployment complete:
    → refs/atmos/deployments/api/prod/api → abc123def
    → atmos/deployments/api/prod/api/2025-01-23T16-00-00Z

  Promoted in 4.1s
```

**Note:** Atmos validates the promotion path. Your CI/CD system handles approvals (GitHub Environment protection rules, GitLab approval rules, etc.).

#### 5. Know What's Running Everywhere

```bash
atmos deployment status api
```

```
api deployment status

  dev
    api      def456a  30m ago  ✓ healthy (2/2 tasks)

  staging
    api      abc123d  2h ago   ✓ healthy (4/4 tasks)

  prod
    api      abc123d  2h ago   ✓ healthy (8/8 tasks)
```

#### 6. Compare Environments Instantly

**High-level diff:**
```bash
atmos deployment diff api --from-target prod --to-target staging
```

```
api: prod vs staging

  Commits
    prod     abc123d  (deployed 2h ago)
    staging  def456a  (deployed 30m ago)
    ↓ 3 commits behind

  Components
    nixpack/api         abc123d → def456a  changed

  Configuration
    context.cpu         1024 → 512         ↓ decreased
    context.memory      2048 → 1024        ↓ decreased
    context.replicas    8 → 4              ↓ decreased
```

**Code-level diff:**
```bash
atmos deployment diff api --from-target prod --to-target staging --show-code
```

```
api: prod vs staging (code changes)

  services/api/main.go
    + Added connection pooling (line 45)
    + Improved error handling (line 78)
    + Updated dependency versions (go.mod)

  components/terraform/ecs-service/main.tf
    + Added health check configuration (line 23)
    + Increased check interval to 30s (line 28)

  3 commits, 47 files changed, +234/-156 lines
```

#### 7. View Complete Deployment History

```bash
atmos deployment history api --target prod
```

```
api → prod deployment history

  2025-01-23 16:00  abc123d  ci@example.com   ✓ success
    Promoted from staging, PR #482

  2025-01-20 09:15  def456a  ci@example.com   ✓ success
    Promoted from staging, PR #475

  2025-01-18 14:30  ghi789b  ops@example.com  ⟲ rollback
    Rollback due to incident #42

  Showing 3 deployments
```

#### 8. Rollback When Needed

```bash
atmos deployment rollout api --target prod --git-sha def456a
```

```
Rolling back api to def456a

  ℹ Rollback: abc123d → def456a

  Components (3):
    ✓ terraform/ecr/api (0.4s, no changes)
    ✓ terraform/ecs/taskdef-api (1.5s, rolled back)
    ✓ terraform/ecs/service-api (2.3s, rolled back)

  Deployment complete:
    → refs/atmos/deployments/api/prod/api → def456a
    → atmos/deployments/api/prod/api/2025-01-23T16-15-00Z
    → rollback_of: abc123d

  Rolled back in 4.2s
```

### What This Gives You

**Zero Infrastructure:**
- No deployment database to run or maintain
- No state files to manage or backup
- Git is your single source of truth

**Complete Visibility:**
- Instant answers to "what's deployed where?"
- Compare any two environments in seconds
- Full audit trail in Git history

**Safe Deployments:**
- Enforced promotion paths (dev → staging → prod)
- Approval workflows for production
- Automatic rollback capability

**Developer Friendly:**
- Develop components normally with `atmos terraform` commands
- Deployments orchestrate components you already know
- Auto-build containers from your code (Nixpacks)
- One command deploys entire application stack

**Production Ready:**
- Track all dependencies (automatic SBOM generation)
- No deployment drift (Git is the truth)
- Works offline (full history in local Git)

### Stacks and Deployments: Better Together

**Deployments and stacks are complementary—not competing concepts.**

Think of **stacks as a way to optimize your deployments.** As your deployments grow more complex, you can make them more composable by organizing configuration into reusable stacks and importing them.

**You have complete flexibility:**

- **Use stacks only** - Continue with the mental model you know. If parent stacks work well for your team, keep using them.

- **Use deployments only** - Define everything in the deployment manifest. Simple applications may not need the composability of stacks.

- **Use both** - Start with simple deployments. As complexity grows, refactor configuration into imported stacks. Get the benefits of both:
  - **Stacks** provide configuration composability (DRY, inheritance, imports)
  - **Deployments** provide orchestration and explicit scope (performance, tracking, concurrency)

**Example evolution:**

```yaml
# Simple deployment (no stacks needed)
deployment:
  name: simple-api
  components:
    terraform:
      ecs/service:
        vars: { cluster: "app" }
```

```yaml
# As complexity grows, introduce stacks for reusability
deployment:
  name: complex-api

  # Import common configuration
  stacks:
    - "platform/networking"
    - "platform/ecs-defaults"
    - "platform/monitoring"

  components:
    terraform:
      ecs/service:
        vars: { cluster: "app" }
```

**The key insight:** Deployments define **what to orchestrate**. Stacks provide **how to configure it**. Use the combination that makes sense for your team's complexity and scale.

See **[comparing-stacks-deployments.md](./comparing-stacks-deployments.md)** for a detailed comparison.

### Real-World Workflow

#### Component Development (Normal Workflow)

**Developer working on a component:**
```bash
# You're still developing components like normal
cd components/terraform/ecs-service
# ... make changes to Terraform module ...

# Test the component in a stack
atmos terraform plan ecs/service-api -s dev
atmos terraform apply ecs/service-api -s dev

# Or work on service code
cd services/api
# ... make changes to Go/Node/Python code ...
```

**Developer deploying entire application:**
```bash
# When ready to deploy the full stack
git checkout -b feature/faster-checkout

# Deploy all components together
atmos deployment rollout api --target dev
# → Builds nixpack containers
# → Runs terraform apply on all components (ecr/api, ecs/service-api, etc.)
# → Tracks deployment in Git
```

#### Promotion Through Environments

**CI/CD pipeline (after merge to main):**
```bash
# Merge triggers promotion to staging
atmos deployment rollout api --target staging --promote-from dev
# → Same container builds, new target context (cpu: 512, replicas: 2)
# → Terraform applies with staging config
# → Validates promotion path (dev → staging allowed)
```

**Release to production:**
```yaml
# .github/workflows/deploy.yml
jobs:
  deploy-prod:
    environment: production  # GitHub handles approval here
    steps:
      - name: Deploy to production
        run: atmos deployment rollout api --target prod --promote-from staging
        # → GitHub shows approval UI to ops team
        # → Alice clicks "Approve"
        # → Job continues, Atmos deploys
        # → Full audit trail in Git + GitHub
```

#### Troubleshooting

**SRE during incident:**
```bash
# What's running in production?
atmos deployment status api --target prod

# What dependencies does it have?
atmos deployment sbom api --target prod

# What changed recently?
atmos deployment diff api --from-target prod --to-target staging --show-code

# Rollback if needed
atmos deployment history api --target prod
atmos deployment rollout api --target prod --git-sha <previous-commit>
```

### The Result

**Deployments orchestrate components you already know.** You still develop Terraform components and services the same way. Deployments just give you:

- **Application-level operations** - Deploy entire stacks with one command
- **Deployment confidence** - Always know what's deployed where
- **Safe promotions** - Validated paths through environments
- **Complete history** - Full audit trail in Git
- **No new infrastructure** - Git is your deployment database

You're not learning a new way to work. You're getting better orchestration for what you already do.

---

## Document Organization

The deployments PRD is split into multiple focused documents:

### Core Concepts
1. **[problem-statement.md](./problem-statement.md)** - Enterprise-scale challenges and what we're solving
2. **[definitions.md](./definitions.md)** - Complete terminology reference (deployment, target, component, etc.)
3. **[comparing-stacks-deployments.md](./comparing-stacks-deployments.md)** - Understanding stacks vs deployments
4. **[overview.md](./overview.md)** - Goals, high-level architecture, design principles

### Configuration & Schema
5. **[configuration.md](./configuration.md)** - Complete deployment YAML schema with examples
6. **[adoption-guide.md](./adoption-guide.md)** - Migrating from parent stacks to deployments

### Deployment Tracking
7. **[git-tracking.md](./git-tracking.md)** - Git-based deployment tracking (refs, tags, notes) without external database
8. **[target-promotion.md](./target-promotion.md)** - Promotion paths, validation, approval workflows

### User Interface
9. **[cli-integration.md](./cli-integration.md)** - User-facing CLI commands wrapping Git operations
10. **[cli-commands.md](./cli-commands.md)** - Command reference, usage examples, flags

### Technical Implementation
11. **[vendoring.md](./vendoring.md)** - JIT vendoring strategy, vendor cache, content-addressable storage
12. **[nixpacks.md](./nixpacks.md)** - Container build system integration, nixpack component type
13. **[sbom.md](./sbom.md)** - SBOM generation, component registry pattern, CycloneDX integration
14. **[concurrent-execution.md](./concurrent-execution.md)** - Workspace isolation, DAG-based parallelism
15. **[cicd-integration.md](./cicd-integration.md)** - Git provider abstraction, GitHub Actions, GitLab CI

## Reading Order

If you're new to this PRD, we recommend reading in this order:

1. **[README.md](./README.md)** (this file) - Executive summary of user experience
2. **[problem-statement.md](./problem-statement.md)** - Why we need deployments (enterprise challenges)
3. **[comparing-stacks-deployments.md](./comparing-stacks-deployments.md)** - How deployments relate to stacks
4. **[definitions.md](./definitions.md)** - Core terminology (essential reference)
5. **[overview.md](./overview.md)** - High-level architecture and goals
6. **[cli-integration.md](./cli-integration.md)** - User-facing commands and workflows

Then explore adoption and technical details:
- **Getting started:** adoption-guide.md, configuration.md
- **Deployment tracking:** git-tracking.md, target-promotion.md
- **Component management:** vendoring.md, nixpacks.md, sbom.md
- **Execution:** concurrent-execution.md, cicd-integration.md

## Implementation Status

This PRD documents the **desired state** of Atmos Deployments. Implementation phases will be defined during implementation planning.

For implementation timeline and phasing, see **overview.md**.

## Contributing

When making changes to these PRDs:

1. Keep each document focused on a single concern
2. Use code examples liberally
3. Update the overview if adding new concepts
4. Test examples are valid YAML/Go code
5. Keep diagrams up to date with ASCII art

## Questions?

- **Architecture questions**: See overview.md and architecture diagrams
- **Schema questions**: See configuration.md for complete YAML reference
- **Implementation questions**: See specific technical documents (vendoring.md, sbom.md, etc.)
