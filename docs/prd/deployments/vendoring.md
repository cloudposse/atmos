# Atmos Deployments - Just-In-Time Vendoring

This document describes the JIT (Just-In-Time) vendoring strategy for deployments, including vendor cache architecture, cross-platform file sharing, and CLI commands.

## Overview

JIT vendoring dramatically improves performance by vendoring only the components required by a specific deployment, rather than vendoring the entire component catalog.

**Performance Comparison**:

```
Traditional Vendoring:
  atmos vendor pull
    → Parse vendor.yaml
    → Pull ALL components (50-100+ components)
    → Write to components/**
    → 30-60 seconds

JIT Vendoring with Deployments:
  atmos deployment rollout api --target dev
    → Load deployments/api.yaml
    → Identify referenced components (3-5 components)
    → Check vendor cache (.atmos/vendor-cache/deployments/api/)
    → Pull missing components only
    → Write to deployment-scoped vendor dir
    → 3-5 seconds (10x faster)
```

## Key Concepts

1. **Deployment-Scoped Vendoring**: Each deployment declares its component dependencies. Atmos vendors only those components when the deployment is processed.

2. **Lazy Evaluation**: Components are vendored on first use, not during repository initialization.

3. **Vendor Cache**: Centralized cache (`.atmos/vendor-cache/`) stores vendored components, indexed by source URL and version/tag. Multiple deployments share cached components.

4. **Environment-Specific Vendoring**: Use labels to vendor different component versions per environment (dev/staging/prod).

5. **Garbage Collection**: Unused vendored components can be cleaned up automatically based on age and usage.

## Vendor Configuration Strategies

Deployments support **three vendor configuration strategies**:

### Option 1: Inline Vendor Configuration

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

### Option 2: External vendor.yaml Reference

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

### Option 3: Repository-Wide vendor.yaml Fallback

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

## Configuration Resolution Order

1. **Inline vendor configuration** in deployment file (highest priority)
2. **External vendor manifest** referenced by `vendor.manifest`
3. **Repository-wide vendor.yaml** (fallback, auto-filtered)
4. **Auto-discovery** from component references (if enabled)

## Mixing Strategies

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

## Auto-Discovery

When `vendor.auto_discover: true` (default), Atmos scans the deployment's component definitions and automatically identifies external dependencies:

```yaml
components:
  terraform:
    ecs/service-api:
      component: "ecs-service"  # Auto-discovered: needs vendoring
      vars:
        cluster_name: "app-ecs"
```

**Auto-Discovery Process**:
1. Does `components/terraform/ecs-service/` exist locally?
2. If not, is it defined in `vendor.yaml` or deployment vendor config?
3. If yes, check vendor cache for matching content (by digest)
4. If not cached, pull and store in content-addressable cache
5. Create hard link (Unix/Mac) or copy (Windows) from cache to deployment workspace

## Vendor Cache Architecture

The vendor cache is a **centralized, content-addressable store** that eliminates duplication and enables efficient sharing across deployments and environments.

### Cache Structure

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

### Cross-Platform File Sharing Strategy

1. **Unix/Linux/macOS**: Use hard links from `objects/sha256/<digest>/` to deployment workspace
   - Space efficient (same inode, zero duplication)
   - Transparent to tools (looks like regular files)
   - No special privileges required
   - **Deduplication savings**: ~67% with 3 targets using same version

2. **Windows**: Copy files from cache to deployment workspace
   - Fallback when hard links fail
   - Slight disk usage increase, but still manageable
   - Cache cleanup reclaims space
   - **Deduplication savings**: ~50% (cache stores once, copies shared content)

3. **Implementation**: Detect OS and attempt hard link first, fall back to copy

```go
// pkg/vendor/cache.go
func (c *Cache) copyOrLinkDirectory(src, dest string) error {
    // Walk source directory
    return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        // Calculate destination path
        relPath, _ := filepath.Rel(src, path)
        destPath := filepath.Join(dest, relPath)

        if info.IsDir() {
            // Create directory
            return os.MkdirAll(destPath, info.Mode())
        }

        // Try hard link first (Unix/Mac/Windows NTFS)
        err = os.Link(path, destPath)
        if err != nil {
            // Fall back to copy (Windows FAT32, network drives, cross-device links)
            return copyFile(path, destPath)
        }
        return nil
    })
}

func copyFile(src, dest string) error {
    srcFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer srcFile.Close()

    destFile, err := os.Create(dest)
    if err != nil {
        return err
    }
    defer destFile.Close()

    _, err = io.Copy(destFile, srcFile)
    return err
}
```

### Benefits of Content-Addressable Cache

1. **Deduplication**: Same component version stored once, referenced by multiple deployments
2. **Environment Isolation**: Dev/staging/prod can use different versions without conflicts
3. **Fast Switching**: Changing versions is instant (link/copy from cache)
4. **Disk Efficiency**: 100 deployments using same component = 1x storage (Unix/Mac) or minimal overhead (Windows)
5. **Garbage Collection**: Easy to identify unreferenced objects

### Cache Index

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

### Deployment Lock File

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

## CLI Commands

### Vendor Pull

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

# Update vendored components for specific target
atmos vendor pull --deployment api --target prod --update
```

### Vendor Status

```bash
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
```

### Cache Statistics

```bash
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
```

### Vendor Clean (Garbage Collection)

```bash
# Clean unused vendor cache (interactive)
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

## Integration with Existing Vendoring

JIT vendoring is **opt-in per deployment**. Existing `atmos vendor pull` continues to work for repository-wide vendoring:

```bash
# Traditional: vendor everything from vendor.yaml
atmos vendor pull

# New: vendor only deployment-specific components
atmos vendor pull --deployment api --target dev
```

**Backward compatibility**: Deployments can reference components vendored via traditional `atmos vendor pull` - no duplication occurs.

## Implementation Details

### Vendor Cache Package Structure

```
pkg/vendor/
├── cache.go              # Content-addressable cache implementation
├── cache_test.go
├── discovery.go          # Auto-discovery of component dependencies
├── discovery_test.go
├── deployment.go         # Deployment-scoped vendoring
├── deployment_test.go
├── interface.go          # VendorProvider interface
├── lock.go               # Lock file generation and parsing
├── lock_test.go
└── gc.go                 # Garbage collection
```

### Key Interfaces

```go
// pkg/vendor/interface.go
type VendorProvider interface {
    // Pull vendors components for a deployment/target
    Pull(ctx context.Context, deployment, target string) error

    // Status returns vendor status for deployment/target
    Status(ctx context.Context, deployment, target string) (*VendorStatus, error)

    // Clean removes unused cache entries
    Clean(ctx context.Context, opts CleanOptions) (*CleanResult, error)
}

type Cache interface {
    // Get retrieves cached component by digest
    Get(digest string) (string, error)

    // Put stores component in cache, returns digest
    Put(source, version string, files []string) (string, error)

    // Link creates hard link/copy from cache to deployment workspace
    Link(digest, dest string) error

    // Stats returns cache statistics
    Stats() (*CacheStats, error)
}
```

## See Also

- **[overview.md](./overview.md)** - Core concepts and definitions
- **[configuration.md](./configuration.md)** - Deployment schema and vendor configuration
- **[cli-commands.md](./cli-commands.md)** - Complete command reference
