# Atmos Deployments - Problem Statement

## What We're Solving

**Atmos Deployments solves a universal software delivery problem: how to deploy applications reliably, repeatedly, and confidently across environments.**

This problem affects everyone:
- **Newcomers to Atmos** face a steep learning curve understanding implicit deployment models
- **Growing teams** struggle with unclear deployment boundaries as complexity increases
- **Enterprise organizations** hit performance walls with thousands of component instances

While our focus has often been on enterprise scale (and we excel there), **Atmos is for everyone**. The challenges of defining "what is this application?" and "how do I deploy it safely?" are universal—whether you're deploying your first service or your thousandth.

## Problems with Current Stack-Based Approach

### Universal Problems (Everyone)

#### 1. Deployment Definition is Implicit and Confusing

**Problem:** "What is a deployment?" is determined by heuristic, not explicit definition.

**Current heuristic:**
> A deployment is the "level zero stack" — the top-most stack that imports all other stacks.

**For newcomers:**
```
stacks/
  dev.yaml        # Is this my deployment?
  _defaults.yaml  # Or this?
  prod.yaml       # Or this?
```

**Questions newcomers ask:**
- "Which file do I edit to deploy my application?"
- "What's the difference between a stack and a deployment?"
- "Where do I define my application?"
- "How do I know what will actually get deployed?"

**The "aha!" moment takes too long** because the concept is implicit, not explicit. You have to understand the entire stack inheritance model before you can deploy anything.

**What they want:**
```yaml
# deployments/my-app.yaml
deployment:
  name: my-app

  components:
    terraform:
      database:
        vars: { size: "small" }
      api:
        vars: { port: 8080 }
```

Clear, explicit, self-contained. **This is my application. These are its components. Deploy it.**

#### 2. No Built-in Deployment Tracking

**Problem:** "What's deployed where?" requires external tools or manual tracking.

**Current state:**
```bash
# No built-in answer to:
atmos terraform plan api -s prod  # Is this what's running?
atmos terraform apply api -s prod # Did this actually deploy?

# Teams resort to:
# - External deployment tracking systems
# - Manual spreadsheets
# - CI/CD platform logs (not portable)
# - Slack notifications (ephemeral)
```

**Real consequences:**
- Incident at 2am: "What version is running in prod?"
- Post-mortem: "When did this change deploy?"
- Audit: "Who deployed this and why?"
- Rollback: "What was the previous working version?"

**What everyone needs:**
```bash
atmos deployment status my-app --target prod
# → Exactly what's running, when it was deployed, who deployed it

atmos deployment history my-app --target prod
# → Complete deployment history from Git
```

#### 3. No Safe Promotion Workflows

**Problem:** Nothing prevents deploying untested code directly to production.

**Current state:**
```bash
# Equally valid (and equally scary):
atmos terraform apply api -s dev
atmos terraform apply api -s prod  # No validation this was tested in dev

# No enforcement of:
# - Deploy to dev first
# - Test in staging
# - Require approval for prod
```

**What accidents happen:**
- Developer fat-fingers `prod` instead of `dev`
- Untested changes go straight to production
- No validation that staging was successful
- No approval workflow for critical changes

**What everyone needs:**
```yaml
targets:
  dev: {}    # Anyone can deploy
  staging:
    promotion:
      from: dev  # Must promote from dev
  prod:
    promotion:
      from: staging  # Must promote from staging
      requires_approval: true  # Explicit approval required
```

#### 4. CI/CD Logic Lives in CI Platform

**Problem:** Deployment workflows are implemented in CI/CD platform configuration, making them impossible to test locally and difficult to debug.

**Current approach:**
```yaml
# .github/workflows/deploy.yml - Platform-specific, can't run locally
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - run: |
          # Complex build logic here
          # How do you test this locally?
          # Answer: You can't. Push and pray.

  deploy-prod:
    needs: [build, test]
    environment: production
    steps:
      - run: |
          # More complex logic
          # Debug by: git commit -m "try again"
```

**The painful reality:**
```bash
# Developer workflow:
git commit -m "fix CI script"
git push
# Wait 5 minutes...
# Check GitHub Actions...
# Build failed on line 47

git commit -m "try again"
git push
# Wait 5 minutes...
# Still failing

git commit -m "please work this time"
git push
# Wait 5 minutes...
# Success! (after 15 minutes and 3 commits)
```

**What this causes:**
- **Slow iteration** - Can't test changes locally, must push to CI
- **Brittle pipelines** - Logic spread across YAML, bash scripts, GitHub Actions
- **Platform lock-in** - GitHub Actions syntax ≠ GitLab CI ≠ Jenkins
- **Poor debugging** - No logs until job runs in CI
- **"Works in CI" problems** - Can't reproduce CI environment locally

**What Earthly and Dagger teach us:**
- **Earthly:** "No way to run CI locally with traditional tools means `git commit -m 'try again'` over and over"
- **Dagger:** "Run locally or in CI with the exact same code. Test locally, identify errors early, fewer surprises when pushing."

**What everyone needs:**
```bash
# Test the full deployment workflow locally
atmos deployment build api --target dev
atmos deployment test api --target dev
atmos deployment rollout api --target dev

# Same commands run in CI
# If it works locally, it works in CI
# No surprises, no "try again" commits
```

**CI should validate what you've already tested locally:**
```yaml
# .github/workflows/deploy.yml - Simple validation layer
jobs:
  deploy-prod:
    environment: production
    steps:
      # CI just runs the same command you tested locally
      - run: atmos deployment rollout api --target prod --promote-from staging
```

### Scale Problems (Enterprise)

These problems emerge at enterprise scale—thousands of component instances across hundreds of environments. Not everyone will encounter these challenges, but they're critical for large organizations using Atmos. **It's a good problem to have.**

#### 5. Performance Degrades at Enterprise Scale

**Problem:** As infrastructure grows, Atmos must process the entire repository to determine what to deploy.

**Real-world impact:**
- Enterprise financial services institutions: 10,000+ component instances
- Healthcare systems: 1,000+ environments across regions
- Global e-commerce platforms: Hundreds of microservices × dozens of regions

**What happens:**
```bash
# Current approach: Process everything
atmos terraform plan ecs/payment-api -s prod

# Atmos must:
# 1. Scan all stack files (1000+ files)
# 2. Process all imports and inheritance chains
# 3. Evaluate all templates (slow)
# 4. Load all stack configurations (even unrelated ones)
# 5. Finally: Plan the one component you asked for
```

**The bottleneck:**
- Template evaluation is expensive (Go templates + Gomplate functions)
- Processing 10,000 components to deploy 1 component doesn't scale
- Every operation pays the full cost of repository scanning

**We've optimized, but physics wins:**
- Caching helps, but cold starts are still slow
- Parallelization helps, but you still process everything
- As infrastructure grows, the problem compounds

### 2. Deployment Definition is Implicit and Confusing

**Problem:** "What is a deployment?" is determined by heuristic, not explicit definition.

**Current heuristic:**
> A deployment is the "level zero stack" — the top-most stack that imports all other stacks.

**Why this worked initially:**
- Small repos: 10-20 stacks, clear hierarchy
- Simple imports: One top-level stack per environment
- Easy to visualize: `prod.yaml` imports everything for production

**Why this breaks at scale:**
```
stacks/
  orgs/
    acme/
      _defaults.yaml              # Is this level zero?
      platform/
        _defaults.yaml            # Or is this?
        networking/
          _defaults.yaml          # Or this?
          prod/
            us-east-1.yaml        # Or THIS?
```

**Real consequences:**
- New team members: "Which file do I edit?"
- CI/CD: "What do we actually deploy?"
- Documentation: "How do I explain this?"

**The fundamental issue:**
- Implicit definitions scale poorly
- Heuristics become ambiguous as complexity grows
- No single source of truth for "what is this application?"

#### 6. Version Pinning Requires Folder Structure

**Problem:** Pinning component versions per environment requires duplicating folder structure.

**Current approach:**
```
components/terraform/
  rds/              # Production version (v1.2.5 - stable)
  rds-dev/          # Dev version (v1.3.0 - bleeding edge)
  rds-staging/      # Staging version (v1.3.0-beta.1)
```

**Why this is painful:**
```bash
# Same component, three copies
# - Different versions
# - Same bugs need three fixes
# - Drift between copies
# - Folder explosion
```

**What teams want:**
```yaml
# Conceptually: One component definition, multiple versions per environment
# (This is pseudocode showing the desired outcome, not actual syntax)

vendor:
  components:
    - source: "github.com/cloudposse/terraform-aws-components//modules/rds"
      version: "1.3.0"
      targets: ["dev"]          # Dev gets latest

    - source: "github.com/cloudposse/terraform-aws-components//modules/rds"
      version: "1.3.0-beta.1"
      targets: ["staging"]      # Staging validates RC

    - source: "github.com/cloudposse/terraform-aws-components//modules/rds"
      version: "1.2.5"
      targets: ["prod"]         # Prod gets stable
```

**The deeper issue:**
- Atmos vendors to folders at repo level
- Folders are global, not scoped to environments
- Just-in-time vendoring could solve this, but requires deployment scope

#### 7. No Concurrent Deployments

**Problem:** Atmos generates files in component folders, preventing concurrent operations.

**What happens:**
```bash
# Terminal 1
atmos terraform plan ecs/api -s prod
# → Generates: components/terraform/ecs-api/backend.tf
# → Generates: components/terraform/ecs-api/providers.tf

# Terminal 2 (simultaneously)
atmos terraform plan ecs/api -s staging
# → CONFLICT: Overwrites backend.tf with staging config
# → CONFLICT: Overwrites providers.tf with staging config
```

**Real-world scenarios that fail:**
- CI/CD: Parallel plan jobs for multiple environments
- Local dev: Testing changes in dev while prod deploy runs
- Multi-region: Deploying to us-east-1 and eu-west-1 simultaneously

**Why this matters at scale:**
- Large infrastructures have hundreds of components
- Sequential deployment takes hours
- No parallelization = infrastructure deployment bottleneck
- Teams wait for single-threaded operations to complete

**Current state:**
```bash
# Deploying 100 components sequentially
Component 1:  2 minutes
Component 2:  2 minutes
...
Component 100: 2 minutes
Total: 200 minutes (3.3 hours)

# With 10-way parallelism (if we had isolation)
10 batches × 2 minutes = 20 minutes
```

**Why this happens:**
- Generated files stored in component directory
- No isolation between operations
- No working directory per execution

**What we need:**
- Copy-on-write component directories
- Isolated working directories per operation
- Concurrent plans and applies
- Parallelism that actually works at scale

#### 8. No Deployable Artifacts

**Problem:** Generated files (backend, providers) are not stored as artifacts.

**What's missing:**
```
# Current state: Only source code versioned
components/terraform/ecs-api/
  main.tf           ✓ Versioned
  variables.tf      ✓ Versioned
  backend.tf        ✗ Generated, ephemeral
  providers.tf      ✗ Generated, ephemeral
  .terraform.lock.hcl  ✗ Generated, ephemeral
```

**Why this matters in regulated industries:**
- **Compliance:** "Show me exactly what was deployed on 2025-01-15"
- **Auditing:** "What provider versions were used?"
- **Reproducibility:** "Re-deploy this exact configuration"

**Current workaround:**
- Store generated files in git (messy)
- Regenerate on demand (non-deterministic)
- Hope deterministic generation works (it mostly does)

**Desired state:**
```
releases/payment-service/prod/release-abc123/
  backend.tf              # Exact backend config used
  providers.tf            # Exact provider config used
  .terraform.lock.hcl     # Exact provider versions used
  metadata.yaml           # Deployment metadata
```

#### 9. Ambiguous Concept of "Stages"

**Problem:** Two different meanings of "stages" cause confusion.

**Stage as Environment (SDLC):**
```
dev → staging → prod
```

**Stage as Phase (Deployment Pipeline):**
```
build → test → deploy → validate
```

**Real confusion:**
```bash
# What does this mean?
atmos deployment stage payment-service --target prod

# Is "stage" a verb (do something) or noun (environment)?
# Is --target an environment or a phase?
```

**Current overload in CI/CD:**
```yaml
# .github/workflows/deploy.yml
jobs:
  build:          # Phase
    runs-on: ubuntu-latest
  test:           # Phase
    needs: build
  deploy-staging: # Environment
    needs: test
  deploy-prod:    # Environment
    needs: deploy-staging
```

**The problem:**
- Logic lives in CI/CD platform
- Hard to test locally
- Brittle, platform-specific
- Can't reproduce "exactly what CI did"

**What we need:**
```bash
# Clear separation
atmos deployment build api --target dev     # Build for dev environment
atmos deployment test api --target dev      # Test dev build
atmos deployment rollout api --target dev   # Deploy to dev environment

# Phases are verbs (build, test, rollout)
# Targets are nouns (dev, staging, prod)
```

#### 10. Full Repository Access Required

**Problem:** Atmos must access ALL stacks to find the top-level parent.

**Current requirement:**
```bash
# To deploy one component in prod...
atmos terraform apply ecs/api -s prod

# Atmos needs read access to:
# - All dev stacks (to find inheritance chain)
# - All staging stacks (to find inheritance chain)
# - All prod stacks (to process the one you want)
# - All platform stacks (imported by everything)
```

**Permissions nightmare:**
```yaml
# CI service account needs:
permissions:
  dev: read       # Just to figure out inheritance
  staging: read   # Just to figure out inheritance
  prod: write     # What we actually want to deploy

# Security team: "Why does prod deploy need dev access?"
# Answer: "To compute the configuration."
# Security team: "That makes no sense."
# Answer: "We know. We're working on it."
```

**Secrets sprawl:**
```bash
# Deploying to prod loads secrets from:
# - dev (because inheritance)
# - staging (because inheritance)
# - prod (what we actually need)

# Result: Prod pipeline has dev secrets
# Security audit: CRITICAL FINDING
```

**Workaround (contributes to problem #8):**
```yaml
# Disable template processing on stacks we don't need
# stacks/dev/_defaults.yaml
metadata:
  settings:
    templates:
      enabled: false  # Don't process dev when deploying prod

# But this requires:
# - Human decision: "Which stacks should have this disabled?"
# - Manual maintenance: "Did I remember to disable this?"
# - Breaks when structure changes: "Why is prod broken now?"
```

**What we need:**
```yaml
# deployments/payment-service.yaml
stacks:
  - "platform/vpc"    # Only load what we use
  - "platform/eks"
  - "ecr"
  - "ecs"

# Don't load:
# - Unrelated services
# - Other environments
# - Other regions

# Result:
# - No access to dev/staging stacks when deploying prod
# - No dev/staging secrets loaded
# - No template processing for unused stacks
# - Automatic, context-based optimization
```

#### 11. Template Processing is Slow

**Problem:** Atmos templates are powerful but expensive.

**What's slow:**
```yaml
# Every stack file can use templates
cpu: '{{ atmos.Component "ecs-api" "dev" | jq ".vars.cpu" }}'

# This requires:
# 1. Process other component
# 2. Marshal to JSON
# 3. Execute jq
# 4. Parse result
# 5. Continue processing
```

**Compounding effect:**
- 1,000 stacks × 10 template functions each = 10,000 evaluations
- Template functions can call other components (recursive)
- Every deployment pays this cost

**Real-world timing:**
```bash
# Small repo (50 stacks)
atmos terraform plan ecs/api -s dev
# → 2 seconds (acceptable)

# Enterprise repo (1,000 stacks)
atmos terraform plan ecs/api -s dev
# → 45 seconds (painful)

# Global platform (10,000 component instances)
atmos terraform plan ecs/api -s dev
# → 8 minutes (unacceptable)
```

**Workarounds teams have implemented:**
```yaml
# Disable template processing on certain stacks
metadata:
  settings:
    templates:
      enabled: false

# Disable function execution
metadata:
  settings:
    functions:
      enabled: false
```

**The problem with workarounds:**
- ❌ Requires human decision-making ("Should I disable templates here?")
- ❌ Easy to forget and enable when not needed
- ❌ Easy to forget and disable when actually needed
- ❌ No automatic determination based on usage context
- ❌ Brittle - breaks when stack structure changes

**What we need:**
- ✅ Automatic determination based on context
- ✅ Only process templates when deployment uses them
- ✅ Only execute functions when deployment references them
- ✅ Scope-based optimization (deployments declare what they use)

**The optimization path:**
- Reduce scope: Only process what's needed (deployments)
- Cache more: Cache template results (we do this)
- Parallelize: Process independent stacks concurrently (we do this)
- But ultimately: Less processing is the only real solution

## What We've Learned

After years of Atmos at enterprise scale:

1. **Implicit is fragile** - Explicit definitions scale better
2. **Global scope doesn't scale** - Scoped operations are faster
3. **Heuristics break** - Formal types are clearer
4. **Generated files need homes** - Artifacts should be first-class
5. **Folder structure is limiting** - JIT vendoring enables flexibility
6. **Templates are powerful but expensive** - Reduce scope to reduce cost
7. **Permissions should be minimal** - Load only what you need
8. **Isolation enables concurrency** - Working directories per operation

## Solution: Formalize Deployments as a Type

Instead of:
> "A deployment is whatever heuristic says is level zero"

We define:
> "A deployment is an explicit YAML file that declares its scope"

```yaml
# deployments/payment-service.yaml
deployment:
  name: payment-service

  # Explicit: Which stacks to load
  stacks:
    - "platform/vpc"
    - "platform/eks"
    - "ecr"
    - "ecs"

  # Explicit: Which components comprise this app
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
      ecs/service-api:
        vars:
          cluster_name: "app-ecs"

  # Explicit: Which environments exist
  targets:
    dev:
      context: { cpu: 256, memory: 512 }
    staging:
      context: { cpu: 512, memory: 1024 }
    prod:
      context: { cpu: 2048, memory: 4096 }
```

**This solves:**
- ✅ Scope defined explicitly (only load what's declared)
- ✅ Fast processing (10-20x faster, only process deployment scope)
- ✅ JIT vendoring possible (scoped to deployment + target)
- ✅ Concurrent operations (isolated working directories)
- ✅ Clear deployment definition (newcomers understand immediately)
- ✅ Minimal permissions (only access declared stacks)
- ✅ Deployable artifacts (release records per target)
- ✅ Clear terminology (targets = environments, verbs = phases)
- ✅ Deployment tracking (what was deployed, when, by whom)
- ✅ Artifact association (SBOMs, test results tied to commits)
- ✅ Complete audit trail (all history in Git)
- ✅ No new backends required (Git is the database)

## Non-Goals

**We are NOT:**
- Replacing stacks (stacks remain the foundation)
- Deprecating current workflows (`atmos terraform` commands stay)
- Forcing everyone to use deployments (opt-in for those who need it)
- Creating a new infrastructure language (still Terraform/Helm/etc.)

**We ARE:**
- Adding deployment orchestration layer above stacks
- Enabling enterprise-scale performance
- Formalizing implicit concepts
- Making Atmos easier to explain and use

## Success Criteria

**Performance:**
- 10-20x faster than full repository processing
- Sub-second response for deployment status queries
- Concurrent operations work reliably

**Usability:**
- New users understand deployments in < 5 minutes
- Clear mental model: "Deployments orchestrate components"
- Obvious how to deploy applications

**Scalability:**
- 10,000+ component instances: No problem
- 1,000+ environments: Fast queries
- 100+ concurrent CI jobs: No conflicts

**Adoption:**
- Existing users can migrate incrementally
- New users start with deployments
- Enterprise customers get tools they need

## See Also

- **[definitions.md](./definitions.md)** - Core concepts and terminology
- **[overview.md](./overview.md)** - High-level architecture
- **[git-tracking.md](./git-tracking.md)** - How we avoid external databases
