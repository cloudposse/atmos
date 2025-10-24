# Atmos Deployments - Target Promotion

This document describes how Atmos enforces promotion paths between deployment targets, ensuring controlled progression through environments (e.g., dev → staging → prod).

## Overview

Target promotion defines the allowed progression paths for deployments across environments. This ensures:
- **Controlled rollout** - Changes must flow through specific environments
- **Quality gates** - Each environment validates before promotion
- **Approval workflows** - Production deployments require explicit approval
- **Audit trail** - Promotion history tracked in Git

**Key principle:** Promotions are validated at deploy-time using configuration-driven rules.

## Promotion Configuration

Define promotion paths in the deployment configuration:

```yaml
# deployments/payment-service.yaml
deployment:
  name: payment-service

  targets:
    dev:
      # No promotion config = entry point
      labels:
        environment: dev
      context:
        cpu: 256
        memory: 512

    staging:
      promotion:
        from: dev  # Can only promote from dev
      labels:
        environment: staging
      context:
        cpu: 512
        memory: 1024

    prod:
      promotion:
        from: staging  # Must promote from staging
        requires_approval: true  # Atmos Pro approval required
      labels:
        environment: prod
      context:
        cpu: 2048
        memory: 4096
```

**Promotion graph:**
```
dev → staging → prod
```

## Complex Promotion Scenarios

### Multiple Source Targets

Allow promotion from multiple sources:

```yaml
targets:
  dev:
    # Entry point

  qa:
    promotion:
      from: dev

  staging:
    promotion:
      allowed_from: [dev, qa]  # Can promote from either dev or qa

  prod:
    promotion:
      from: staging
      requires_approval: true
```

**Promotion graph:**
```
     ┌─→ qa ─┐
dev ─┤       ├─→ staging → prod
     └───────┘
```

### Per-Deployment Targets

Each deployment can have different targets and promotion paths:

```yaml
# deployments/payment-service.yaml
deployment:
  name: payment-service
  targets:
    dev: {}
    staging:
      promotion:
        from: dev
    prod:
      promotion:
        from: staging
        requires_approval: true

# deployments/experimental-feature.yaml
deployment:
  name: experimental-feature
  targets:
    dev: {}
    canary:
      promotion:
        from: dev
    prod:
      promotion:
        from: canary
        requires_approval: true
```

**Rationale:** Different applications have different deployment strategies. Payment service goes through standard environments, while experimental features use canary deployments.

### No Promotion Restrictions

Targets without promotion config accept deployments from anywhere:

```yaml
targets:
  dev:
    # No restrictions - can deploy any commit

  debug:
    # No restrictions - can deploy any commit for debugging
```

## Promotion Enforcement

### CLI Validation

The CLI validates promotion paths before deploying:

```bash
# Valid: following promotion path
atmos deployment rollout payment-service --target staging --promote-from dev
# ✅ Promotes dev deployment to staging

# Invalid: skipping staging
atmos deployment rollout payment-service --target prod --promote-from dev
# ❌ Error: Cannot promote directly from dev to prod. Must promote from staging.

# Valid: with approval
atmos deployment rollout payment-service --target prod --promote-from staging
# → Requests Atmos Pro approval
# → Waits for approval
# → Deploys to prod
```

### Validation Logic

```go
// pkg/deployment/promotion.go
type PromotionConfig struct {
    From             string   `yaml:"from"`              // Single source
    AllowedFrom      []string `yaml:"allowed_from"`      // Multiple sources
    RequiresApproval bool     `yaml:"requires_approval"`
}

func ValidatePromotion(from, to string, deployment *Deployment) error {
    toTarget := deployment.Targets[to]

    if toTarget.Promotion == nil {
        return nil  // No restrictions
    }

    // Check single source
    if toTarget.Promotion.From != "" && toTarget.Promotion.From != from {
        return fmt.Errorf("cannot promote from %s to %s (must promote from %s)",
            from, to, toTarget.Promotion.From)
    }

    // Check multiple sources
    if len(toTarget.Promotion.AllowedFrom) > 0 {
        allowed := false
        for _, allowedFrom := range toTarget.Promotion.AllowedFrom {
            if allowedFrom == from {
                allowed = true
                break
            }
        }
        if !allowed {
            return fmt.Errorf("cannot promote from %s to %s (allowed: %v)",
                from, to, toTarget.Promotion.AllowedFrom)
        }
    }

    return nil
}
```

## Approval Workflows

### Atmos Pro Approvals

When `requires_approval: true`:

1. **CLI requests approval** via Atmos Pro API
2. **Approval notification** sent to configured approvers
3. **CLI waits** for approval (with timeout)
4. **Approval recorded** as Git note on commit
5. **Deployment proceeds** after approval

**Example workflow:**

```bash
# Request promotion to prod
atmos deployment rollout payment-service --target prod --promote-from staging

# Output:
# → Validating promotion path: staging → prod ✓
# → Requesting approval from Atmos Pro...
# → Approval request sent to: ops-team@example.com
# → Waiting for approval (timeout: 1h)...
# → Approval received from: alice@example.com at 2025-01-22T12:30:00Z
# → Recording approval as git note
# → Deploying to prod...
```

**Approval recorded as Git note:**

```bash
git notes --ref=atmos/payment-service/approvals show abc123def
```

```json
{
  "approved_by": "alice@example.com",
  "approved_at": "2025-01-22T12:30:00Z",
  "approver_role": "ops-lead",
  "target": "prod",
  "deployment": "payment-service",
  "promoted_from": "staging"
}
```

### Manual Approvals (Future)

For self-hosted deployments without Atmos Pro:

```yaml
prod:
  promotion:
    from: staging
    requires_approval: true
    approval_method: manual  # CLI prompts for confirmation
```

```bash
atmos deployment rollout payment-service --target prod --promote-from staging

# Output:
# → Validating promotion path: staging → prod ✓
# → Manual approval required for prod deployment
# → Deploying payment-service to prod
# → Git SHA: abc123def
# → Components: api, worker, database
# → Approve deployment? [y/N]:
```

## Promotion Workflows

### Standard Promotion

Promote the currently deployed version from one target to another:

```bash
# Get current staging deployment
STAGING_SHA=$(git show-ref --hash refs/atmos/deployments/payment-service/staging/api)

# Promote to prod (same commit as staging)
atmos deployment rollout payment-service --target prod --promote-from staging
# → Validates promotion path
# → Requests approval (if required)
# → Updates all component refs to same SHA as staging
# → Creates deployment tags
```

### Selective Component Promotion

Promote only specific components:

```bash
# Promote only the API component
atmos deployment rollout payment-service --target prod --promote-from staging --component api
# → Validates promotion path
# → Requests approval (if required)
# → Updates only refs/atmos/deployments/payment-service/prod/api
```

### Cross-Deployment Promotion (Not Allowed)

Cannot promote across different deployments:

```bash
# Invalid: promoting from different deployment
atmos deployment rollout inventory-service --target prod --promote-from payment-service/prod
# ❌ Error: Cannot promote across deployments. Use same-deployment promotion only.
```

## Git Tracking Integration

Promotion creates Git refs and tags with promotion metadata:

### Deployment Tag Metadata

```yaml
atmos.tools/v1alpha1
kind: Deployment
---
deployment: payment-service
target: prod
component: api
deployed_by: ci@example.com
build_id: abc123
git_sha: abc123def456789
timestamp: 2025-01-22T12:00:00Z
status: success
promoted_from: staging  # Source target
promoted_at: 2025-01-22T12:00:00Z
metadata:
  pr: "#482"
  jira: PROJ-123
  approver: alice@example.com
  approval_timestamp: 2025-01-22T11:55:00Z
```

### Promotion History Query

```bash
# Show all promotions to prod
git tag -l "atmos/deployments/payment-service/prod/*" --sort=-creatordate | while read tag; do
  git cat-file -p "$tag" | grep -A5 "promoted_from"
done

# Output:
# promoted_from: staging
# promoted_at: 2025-01-22T12:00:00Z
# approver: alice@example.com
```

## Validation Rules

### Promotion Path Rules

1. **Entry points** - Targets without `promotion` config accept any deployment
2. **Single source** - `from: <target>` enforces single allowed source
3. **Multiple sources** - `allowed_from: [<target1>, <target2>]` allows multiple sources
4. **Mutual exclusivity** - Cannot specify both `from` and `allowed_from`
5. **Target existence** - Source target must exist in same deployment
6. **Cycle detection** - No circular promotion paths allowed

### Validation Implementation

```go
// pkg/deployment/validation.go
func ValidatePromotionConfig(deployment *Deployment) error {
    for targetName, target := range deployment.Targets {
        if target.Promotion == nil {
            continue
        }

        // Rule: Mutual exclusivity
        if target.Promotion.From != "" && len(target.Promotion.AllowedFrom) > 0 {
            return fmt.Errorf("target %s: cannot specify both 'from' and 'allowed_from'", targetName)
        }

        // Rule: Target existence (single source)
        if target.Promotion.From != "" {
            if _, exists := deployment.Targets[target.Promotion.From]; !exists {
                return fmt.Errorf("target %s: promotion source '%s' does not exist",
                    targetName, target.Promotion.From)
            }
        }

        // Rule: Target existence (multiple sources)
        for _, source := range target.Promotion.AllowedFrom {
            if _, exists := deployment.Targets[source]; !exists {
                return fmt.Errorf("target %s: promotion source '%s' does not exist",
                    targetName, source)
            }
        }
    }

    // Rule: Cycle detection
    return detectPromotionCycles(deployment)
}

func detectPromotionCycles(deployment *Deployment) error {
    visited := make(map[string]bool)
    recStack := make(map[string]bool)

    for targetName := range deployment.Targets {
        if isCyclic(targetName, deployment, visited, recStack) {
            return fmt.Errorf("circular promotion path detected involving target: %s", targetName)
        }
    }

    return nil
}

func isCyclic(target string, deployment *Deployment, visited, recStack map[string]bool) bool {
    visited[target] = true
    recStack[target] = true

    targetConfig := deployment.Targets[target]
    if targetConfig.Promotion != nil {
        // Check single source
        if targetConfig.Promotion.From != "" {
            if !visited[targetConfig.Promotion.From] {
                if isCyclic(targetConfig.Promotion.From, deployment, visited, recStack) {
                    return true
                }
            } else if recStack[targetConfig.Promotion.From] {
                return true
            }
        }

        // Check multiple sources
        for _, source := range targetConfig.Promotion.AllowedFrom {
            if !visited[source] {
                if isCyclic(source, deployment, visited, recStack) {
                    return true
                }
            } else if recStack[source] {
                return true
            }
        }
    }

    recStack[target] = false
    return false
}
```

## CLI Commands

### Promotion Commands

```bash
# Promote from one target to another
atmos deployment rollout <deployment> --target <to> --promote-from <from>

# Promote specific component
atmos deployment rollout <deployment> --target <to> --promote-from <from> --component <component>

# Show promotion graph
atmos deployment promotion-graph <deployment>
# Output:
# dev → staging → prod
#       ↓
#       qa

# Validate promotion config
atmos deployment validate <deployment>
# Checks for cycles, missing targets, invalid config
```

### Promotion Status

```bash
# Show what can be promoted
atmos deployment promotion-status payment-service

# Output:
# payment-service promotion status:
#
# dev → staging (ready)
#   api: abc123def (deployed 2h ago)
#   worker: abc123def (deployed 2h ago)
#   database: abc123def (deployed 2h ago)
#
# staging → prod (ready, requires approval)
#   api: abc123def (deployed 1h ago)
#   worker: abc123def (deployed 1h ago)
#   database: abc123def (deployed 1h ago)
```

## Benefits

### Safety

- ✅ **Prevents accidental prod deployments** - Cannot skip staging
- ✅ **Enforces quality gates** - Each environment validates before next
- ✅ **Approval workflows** - Explicit sign-off for production
- ✅ **Audit trail** - All promotions tracked in Git

### Flexibility

- ✅ **Per-deployment targets** - Different apps have different environments
- ✅ **Multiple promotion paths** - Support complex workflows (dev/qa → staging)
- ✅ **Optional restrictions** - Targets without promotion config are unrestricted
- ✅ **Configuration-driven** - No code changes for new promotion paths

### Developer Experience

- ✅ **Clear error messages** - "Cannot promote from dev to prod. Must promote from staging."
- ✅ **Promotion visualization** - `atmos deployment promotion-graph`
- ✅ **Status visibility** - `atmos deployment promotion-status`
- ✅ **Git-native** - Promotion history in Git tags

## See Also

- **[git-tracking.md](./git-tracking.md)** - Git refs, tags, and notes for deployment tracking
- **[overview.md](./overview.md)** - Core concepts and definitions
- **[configuration.md](./configuration.md)** - Deployment schema
- **[cli-commands.md](./cli-commands.md)** - Complete command reference
