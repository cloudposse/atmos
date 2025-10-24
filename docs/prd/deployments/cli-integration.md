# Atmos Deployments - CLI Integration

This document describes the user-facing Atmos CLI commands for deployments, showing how the Git-based tracking system integrates seamlessly into the existing Atmos workflow.

## Overview

Users interact with deployments through high-level Atmos commands. The Git-based tracking (refs, tags, notes) is an implementation detail that provides:
- **No external database** - Git is the storage layer
- **Instant queries** - Git operations are fast
- **Audit trail** - All deployment history in version control
- **Universal compatibility** - Works with any Git remote

**Key principle:** Users use `atmos deployment` commands, not `git` commands.

## Command Structure

All deployment commands follow this pattern:

```bash
atmos deployment <verb> [deployment] [flags]
```

**Verbs:**
- `build` - Build container images and generate SBOMs
- `rollout` - Deploy to a target (or promote between targets)
- `status` - Show current deployment state
- `history` - Show deployment history
- `diff` - Compare deployments between targets
- `list` - List deployments or components
- `sbom` - Show SBOM for a deployment
- `logs` - Show deployment logs (future)
- `validate` - Validate deployment configuration
- `promotion-graph` - Visualize promotion paths

## Core Commands

### `atmos deployment build`

Build container images and generate artifacts.

**Usage:**
```bash
atmos deployment build <deployment> --target <target> [flags]
```

**Examples:**
```bash
# Build all components for dev target
atmos deployment build payment-service --target dev

# Build specific component
atmos deployment build payment-service --target dev --component api

# Build with custom context
atmos deployment build payment-service --target dev --context cpu=512
```

**Behind the scenes:**
1. Loads deployment config: `deployments/payment-service.yaml`
2. Finds components with `type: nixpack`
3. Builds container images using Nixpacks
4. Generates SBOM (CycloneDX JSON)
5. Attaches SBOM to commit as Git note: `refs/notes/atmos/payment-service/sbom`
6. Pushes images to registry

**Output:**
```
Building payment-service for target dev...

✓ api       Built sha256:abc123... (2.3s)
✓ worker    Built sha256:def456... (1.8s)
✓ database  Built sha256:ghi789... (0.9s)

Generated SBOM: 3 components, 47 dependencies
Attached SBOM to commit abc123def

Images pushed to registry.example.com/payment-service
```

### `atmos deployment rollout`

Deploy to a target or promote between targets.

**Usage:**
```bash
# Deploy to target (new deployment)
atmos deployment rollout <deployment> --target <target> [flags]

# Promote from another target
atmos deployment rollout <deployment> --target <target> --promote-from <source-target> [flags]
```

**Examples:**
```bash
# Deploy to dev (current commit)
atmos deployment rollout payment-service --target dev

# Deploy specific commit to dev
atmos deployment rollout payment-service --target dev --git-sha abc123def

# Promote dev to staging
atmos deployment rollout payment-service --target staging --promote-from dev

# Promote staging to prod (requires approval)
atmos deployment rollout payment-service --target prod --promote-from staging

# Rollback prod to previous deployment
atmos deployment rollout payment-service --target prod --git-sha def456abc
```

**Behind the scenes (new deployment):**
1. Validates deployment config exists
2. Builds containers (if not already built)
3. Applies infrastructure using Terraform/OpenTofu
4. Updates Git refs: `refs/atmos/deployments/payment-service/dev/api → abc123def`
5. Creates Git tags with metadata: `atmos/deployments/payment-service/dev/api/2025-01-22T10-30-00Z`
6. Pushes refs and tags to remote

**Behind the scenes (promotion):**
1. Validates promotion path (dev → staging allowed?)
2. Gets current commit from source target refs
3. Requests approval if required
4. Records approval as Git note
5. Updates target refs to source commit
6. Creates deployment tags with promotion metadata
7. Applies infrastructure with promoted config

**Output (promotion):**
```
Promoting payment-service from staging to prod...

→ Validating promotion path: staging → prod ✓
→ Current staging deployment: abc123def (deployed 2h ago)
→ Requesting approval from Atmos Pro...
→ Approval received from alice@example.com
→ Recording approval

Deploying to prod:
  api      abc123def → abc123def (no change)
  worker   abc123def → abc123def (no change)
  database abc123def → abc123def (no change)

✓ Deployment complete (12.3s)

Deployment ID: atmos/deployments/payment-service/prod/api/2025-01-22T12-00-00Z
```

### `atmos deployment status`

Show current deployment state.

**Usage:**
```bash
atmos deployment status <deployment> --target <target> [flags]
```

**Examples:**
```bash
# Show status for specific target
atmos deployment status payment-service --target prod

# Show status across all targets
atmos deployment status payment-service

# Show status for all deployments
atmos deployment status
```

**Behind the scenes:**
- Queries Git refs: `git show-ref | grep refs/atmos/deployments/payment-service/prod`
- Parses latest deployment tags for metadata
- Formats output

**Output:**
```bash
$ atmos deployment status payment-service --target prod

payment-service → prod

Component   Commit      Deployed              By                  Status
api         abc123d     2h ago (12:00:00)     ci@example.com      success
worker      abc123d     2h ago (12:00:00)     ci@example.com      success
database    abc123d     2h ago (12:00:00)     ci@example.com      success

Promoted from: staging
Approved by: alice@example.com (11:55:00)
Build ID: build-482
PR: #482
```

```bash
$ atmos deployment status payment-service

payment-service

Target      Component   Commit      Deployed          Status
dev         api         def456a     30m ago           success
dev         worker      def456a     30m ago           success
dev         database    def456a     30m ago           success
staging     api         abc123d     2h ago            success
staging     worker      abc123d     2h ago            success
staging     database    abc123d     2h ago            success
prod        api         abc123d     2h ago            success
prod        worker      abc123d     2h ago            success
prod        database    abc123d     2h ago            success
```

### `atmos deployment history`

Show deployment history.

**Usage:**
```bash
atmos deployment history <deployment> --target <target> [flags]
```

**Examples:**
```bash
# Show history for target (all components)
atmos deployment history payment-service --target prod

# Show history for specific component
atmos deployment history payment-service --target prod --component api

# Show last 10 deployments
atmos deployment history payment-service --target prod --limit 10

# Show deployments in date range
atmos deployment history payment-service --target prod --since 2025-01-01 --until 2025-01-31
```

**Behind the scenes:**
- Queries Git tags: `git tag -l "atmos/deployments/payment-service/prod/*" --sort=-creatordate`
- Parses tag messages for metadata
- Formats timeline

**Output:**
```bash
$ atmos deployment history payment-service --target prod

payment-service → prod deployment history

Date                Commit      Component   By                  Status      Details
2025-01-22 12:00    abc123d     api         ci@example.com      success     Promoted from staging, PR #482
2025-01-22 12:00    abc123d     worker      ci@example.com      success     Promoted from staging, PR #482
2025-01-22 12:00    abc123d     database    abc123d             success     Promoted from staging, PR #482
2025-01-20 09:15    def456a     api         ci@example.com      success     Promoted from staging, PR #475
2025-01-20 09:15    def456a     worker      ci@example.com      success     Promoted from staging, PR #475
2025-01-20 09:15    def456a     database    def456a             success     Promoted from staging, PR #475
2025-01-18 14:30    ghi789b     api         ci@example.com      rollback    Rollback of jkl012c, INCIDENT-42

Showing 7 deployments (use --limit to show more)
```

### `atmos deployment diff`

Compare deployments between targets or commits.

**Usage:**
```bash
atmos deployment diff <deployment> --from-target <target1> --to-target <target2> [flags]
atmos deployment diff <deployment> --from-sha <sha1> --to-sha <sha2> [flags]
```

**Examples:**
```bash
# Compare prod vs staging
atmos deployment diff payment-service --from-target prod --to-target staging

# Compare specific commits
atmos deployment diff payment-service --from-sha abc123 --to-sha def456

# Show component-level diff
atmos deployment diff payment-service --from-target prod --to-target staging --component api

# Show file changes
atmos deployment diff payment-service --from-target prod --to-target staging --show-files
```

**Behind the scenes:**
- Gets commits from Git refs for each target
- Runs `git diff <sha1>..<sha2>`
- Parses deployment configs for component changes
- Shows infrastructure drift

**Output:**
```bash
$ atmos deployment diff payment-service --from-target prod --to-target staging

payment-service: prod vs staging

Commit Diff:
  prod:    abc123def (deployed 2h ago)
  staging: def456abc (deployed 30m ago)
  Commits behind: 3

Component Changes:
  api:      abc123d → def456a  (changed)
  worker:   abc123d → def456a  (changed)
  database: abc123d → abc123d  (no change)

Configuration Changes:
  api.context.cpu:     2048 → 2048  (no change)
  api.context.memory:  4096 → 4096  (no change)
  worker.context.cpu:  1024 → 2048  (changed)

File Changes (3 commits):
  feat: increase worker CPU for better performance (def456a)
  fix: api connection timeout handling (cde345b)
  docs: update deployment readme (bcd234a)

Infrastructure Drift:
  ✓ No drift detected
```

### `atmos deployment list`

List deployments, targets, or components.

**Usage:**
```bash
atmos deployment list [flags]
atmos deployment list deployments
atmos deployment list targets <deployment>
atmos deployment list components <deployment> --target <target>
```

**Examples:**
```bash
# List all deployments
atmos deployment list
atmos deployment list deployments

# List targets for deployment
atmos deployment list targets payment-service

# List components for deployment/target
atmos deployment list components payment-service --target prod

# Filter by label
atmos deployment list --label environment=prod
```

**Behind the scenes:**
- Scans `deployments/` directory for YAML files
- Parses deployment configs
- Queries Git refs for deployment state

**Output:**
```bash
$ atmos deployment list

Deployments (3)

Name                Targets                 Components       Last Deploy
payment-service     dev, staging, prod      3 (api, worker)  2h ago (prod)
inventory-service   dev, staging, prod      2 (api, cache)   1d ago (prod)
analytics-service   dev, staging            4 (...)          3h ago (staging)
```

```bash
$ atmos deployment list targets payment-service

payment-service targets

Target      Promotion From    Requires Approval    Last Deploy       Status
dev         (entry point)     no                   30m ago           healthy
staging     dev               no                   2h ago            healthy
prod        staging           yes                  2h ago            healthy
```

```bash
$ atmos deployment list components payment-service --target prod

payment-service → prod components

Component   Type      Commit      Deployed    Status      Health
api         nixpack   abc123d     2h ago      success     healthy (3/3 pods)
worker      nixpack   abc123d     2h ago      success     healthy (5/5 pods)
database    nixpack   abc123d     2h ago      success     healthy (1/1 pods)
```

### `atmos deployment sbom`

Show SBOM (Software Bill of Materials) for a deployment.

**Usage:**
```bash
atmos deployment sbom <deployment> --target <target> [flags]
```

**Examples:**
```bash
# Show SBOM for deployed commit
atmos deployment sbom payment-service --target prod

# Show SBOM for specific commit
atmos deployment sbom payment-service --git-sha abc123def

# Export SBOM to file
atmos deployment sbom payment-service --target prod --output sbom.json

# Show summary
atmos deployment sbom payment-service --target prod --summary
```

**Behind the scenes:**
- Gets commit from Git ref
- Retrieves SBOM from Git note: `refs/notes/atmos/payment-service/sbom`
- Parses CycloneDX JSON
- Formats output

**Output:**
```bash
$ atmos deployment sbom payment-service --target prod --summary

payment-service → prod SBOM

Commit: abc123def
Generated: 2025-01-22 10:25:00
Format: CycloneDX 1.5 (JSON)

Components: 3
  api       (docker image)
  worker    (docker image)
  database  (docker image)

Dependencies: 47 total
  golang.org/x/net       v0.17.0
  github.com/gin-gonic   v1.9.1
  ...

Vulnerabilities: 0 critical, 2 medium, 5 low

Full SBOM: 234 KB
```

### `atmos deployment validate`

Validate deployment configuration.

**Usage:**
```bash
atmos deployment validate <deployment> [flags]
```

**Examples:**
```bash
# Validate deployment config
atmos deployment validate payment-service

# Validate all deployments
atmos deployment validate --all

# Validate promotion paths
atmos deployment validate payment-service --check-promotions
```

**Behind the scenes:**
- Loads deployment YAML
- Validates against JSON schema
- Checks promotion path cycles
- Validates target existence
- Checks component references

**Output:**
```bash
$ atmos deployment validate payment-service

Validating payment-service...

✓ Deployment configuration is valid
✓ All targets defined
✓ All components exist
✓ Promotion paths valid (no cycles)
✓ Context variables valid
✓ Labels valid

Summary:
  Deployment: payment-service
  Targets: 3 (dev, staging, prod)
  Components: 3 (api, worker, database)
  Promotion path: dev → staging → prod
```

### `atmos deployment promotion-graph`

Visualize promotion paths.

**Usage:**
```bash
atmos deployment promotion-graph <deployment> [flags]
```

**Examples:**
```bash
# Show promotion graph
atmos deployment promotion-graph payment-service

# Show graph with status
atmos deployment promotion-graph payment-service --show-status
```

**Output:**
```bash
$ atmos deployment promotion-graph payment-service

payment-service promotion graph:

     ┌─────────────┐
     │     dev     │  (entry point)
     │  def456abc  │  deployed 30m ago
     └─────────────┘
            ↓
     ┌─────────────┐
     │   staging   │  from: dev
     │  abc123def  │  deployed 2h ago
     └─────────────┘
            ↓
     ┌─────────────┐
     │    prod     │  from: staging, requires approval
     │  abc123def  │  deployed 2h ago
     └─────────────┘
```

```bash
$ atmos deployment promotion-graph payment-service --show-status

payment-service promotion graph:

     ┌─────────────────────────────┐
     │           dev               │
     │       def456abc             │
     │   deployed 30m ago          │
     │   status: healthy           │
     │   ready to promote ✓        │
     └─────────────────────────────┘
                  ↓
     ┌─────────────────────────────┐
     │         staging             │
     │       abc123def             │
     │   deployed 2h ago           │
     │   status: healthy           │
     │   drift: 3 commits behind   │
     │   ready to promote ✓        │
     └─────────────────────────────┘
                  ↓
     ┌─────────────────────────────┐
     │          prod               │
     │       abc123def             │
     │   deployed 2h ago           │
     │   status: healthy           │
     │   up to date ✓              │
     └─────────────────────────────┘
```

## Integration Points

### Where Git Tracking Integrates

**Command → Git Operations:**

| Command | Git Reads | Git Writes |
|---------|-----------|------------|
| `build` | (none) | Git note (SBOM) |
| `rollout` | Refs (for promotion) | Refs + Tags + Notes (approval) |
| `status` | Refs + Tags | (none) |
| `history` | Tags | (none) |
| `diff` | Refs + Tags | (none) |
| `list` | Refs + Tags | (none) |
| `sbom` | Refs + Notes | (none) |
| `validate` | (none, config only) | (none) |
| `promotion-graph` | Refs + Config | (none) |

### Implementation Packages

```
pkg/
  git/                    # Git operations (refs, tags, notes)
    interface.go          # Repository interface
    repository.go         # Git command wrapper
    refs.go               # Ref management
    tags.go               # Tag creation/parsing
    notes.go              # Note attachment/retrieval

  deployment/             # Deployment logic
    interface.go          # Deployment interface
    deployment.go         # Core deployment operations
    promotion.go          # Promotion validation
    status.go             # Status queries
    history.go            # History queries
    diff.go               # Diff operations
    sbom.go               # SBOM retrieval

cmd/
  deployment/             # CLI commands
    build.go              # Build command
    rollout.go            # Rollout command
    status.go             # Status command
    history.go            # History command
    diff.go               # Diff command
    list.go               # List command
    sbom.go               # SBOM command
    validate.go           # Validate command
    promotion_graph.go    # Promotion graph command
    provider.go           # CommandProvider interface
```

### Existing Atmos Integration

Deployment commands integrate with existing Atmos features:

**Stack processing:**
- Deployment configs use same YAML processing as stacks
- Components reference existing Terraform/Helmfile/Packer components
- Template functions work in deployment configs

**Vendoring:**
- JIT vendoring for deployment components
- Vendor cache shared with stack components

**Workflows:**
- Deployment commands can be orchestrated via workflows
- `atmos workflow deploy-all` can call `atmos deployment rollout`

**Validation:**
- JSON schema validation for deployment configs
- OPA policies can validate deployment decisions

**Pro features:**
- Approval workflows integrate with Atmos Pro
- RBAC controls who can deploy to which targets

## User Experience

### Typical Workflow

**Developer (dev environment):**
```bash
# Make code changes
git checkout -b feature/improve-performance

# Build and deploy to dev
atmos deployment build payment-service --target dev
atmos deployment rollout payment-service --target dev

# Verify deployment
atmos deployment status payment-service --target dev
```

**CI/CD (staging promotion):**
```bash
# Merge to main triggers CI
git checkout main
git pull

# Promote dev to staging
atmos deployment rollout payment-service --target staging --promote-from dev

# Run smoke tests
atmos deployment status payment-service --target staging
```

**Release Manager (prod promotion):**
```bash
# Check what's in staging
atmos deployment status payment-service --target staging

# Compare staging vs prod
atmos deployment diff payment-service --from-target prod --to-target staging

# View promotion graph
atmos deployment promotion-graph payment-service

# Promote to prod (with approval)
atmos deployment rollout payment-service --target prod --promote-from staging
# → Waits for approval from Atmos Pro
# → Deploys after approval
```

**SRE (troubleshooting):**
```bash
# View deployment history
atmos deployment history payment-service --target prod

# Check SBOM for vulnerabilities
atmos deployment sbom payment-service --target prod --summary

# Rollback if needed
atmos deployment rollout payment-service --target prod --git-sha <previous-sha>
```

### No Git Commands Required

Users **never** run `git` commands directly for deployments. All Git operations are abstracted:

❌ **Don't do this:**
```bash
git show-ref refs/atmos/deployments/payment-service/prod/api
git tag -l "atmos/deployments/payment-service/prod/*"
git notes --ref=atmos/payment-service/sbom show abc123
```

✅ **Do this:**
```bash
atmos deployment status payment-service --target prod
atmos deployment history payment-service --target prod
atmos deployment sbom payment-service --target prod
```

### Error Messages

Clear, actionable error messages guide users:

```bash
$ atmos deployment rollout payment-service --target prod --promote-from dev

Error: Cannot promote directly from dev to prod
Promotion path requires: dev → staging → prod

To deploy to prod:
  1. Promote to staging first:
     atmos deployment rollout payment-service --target staging --promote-from dev

  2. Then promote to prod:
     atmos deployment rollout payment-service --target prod --promote-from staging

Or skip promotion validation (not recommended):
  atmos deployment rollout payment-service --target prod --skip-promotion-check
```

## Benefits

### Developer Experience

- ✅ **Familiar Atmos commands** - Same patterns as `atmos terraform`, `atmos describe`
- ✅ **No Git expertise required** - Git is implementation detail
- ✅ **Rich output** - Tables, graphs, diffs formatted for readability
- ✅ **Fast queries** - Git operations are near-instant
- ✅ **Works offline** - Local git repo has full history

### Operations

- ✅ **No external database** - Git is the storage layer
- ✅ **Instant queries** - No database round-trips
- ✅ **Audit trail** - All deployment history in version control
- ✅ **Disaster recovery** - Clone repo, have full deployment history
- ✅ **Multi-region** - Each region can have separate Git remotes

### CI/CD Integration

- ✅ **Simple automation** - Just run Atmos commands
- ✅ **No credentials** - Uses existing Git access
- ✅ **Idempotent** - Safe to re-run commands
- ✅ **Parallel deploys** - Different deployments don't conflict

## See Also

- **[git-tracking.md](./git-tracking.md)** - Git refs, tags, and notes implementation details
- **[target-promotion.md](./target-promotion.md)** - Promotion path validation
- **[configuration.md](./configuration.md)** - Deployment YAML schema
- **[overview.md](./overview.md)** - Core concepts and definitions
