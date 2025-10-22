# Atmos Deployments - CLI Commands Reference

This document provides a complete reference for all deployment-related CLI commands.

## Command Structure

All deployment commands follow the pattern:
```
atmos deployment <verb> <deployment> --target <target> [options]
```

## Core Commands

### `atmos deployment list`

List all deployments in the repository.

```bash
# List all deployments
atmos deployment list

# Output:
# NAME                STACKS    COMPONENTS    TARGETS
# payment-service     4         5             dev, staging, prod
# background-worker   3         3             dev, prod
# platform            6         12            dev, staging, prod

# List with details
atmos deployment list --verbose

# Filter by label
atmos deployment list --label team=backend
```

### `atmos deployment describe`

Show detailed information about a deployment.

```bash
# Describe deployment
atmos deployment describe payment-service

# Output:
# Deployment: payment-service
# Description: Payment processing service
# Stacks: platform/vpc, platform/eks, ecr, ecs
# Targets: dev, staging, prod
#
# Components:
#   nixpack/api (depends: terraform/ecr/api)
#   terraform/ecr/api
#   terraform/ecs/taskdef-api (depends: nixpack/api)
#   terraform/ecs/service-api (depends: terraform/ecs/taskdef-api)
#
# Targets:
#   dev: cpu=256, memory=512, replicas=1
#   staging: cpu=512, memory=1024, replicas=2
#   prod: cpu=1024, memory=2048, replicas=4

# Describe specific target
atmos deployment describe payment-service --target prod

# Output as JSON
atmos deployment describe payment-service --format json
```

## Build Stage

### `atmos deployment build`

Build container images for nixpack components.

```bash
# Build all nixpack components for target
atmos deployment build payment-service --target dev

# Output:
# Building nixpack components for deployment 'payment-service' target 'dev'...
# ✓ api: Detected Go application
# ✓ api: Building container image...
# ✓ api: Pushing to registry: 123456789012.dkr.ecr.us-east-1.amazonaws.com/api@sha256:abc123...
# ✓ api: Generating SBOM (cyclonedx-json)...
# Build completed in 2m 34s

# Build specific component
atmos deployment build payment-service --target dev --component api

# Build without pushing to registry
atmos deployment build payment-service --target dev --skip-push

# Show build plan without building
atmos deployment build payment-service --target dev --plan

# Build with custom tag (advisory)
atmos deployment build payment-service --target dev --tag v1.2.3
```

## Test Stage

### `atmos deployment test`

Run tests inside built containers.

```bash
# Test all components
atmos deployment test payment-service --target dev

# Test specific component
atmos deployment test payment-service --target dev --component api

# Test with custom command
atmos deployment test payment-service --target dev --command "npm test"

# Output JUnit XML
atmos deployment test payment-service --target dev --junit-xml test-results.xml
```

## Release Stage

### `atmos deployment release`

Create immutable release record.

```bash
# Create release for target
atmos deployment release payment-service --target dev

# Output:
# Creating release for deployment 'payment-service' target 'dev'...
# ✓ Captured image digests from build
# ✓ Generated SBOM (cyclonedx-json)
# ✓ Collected Git metadata (sha: abc123, branch: main)
# ✓ Created release record: releases/payment-service/dev/release-xyz789.yaml
# Release ID: xyz789

# Create release with annotations
atmos deployment release payment-service --target dev \
  --annotation pr=#482 \
  --annotation jira=PROJ-123 \
  --annotation description="Add user authentication"

# Create release with specific ID
atmos deployment release payment-service --target dev --id v1.2.3
```

### `atmos deployment releases`

List releases for a deployment.

```bash
# List all releases for deployment
atmos deployment releases payment-service

# Output:
# ID       TARGET    CREATED              STATUS      IMAGES
# xyz789   dev       2025-01-15 10:30:00  active      api@sha256:abc123...
# abc456   dev       2025-01-14 15:20:00  superseded  api@sha256:def456...
# ghi123   staging   2025-01-15 11:00:00  active      api@sha256:abc123...
# jkl456   prod      2025-01-13 09:00:00  active      api@sha256:mno789...

# List releases for specific target
atmos deployment releases payment-service --target prod

# Show detailed release information
atmos deployment releases payment-service --target dev --verbose
```

## Rollout Stage

### `atmos deployment rollout`

Update infrastructure components with release.

```bash
# Rollout latest release to target
atmos deployment rollout payment-service --target dev

# Output:
# Rolling out deployment 'payment-service' to target 'dev'...
# Using release: xyz789
# ✓ Component: terraform/ecr/api (12s)
# ✓ Component: nixpack/api (skipped - already built)
# ✓ Component: terraform/ecs/taskdef-api (8s)
#   → Updated image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/api@sha256:abc123...
# ✓ Component: terraform/ecs/service-api (45s)
#   → Service updated, waiting for deployment...
# Rollout completed in 3m 12s

# Rollout specific release
atmos deployment rollout payment-service --target dev --release abc456

# Rollout specific component
atmos deployment rollout payment-service --target dev --component ecs/service-api

# Rollout with parallelism
atmos deployment rollout payment-service --target prod --parallelism 4

# Dry-run (plan only)
atmos deployment rollout payment-service --target prod --plan

# Rollout and keep workspace for debugging
atmos deployment rollout payment-service --target dev --keep-workspace

# Promote release from dev to staging
atmos deployment rollout payment-service --target staging --release xyz789
```

## Status & Monitoring

### `atmos deployment status`

Show deployment status across targets.

```bash
# Show status for all targets
atmos deployment status payment-service

# Output:
# TARGET    RELEASE    DEPLOYED             STATUS    DRIFT
# dev       xyz789     2025-01-15 10:45:00  healthy   none
# staging   ghi123     2025-01-15 11:15:00  healthy   none
# prod      jkl456     2025-01-13 09:30:00  healthy   detected

# Show status for specific target
atmos deployment status payment-service --target prod

# Check for drift
atmos deployment status payment-service --target prod --check-drift
```

## CI/CD Integration

### `atmos deployment matrix`

Generate CI matrix configuration.

```bash
# Generate matrix for all components
atmos deployment matrix payment-service --target prod --format json

# Output:
{
  "include": [
    {"component": "ecr/api", "wave": 1, "depends_on": []},
    {"component": "nixpack/api", "wave": 2, "depends_on": ["ecr/api"]},
    {"component": "ecs/taskdef-api", "wave": 3, "depends_on": ["nixpack/api"]},
    {"component": "ecs/service-api", "wave": 4, "depends_on": ["ecs/taskdef-api"]}
  ]
}

# Generate wave-based matrix (sequential waves)
atmos deployment matrix payment-service --target prod --waves --format json

# Generate matrix for specific components
atmos deployment matrix payment-service --target prod --components api,worker
```

### `atmos deployment approve`

Request approval for deployment (Atmos Pro integration).

```bash
# Request approval for rollout
atmos deployment approve payment-service --target prod

# Output:
# Requesting approval for deployment 'payment-service' to 'prod'...
# Approval URL: https://app.atmos.tools/approvals/abc-123
# Waiting for approval...
# ✓ Approved by: user@example.com at 2025-01-15 12:00:00

# Request approval for specific component
atmos deployment approve payment-service --target prod --component ecs/service-api

# Approve with timeout
atmos deployment approve payment-service --target prod --timeout 30m
```

## Vendoring Commands

### `atmos vendor pull --deployment`

Vendor components for deployment (JIT vendoring).

```bash
# Vendor all components for deployment
atmos vendor pull --deployment payment-service

# Vendor for specific target
atmos vendor pull --deployment payment-service --target dev

# Update vendored components
atmos vendor pull --deployment payment-service --target prod --update
```

### `atmos vendor status --deployment`

Show vendor status for deployment.

```bash
# Show status for all targets
atmos vendor status --deployment payment-service

# Show status for specific target
atmos vendor status --deployment payment-service --target prod
```

### `atmos vendor cache stats`

Show vendor cache statistics.

```bash
# Show cache statistics
atmos vendor cache stats

# Output:
# Total size: 2.3 GB
# Unique objects: 145
# Deployment workspaces: 23
# Deduplication savings: 8.7 GB (78% reduction)
```

### `atmos vendor clean`

Clean unused vendor cache.

```bash
# Interactive cleanup
atmos vendor clean

# Force clean specific deployment
atmos vendor clean --deployment old-service --force

# Clean specific target
atmos vendor clean --deployment api --target staging --force

# Prune old objects
atmos vendor clean --prune --older-than 30d
```

## SBOM Commands

### `atmos deployment sbom`

Generate SBOM for deployment.

```bash
# Generate SBOM for deployment
atmos deployment sbom payment-service --target dev

# Output formats
atmos deployment sbom payment-service --target dev --format cyclonedx-json > sbom.cdx.json
atmos deployment sbom payment-service --target dev --format spdx-json > sbom.spdx.json

# Generate SBOM for specific component
atmos deployment sbom payment-service --target dev --component api
```

## Migration Commands

### `atmos deployment migrate`

Generate deployment definitions from existing stacks.

```bash
# Generate deployment from top-level stack
atmos deployment migrate --from-stack ue2-prod --name platform

# Output:
# Generated deployment: deployments/platform.yaml
# Discovered 12 components from stack 'ue2-prod'
# Targets: dev, staging, prod

# Migrate with custom output path
atmos deployment migrate --from-stack ue2-prod --name platform --output deployments/custom.yaml

# Dry-run (show what would be generated)
atmos deployment migrate --from-stack ue2-prod --name platform --dry-run
```

## Global Flags

All commands support these global flags:

```bash
--config FILE         # Specify atmos.yaml path
--chdir DIR           # Change to directory before executing
--verbose, -v         # Enable verbose output
--quiet, -q           # Suppress non-error output
--format FORMAT       # Output format: yaml, json, table (default: yaml)
--color               # Enable/disable color output (auto, always, never)
```

## Examples

### Complete Workflow

```bash
# 1. Build containers
atmos deployment build payment-service --target dev

# 2. Test containers
atmos deployment test payment-service --target dev

# 3. Create release
atmos deployment release payment-service --target dev

# 4. Rollout to dev
atmos deployment rollout payment-service --target dev

# 5. Check status
atmos deployment status payment-service --target dev

# 6. Promote to staging
RELEASE_ID=$(atmos deployment releases payment-service --target dev --format json | jq -r '.[0].id')
atmos deployment rollout payment-service --target staging --release $RELEASE_ID
```

### Rollback

```bash
# List releases
atmos deployment releases payment-service --target prod

# Rollback to previous release
atmos deployment rollout payment-service --target prod --release abc456
```

### Concurrent Deployment

```bash
# Rollout with parallelism
atmos deployment rollout payment-service --target prod --parallelism 4

# Deployment executes in waves:
# Wave 1 (parallel, max 4): ecr/api
# Wave 2 (parallel, max 4): nixpack/api
# Wave 3 (parallel, max 4): ecs/taskdef-api
# Wave 4 (parallel, max 4): ecs/service-api
```

## See Also

- **[overview.md](./overview.md)** - Core concepts and definitions
- **[configuration.md](./configuration.md)** - Deployment YAML schema
- **[vendoring.md](./vendoring.md)** - JIT vendoring commands
- **[concurrent-execution.md](./concurrent-execution.md)** - Parallelism and workspaces
- **[cicd-integration.md](./cicd-integration.md)** - CI/CD provider integration
