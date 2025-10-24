# Adopting Atmos Deployments

This guide shows you how to adopt Atmos Deployments in an existing repository with established stacks. The migration is incremental, non-breaking, and can be done one application at a time.

## Philosophy

**Deployments don't replace your stacks.** They reference them.

You create deployment manifests that:
- Declare which stacks to load
- Group components into applications
- Define deployment targets (environments)

Your existing stacks continue to work unchanged.

## Migration Strategy

### Option 1: Incremental Adoption (Recommended)

Migrate one application at a time while keeping existing workflows working.

**Benefits:**
- Zero risk (nothing breaks)
- Validate benefits before full migration
- Team learns gradually
- Easy rollback (just don't use deployment commands)

### Option 2: Parallel Usage

Use deployments for new applications, stacks for existing ones.

**Benefits:**
- Immediate value for new work
- No migration effort
- Existing apps stay stable

### Option 3: Full Migration

Convert all parent stacks to deployment manifests.

**Benefits:**
- Maximum consistency
- Full performance gains
- Clean repository structure

**When:** After validating deployments work for your use cases.

## Step-by-Step: Incremental Adoption

### Step 1: Identify Your Parent Stacks

Parent stacks are your current "deployments" - the top-level stacks that import everything else.

**Find them:**
```bash
# List all stacks
find stacks -name "*.yaml" -type f

# Identify parent stacks (stacks with many imports, few imports of them)
# These are typically in: stacks/prod.yaml, stacks/dev/main.yaml, etc.
```

**Example parent stack:**
```yaml
# stacks/prod/us-east-1.yaml
import:
  - orgs/acme/platform/networking/_defaults
  - orgs/acme/platform/ecs/_defaults
  - orgs/acme/platform/ecr/_defaults
  - orgs/acme/services/payment-api/_defaults

components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.0.0.0/16"

    ecs/payment-api:
      vars:
        cluster_name: "app-cluster"
        cpu: 2048
        memory: 4096
```

### Step 2: Create Your First Deployment Manifest

**Choose one application to start with.** Pick something:
- ✅ Actively developed (you'll see immediate benefits)
- ✅ Well-understood (easier to validate)
- ✅ Not mission-critical (lower risk for first migration)

**Convert parent stack to deployment:**

**Before (parent stack):**
```yaml
# stacks/prod/payment-api.yaml
import:
  - orgs/acme/platform/networking/_defaults
  - orgs/acme/platform/ecs/_defaults
  - orgs/acme/platform/ecr/_defaults

components:
  terraform:
    ecr/payment-api:
      vars:
        name: "payment-api"

    ecs/payment-api-taskdef:
      vars:
        cpu: 2048
        memory: 4096

    ecs/payment-api-service:
      vars:
        cluster_name: "app-cluster"
```

**After (deployment manifest):**
```yaml
# deployments/payment-api.yaml
deployment:
  name: payment-api

  # Reference the stacks your parent stack imported
  stacks:
    - "orgs/acme/platform/networking/_defaults"
    - "orgs/acme/platform/ecs/_defaults"
    - "orgs/acme/platform/ecr/_defaults"

  # List the components that comprise this app
  components:
    terraform:
      ecr/payment-api:
        vars:
          name: "payment-api"

      ecs/payment-api-taskdef:
        vars:
          family: "payment-api"
          cpu: 512  # Base config, overridden by targets
          memory: 1024

      ecs/payment-api-service:
        vars:
          cluster_name: "app-cluster"

  # Define targets (environments) explicitly
  targets:
    dev:
      labels:
        environment: dev
      context:
        cpu: 256
        memory: 512
        replicas: 1

    staging:
      labels:
        environment: staging
      context:
        cpu: 512
        memory: 1024
        replicas: 2

    prod:
      labels:
        environment: prod
      context:
        cpu: 2048
        memory: 4096
        replicas: 8
```

**Key changes:**
1. `stacks:` field explicitly lists what to load
2. `components:` section defines the application boundary
3. `targets:` section replaces multiple stack files per environment
4. Environment-specific config moved to `targets[].context`

### Step 3: Test the Deployment

**Validate it works:**
```bash
# Validate deployment config
atmos deployment validate payment-api
```

```
Validating payment-api

  ✓ Deployment configuration is valid
  ✓ All stacks exist
  ✓ All components exist
  ✓ No promotion path cycles
  ✓ Context variables valid

  Summary:
    Deployment: payment-api
    Stacks: 3
    Components: 3
    Targets: 3 (dev, staging, prod)
```

**Compare with stack-based approach:**
```bash
# Old way
atmos terraform plan ecr/payment-api -s prod

# New way (should produce identical plan)
atmos deployment build payment-api --target prod --dry-run
```

**If plans differ:** Your deployment manifest doesn't match your stacks. Adjust until they're identical.

### Step 4: Deploy Using Deployment Commands

**Start with dev:**
```bash
atmos deployment rollout payment-api --target dev
```

```
Rolling out payment-api to dev

  Components (3):
    ✓ terraform/ecr/payment-api (0.5s, no changes)
    ✓ terraform/ecs/payment-api-taskdef (1.2s, applied)
    ✓ terraform/ecs/payment-api-service (2.1s, applied)

  Deployment complete:
    → refs/atmos/deployments/payment-api/dev/... → abc123def
    → atmos/deployments/payment-api/dev/.../2025-01-23T15-30-00Z

  Deployed in 3.8s
```

**Verify it worked:**
```bash
atmos deployment status payment-api --target dev
```

**Check deployment history:**
```bash
atmos deployment history payment-api --target dev
```

### Step 5: Update CI/CD

**Before (stack-based):**
```yaml
# .github/workflows/deploy.yml
jobs:
  deploy:
    steps:
      - name: Deploy to prod
        run: |
          atmos terraform apply ecr/payment-api -s prod
          atmos terraform apply ecs/payment-api-taskdef -s prod
          atmos terraform apply ecs/payment-api-service -s prod
```

**After (deployment-based):**
```yaml
# .github/workflows/deploy.yml
jobs:
  deploy-dev:
    runs-on: ubuntu-latest
    steps:
      - name: Deploy to dev
        run: atmos deployment rollout payment-api --target dev

  deploy-staging:
    needs: deploy-dev
    runs-on: ubuntu-latest
    steps:
      - name: Promote to staging
        run: atmos deployment rollout payment-api --target staging --promote-from dev

  deploy-prod:
    needs: deploy-staging
    runs-on: ubuntu-latest
    environment: production  # GitHub approval
    steps:
      - name: Promote to prod
        run: atmos deployment rollout payment-api --target prod --promote-from staging
```

**Benefits:**
- ✅ Single command per environment
- ✅ Automatic promotion validation
- ✅ Built-in deployment tracking
- ✅ Clearer pipeline stages

### Step 6: Repeat for Other Applications

Now that you've validated the approach:

1. Pick next application
2. Create deployment manifest
3. Test and validate
4. Update CI/CD
5. Repeat

**Prioritization:**
- Start with actively developed apps (immediate value)
- Then mission-critical apps (highest benefit)
- Then legacy apps (lowest priority)

### Step 7: Clean Up (Optional)

Once all applications are deployments, you can optionally:

**Remove redundant parent stacks:**
```bash
# If stacks/prod/payment-api.yaml is fully replaced by deployments/payment-api.yaml
# You can remove the stack file
rm stacks/prod/payment-api.yaml
```

**Keep imported stacks:**
```
stacks/
  orgs/acme/
    platform/
      networking/_defaults.yaml  # KEEP - referenced by deployments
      ecs/_defaults.yaml         # KEEP - referenced by deployments
```

**Update documentation:**
```
# README.md
Old: "Deploy using: atmos terraform apply <component> -s <stack>"
New: "Deploy using: atmos deployment rollout <deployment> --target <target>"
```

## Common Patterns

### Pattern 1: Simple Service

**Stack structure:**
```
stacks/
  dev.yaml
  staging.yaml
  prod.yaml
```

**Deployment manifest:**
```yaml
# deployments/api.yaml
deployment:
  name: api
  stacks:
    - "platform/base"
  components:
    terraform:
      api: {}
  targets:
    dev: { context: { cpu: 256 } }
    staging: { context: { cpu: 512 } }
    prod: { context: { cpu: 2048 } }
```

### Pattern 2: Multi-Service Application

**Stack structure:**
```
stacks/
  orgs/acme/
    platform/
      networking/_defaults.yaml
      ecs/_defaults.yaml
    services/
      api/_defaults.yaml
      worker/_defaults.yaml
      frontend/_defaults.yaml
```

**Deployment manifest:**
```yaml
# deployments/payment-platform.yaml
deployment:
  name: payment-platform

  stacks:
    - "orgs/acme/platform/networking/_defaults"
    - "orgs/acme/platform/ecs/_defaults"
    - "orgs/acme/services/api/_defaults"
    - "orgs/acme/services/worker/_defaults"

  components:
    terraform:
      ecr/api: {}
      ecr/worker: {}
      ecs/api: {}
      ecs/worker: {}
      alb: {}

  targets:
    dev: { context: { api_replicas: 1, worker_replicas: 1 } }
    staging: { context: { api_replicas: 2, worker_replicas: 2 } }
    prod: { context: { api_replicas: 8, worker_replicas: 4 } }
```

### Pattern 3: Regional Deployment

**Stack structure:**
```
stacks/
  regions/
    us-east-1/_defaults.yaml
    us-west-2/_defaults.yaml
    eu-west-1/_defaults.yaml
```

**Deployment manifest (per region):**
```yaml
# deployments/api-us-east-1.yaml
deployment:
  name: api-us-east-1

  stacks:
    - "regions/us-east-1/_defaults"
    - "platform/ecs"

  components:
    terraform:
      api: {}

  targets:
    dev: { context: { region: "us-east-1" } }
    prod: { context: { region: "us-east-1" } }
```

**Or: Single deployment with region context:**
```yaml
# deployments/api.yaml
deployment:
  name: api

  components:
    terraform:
      api: {}

  targets:
    dev-us-east-1:
      context: { region: "us-east-1", cpu: 256 }
    dev-us-west-2:
      context: { region: "us-west-2", cpu: 256 }
    prod-us-east-1:
      context: { region: "us-east-1", cpu: 2048 }
    prod-us-west-2:
      context: { region: "us-west-2", cpu: 2048 }
```

## Handling Edge Cases

### Edge Case 1: Deeply Nested Imports

**Stack with many layers:**
```yaml
# stacks/prod/us-east-1/payment-api.yaml
import:
  - ../../_defaults
  - ../../platform/_defaults
  - ../../platform/networking/_defaults
  - ../../platform/ecs/_defaults
  - ../payment/_defaults
```

**Deployment approach:**
```yaml
# deployments/payment-api.yaml
stacks:
  # List all imported stacks explicitly
  - "_defaults"
  - "platform/_defaults"
  - "platform/networking/_defaults"
  - "platform/ecs/_defaults"
  - "payment/_defaults"

# Benefit: Clear dependency tree, faster processing
```

### Edge Case 2: Stack-Specific Overrides

**Stack with environment-specific config:**
```yaml
# stacks/prod.yaml
components:
  terraform:
    api:
      vars:
        cpu: 2048
        enable_autoscaling: true
        log_retention: 90
```

**Deployment equivalent:**
```yaml
# deployments/api.yaml
targets:
  prod:
    context:
      cpu: 2048
      enable_autoscaling: true
      log_retention: 90
```

### Edge Case 3: Multiple Applications in One Stack

**Monolithic stack:**
```yaml
# stacks/prod.yaml
components:
  terraform:
    api: {}
    worker: {}
    frontend: {}
    database: {}
    cache: {}
```

**Split into deployments:**
```yaml
# deployments/api.yaml
components:
  terraform:
    api: {}
    database: {}  # API depends on database

# deployments/worker.yaml
components:
  terraform:
    worker: {}
    cache: {}  # Worker depends on cache

# deployments/frontend.yaml
components:
  terraform:
    frontend: {}
```

**Benefit:** Deploy API without deploying worker (faster, safer).

## Validation Checklist

Before considering migration complete:

- [ ] Deployment manifest validates: `atmos deployment validate <name>`
- [ ] Plans match stack-based approach (no unexpected changes)
- [ ] Deployment commands work: `build`, `rollout`, `status`
- [ ] CI/CD updated and tested
- [ ] Team trained on new commands
- [ ] Documentation updated
- [ ] Rollback plan tested (deployment commands are opt-in, can revert to stacks)

## Rollback Plan

If deployments don't work for you:

**Deployments are opt-in.** Nothing breaks if you stop using them.

```bash
# Stop using deployment commands
# atmos deployment rollout api --target prod  # STOP DOING THIS

# Resume using stack commands
atmos terraform apply ecs/api -s prod  # WORKS FINE
```

**Delete deployment manifests:**
```bash
rm deployments/*.yaml
```

**Your stacks still work unchanged.**

## Benefits Timeline

**Week 1-2: First deployment migrated**
- ✅ Deployment tracking working
- ✅ Faster deployment commands
- ✅ Team learning new workflow

**Week 3-4: Core applications migrated**
- ✅ CI/CD simplified
- ✅ Promotion paths validated
- ✅ Performance improvements visible

**Month 2: Most applications migrated**
- ✅ Deployment tracking across all apps
- ✅ Concurrent deployments working
- ✅ JIT vendoring providing value

**Month 3+: Full adoption**
- ✅ 10-20x faster than stack-only approach
- ✅ Complete audit trail
- ✅ Enterprise-scale infrastructure manageable

## Getting Help

**Questions?**
- Review **[comparing-stacks-deployments.md](./comparing-stacks-deployments.md)** for conceptual clarity
- Check **[definitions.md](./definitions.md)** for terminology
- See **[configuration.md](./configuration.md)** for complete schema

**Issues?**
- Start small (one deployment)
- Validate thoroughly before scaling
- Keep stack-based workflows as fallback
- Migration is incremental, not all-or-nothing

## See Also

- **[comparing-stacks-deployments.md](./comparing-stacks-deployments.md)** - Understanding the relationship
- **[problem-statement.md](./problem-statement.md)** - Why deployments exist
- **[configuration.md](./configuration.md)** - Complete deployment schema
- **[cli-integration.md](./cli-integration.md)** - Command reference
