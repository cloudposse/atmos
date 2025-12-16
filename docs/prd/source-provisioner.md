# PRD: Source Provisioner (Just-in-Time Vendoring)

**Status:** Draft
**Version:** 1.1
**Last Updated:** 2025-12-16
**Author:** Claude Code

---

## Executive Summary

The Source Provisioner enables just-in-time (JIT) vendoring of component sources directly from stack configuration via `metadata.source`. This allows components to declare their source location inline without requiring a separate `component.yaml` or `vendor.yaml` file, streamlining component reuse and reducing configuration overhead.

**Key Principle:** Components should be self-describing - the source location is metadata about the component, just like `metadata.component` defines the base component path.

---

## Overview

### Problem Statement

Currently, vendoring in Atmos requires one of:
1. A `vendor.yaml` manifest file listing sources
2. A `component.yaml` file in the component directory with vendor spec
3. Manual component placement in the filesystem

This creates friction for:
- **Quick prototyping** - needing to set up vendor configuration
- **Component reuse** - sharing components across projects
- **Monorepo migrations** - transitioning to Atmos

### Solution

Add `metadata.source` to component configuration, enabling inline source declaration:

```yaml
# Simple form - go-getter compatible string
components:
  terraform:
    vpc:
      metadata:
        source: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=1.2.3"

# Map form - full vendor spec
components:
  terraform:
    vpc:
      metadata:
        source:
          uri: "github.com/cloudposse/terraform-aws-components//modules/vpc"
          version: "1.2.3"
          included_paths:
            - "**/*.tf"
          excluded_paths:
            - "**/*_test.go"
```

### How It Works

1. **Source Provisioner** registers for `before.terraform.init` hook event
2. When triggered, checks if `metadata.source` is defined
3. If source exists and component directory is missing (or outdated), vendors the component
4. Vendors to component directory (or `workdir` when that feature merges)
5. Terraform execution proceeds with vendored component

### Integration with Workdir

After the workdir PR (#1852) merges:
- `workdir` takes precedence over source provisioner destination
- If `workdir` is specified, source is vendored to that location
- If `workdir` is not specified, source is vendored to default component path

**Precedence (highest to lowest):**
1. `working_directory` (explicit execution location)
2. Local component directory (if exists)
3. `metadata.source` (JIT vendor if component missing)

---

## CLI Commands (CRUD Operations)

Following the backend provisioner pattern, the source provisioner provides explicit CLI commands for managing vendored sources. **All commands are scoped by component type** for consistency with the existing CLI hierarchy.

### Terraform Source Commands

```bash
# CRUD operations
atmos terraform source create <component> --stack <stack>
atmos terraform source update <component> --stack <stack>
atmos terraform source list --stack <stack>
atmos terraform source describe <component> --stack <stack>
atmos terraform source delete <component> --stack <stack> --force

# Cache management (operates on shared cache, but scoped under terraform for CLI consistency)
atmos terraform source cache list
atmos terraform source cache prune --older-than 30d
atmos terraform source cache clear
atmos terraform source cache refresh <uri>
```

### Helmfile Source Commands

```bash
# CRUD operations
atmos helmfile source create <component> --stack <stack>
atmos helmfile source update <component> --stack <stack>
atmos helmfile source list --stack <stack>
atmos helmfile source describe <component> --stack <stack>
atmos helmfile source delete <component> --stack <stack> --force

# Cache management
atmos helmfile source cache list
atmos helmfile source cache prune --older-than 30d
atmos helmfile source cache clear
atmos helmfile source cache refresh <uri>
```

### Packer Source Commands (Future)

```bash
atmos packer source create <component> --stack <stack>
atmos packer source update <component> --stack <stack>
# etc.
```

### Why Component-Type Scoped?

All Atmos commands follow the pattern `atmos <type> <command>`:
- `atmos terraform plan`
- `atmos terraform backend create`
- `atmos helmfile sync`

Source commands follow the same pattern for consistency:
- `atmos terraform source create`
- `atmos helmfile source list`

**Note:** While the cache is shared infrastructure under the hood (all component types share the same XDG cache directory), the CLI commands are scoped per-type for consistency. Each component type that wants source provisioning must implement the `SourceProvider` interface.

---

## Component Registry Integration

### Optional SourceProvider Interface

The source provisioner is implemented as an **optional interface** in the component registry. Component types that want to support source provisioning implement this interface.

```go
// pkg/component/source_provider.go

// SourceProvider is an optional interface that component providers can implement
// to enable source provisioning (JIT vendoring) for their component type.
//
// Component providers that do NOT implement this interface will not have
// source commands available (e.g., `atmos <type> source create` will not exist).
type SourceProvider interface {
    // SourceCreate vendors a component from metadata.source.
    SourceCreate(ctx context.Context, atmosConfig *schema.AtmosConfiguration, component, stack string, force bool) error

    // SourceUpdate re-vendors a component (force refresh).
    SourceUpdate(ctx context.Context, atmosConfig *schema.AtmosConfiguration, component, stack string) error

    // SourceList returns all components with metadata.source in a stack.
    SourceList(ctx context.Context, atmosConfig *schema.AtmosConfiguration, stack string) ([]SourceInfo, error)

    // SourceDescribe returns source configuration for a component.
    SourceDescribe(ctx context.Context, atmosConfig *schema.AtmosConfiguration, component, stack string) (*SourceInfo, error)

    // SourceDelete removes a vendored source (requires force=true).
    SourceDelete(ctx context.Context, atmosConfig *schema.AtmosConfiguration, component, stack string, force bool) error

    // SourceCacheList returns cached repositories.
    SourceCacheList(ctx context.Context, atmosConfig *schema.AtmosConfiguration) ([]CacheEntry, error)

    // SourceCachePrune removes old worktrees from the cache.
    SourceCachePrune(ctx context.Context, atmosConfig *schema.AtmosConfiguration, olderThan time.Duration) error

    // SourceCacheClear clears the entire source cache.
    SourceCacheClear(ctx context.Context, atmosConfig *schema.AtmosConfiguration) error

    // SourceCacheRefresh force-refreshes a specific cached repository.
    SourceCacheRefresh(ctx context.Context, atmosConfig *schema.AtmosConfiguration, uri string) error

    // GetSourceHookEvent returns the hook event that triggers source provisioning.
    // Example: hooks.BeforeTerraformInit, hooks.BeforeHelmfileSync
    GetSourceHookEvent() hooks.HookEvent
}

// SourceInfo contains information about a component's source configuration.
type SourceInfo struct {
    Component     string
    Stack         string
    Source        *schema.VendorComponentSource
    TargetPath    string
    CachedAt      time.Time
    IsVendored    bool
}

// CacheEntry represents a cached repository.
type CacheEntry struct {
    URI         string
    Path        string
    Worktrees   []WorktreeInfo
    LastFetched time.Time
    Size        int64
}

// WorktreeInfo represents a git worktree in the cache.
type WorktreeInfo struct {
    Version   string
    Subpath   string
    Path      string
    CreatedAt time.Time
}
```

### Checking for SourceProvider Support

```go
// pkg/component/source.go

// GetSourceProvider returns the SourceProvider for a component type, if implemented.
// Returns nil, false if the component type does not support source provisioning.
func GetSourceProvider(componentType string) (SourceProvider, bool) {
    provider, ok := GetProvider(componentType)
    if !ok {
        return nil, false
    }

    sourceProvider, ok := provider.(SourceProvider)
    return sourceProvider, ok
}

// SupportsSource returns true if the component type supports source provisioning.
func SupportsSource(componentType string) bool {
    _, ok := GetSourceProvider(componentType)
    return ok
}
```

### Command Registration

Commands are only registered for component types that implement `SourceProvider`:

```go
// cmd/terraform/source.go

func init() {
    // Only register source commands if terraform provider implements SourceProvider
    if component.SupportsSource("terraform") {
        terraformCmd.AddCommand(sourceCmd)
    }
}

var sourceCmd = &cobra.Command{
    Use:   "source",
    Short: "Manage terraform component sources (JIT vendoring)",
    Long: `Manage terraform component sources defined in metadata.source.

Commands:
  create    Vendor component source
  update    Re-vendor component source (force refresh)
  list      List sources in a stack
  describe  Show source configuration
  delete    Remove vendored source
  cache     Manage source cache`,
}
```

### Default Implementation

A default `SourceProvider` implementation is provided that component types can embed:

```go
// pkg/provisioner/source/default_provider.go

// DefaultSourceProvider provides a default implementation of SourceProvider
// that component types can embed to get source provisioning support.
type DefaultSourceProvider struct {
    componentType string
    hookEvent     hooks.HookEvent
}

// NewDefaultSourceProvider creates a new DefaultSourceProvider.
func NewDefaultSourceProvider(componentType string, hookEvent hooks.HookEvent) *DefaultSourceProvider {
    return &DefaultSourceProvider{
        componentType: componentType,
        hookEvent:     hookEvent,
    }
}

// SourceCreate implements SourceProvider.
func (p *DefaultSourceProvider) SourceCreate(ctx context.Context, atmosConfig *schema.AtmosConfiguration, component, stack string, force bool) error {
    return Create(ctx, atmosConfig, p.componentType, component, stack, force)
}

// ... other methods delegate to pkg/provisioner/source functions
```

### Terraform Provider Example

```go
// pkg/component/terraform/provider.go

type TerraformProvider struct {
    *source.DefaultSourceProvider // Embed default source provider
}

func NewTerraformProvider() *TerraformProvider {
    return &TerraformProvider{
        DefaultSourceProvider: source.NewDefaultSourceProvider(
            "terraform",
            hooks.BeforeTerraformInit,
        ),
    }
}

// GetType implements ComponentProvider.
func (p *TerraformProvider) GetType() string {
    return "terraform"
}

// ... other ComponentProvider methods

// Note: SourceProvider methods are provided by embedded DefaultSourceProvider
```

---

## When to Use CLI vs Automatic

**Automatic (via hooks):**
```bash
# Source vendored automatically if metadata.source defined and component missing
atmos terraform apply vpc --stack dev
# → BeforeTerraformInit hook triggers
# → Source provisioner checks metadata.source
# → Vendors if component directory missing
# → Terraform runs
```

**Manual (explicit commands):**
```bash
# Pre-vendor sources in CI/CD
atmos terraform source create vpc --stack dev

# Force re-vendor to get latest
atmos terraform source update vpc --stack dev

# Check what would be vendored
atmos terraform source list --stack dev

# Manage cache
atmos terraform source cache list
atmos terraform source cache prune --older-than 30d
```

---

## Spinner UI Integration

The source provisioner uses the spinner architecture from `pkg/ui/spinner/` for visual feedback:

```go
// pkg/provisioner/source/source.go

import "github.com/cloudposse/atmos/pkg/ui/spinner"

func ProvisionSource(
    ctx context.Context,
    atmosConfig *schema.AtmosConfiguration,
    componentConfig map[string]any,
    authContext *schema.AuthContext,
) error {
    // ... setup ...

    // Use spinner for download operation
    err := spinner.ExecWithSpinnerDynamic(
        "Vendoring component",
        func() (string, error) {
            if err := vendorSource(ctx, atmosConfig, sourceSpec, targetDir, authContext); err != nil {
                return "", err
            }
            return fmt.Sprintf("Vendored `%s` → `%s`", sourceSpec.Uri, targetDir), nil
        },
    )
    if err != nil {
        return errUtils.Build(errUtils.ErrSourceProvision).
            WithCause(err).
            WithContext("source", sourceSpec.Uri).
            WithContext("target", targetDir).
            Err()
    }

    return nil
}
```

### Spinner Behavior

- **TTY available**: Shows animated spinner with progress message, then success/error on completion
- **No TTY (CI/CD)**: Shows completion message without animation
- **Dynamic messages**: Completion message includes actual source and target paths

---

## Schema Design

### metadata.source - String Form

When `metadata.source` is a string, it's interpreted as a go-getter compatible URI:

```yaml
metadata:
  source: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=1.2.3"
```

Equivalent to:
```yaml
metadata:
  source:
    uri: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=1.2.3"
```

### metadata.source - Map Form

When `metadata.source` is a map, it follows the existing `VendorComponentSource` schema exactly. **No new type is needed** - we reuse the existing schema:

```go
// pkg/schema/vendor_component.go (EXISTING - reused for metadata.source)

type VendorComponentSource struct {
    // Type is the source type (git, http, s3, oci, etc.)
    // Optional - auto-detected from URI if not specified
    Type string `yaml:"type" json:"type" mapstructure:"type"`

    // Uri is the source location (go-getter compatible)
    // Required field
    Uri string `yaml:"uri" json:"uri" mapstructure:"uri"`

    // Version is the version/tag/ref to vendor
    // Can use Go template variables: {{ .Version }}
    Version string `yaml:"version" json:"version" mapstructure:"version"`

    // IncludedPaths are glob patterns for files to include
    // Example: ["**/*.tf", "**/*.tfvars"]
    IncludedPaths []string `yaml:"included_paths" json:"included_paths" mapstructure:"included_paths"`

    // ExcludedPaths are glob patterns for files to exclude
    // Example: ["**/tests/**", "**/*.md"]
    ExcludedPaths []string `yaml:"excluded_paths" json:"excluded_paths" mapstructure:"excluded_paths"`
}
```

**Design Decision:** Reuse `VendorComponentSource` rather than creating a new type. This ensures consistency with `component.yaml` vendoring and reduces schema duplication.

### Full Configuration Example

```yaml
components:
  terraform:
    # Example 1: String form (most common)
    vpc:
      metadata:
        source: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=1.2.3"
      vars:
        vpc_cidr: "10.0.0.0/16"

    # Example 2: Map form with version
    eks:
      metadata:
        source:
          uri: "github.com/cloudposse/terraform-aws-components//modules/eks-cluster"
          version: "2.0.0"
      vars:
        cluster_name: "my-cluster"

    # Example 3: Map form with path filters
    rds:
      metadata:
        source:
          uri: "github.com/acme/internal-modules//databases/rds"
          version: "main"
          included_paths:
            - "**/*.tf"
            - "**/*.tfvars"
          excluded_paths:
            - "**/tests/**"
            - "**/*.md"
      vars:
        engine: "postgres"

    # Example 4: OCI registry
    app:
      metadata:
        source:
          type: "oci"
          uri: "oci://public.ecr.aws/cloudposse/components/terraform-aws-lambda"
          version: "1.5.0"

  helmfile:
    # Helmfile components also support metadata.source
    nginx:
      metadata:
        source: "github.com/cloudposse/helmfile-components//charts/nginx?ref=1.0.0"
```

---

## Architecture

### Source Provisioner Registration

Each component type registers its source provisioner for the appropriate hook event:

```go
// pkg/provisioner/source/terraform.go

func init() {
    // Terraform source provisioner runs before terraform.init
    err := provisioner.RegisterProvisioner(provisioner.Provisioner{
        Type:      "source-terraform",
        HookEvent: provisioner.HookEvent(hooks.BeforeTerraformInit),
        Func:      ProvisionTerraformSource,
    })
    if err != nil {
        panic(err)
    }
}

// pkg/provisioner/source/helmfile.go

func init() {
    // Helmfile source provisioner runs before helmfile.sync
    err := provisioner.RegisterProvisioner(provisioner.Provisioner{
        Type:      "source-helmfile",
        HookEvent: provisioner.HookEvent(hooks.BeforeHelmfileSync),
        Func:      ProvisionHelmfileSource,
    })
    if err != nil {
        panic(err)
    }
}
```

### Provisioner Flow

```text
Component Execution Request
  ↓
BeforeTerraformInit / BeforeHelmfileSync Hook Event
  ↓
ExecuteProvisioners() called
  ↓
Source Provisioner triggered
  ↓
Check metadata.source exists?
  ├─ No → Skip (return nil)
  └─ Yes → Continue
      ↓
Check component directory exists?
  ├─ Yes (and not stale) → Skip (return nil)
  └─ No (or stale) → Continue
      ↓
Resolve source (string or map form)
  ↓
Determine target directory
  ├─ workdir specified → Use workdir
  └─ No workdir → Use components/{type}/{component}
      ↓
Vendor source to target
  ↓
Return success
```

### Core Implementation

```go
// pkg/provisioner/source/source.go

func ProvisionSource(
    ctx context.Context,
    atmosConfig *schema.AtmosConfiguration,
    componentType string,
    componentConfig map[string]any,
    authContext *schema.AuthContext,
) error {
    defer perf.Track(atmosConfig, "provisioner.ProvisionSource")()

    // 1. Extract metadata.source
    source, err := extractMetadataSource(componentConfig)
    if err != nil {
        return nil // No source configured - skip silently
    }
    if source == nil {
        return nil
    }

    // 2. Resolve source spec (string → map form)
    sourceSpec, err := resolveSourceSpec(source)
    if err != nil {
        return errUtils.Build(errUtils.ErrSourceProvision).
            WithCause(err).
            WithExplanation("Failed to resolve source specification").
            WithHint("Check metadata.source format").
            Err()
    }

    // 3. Determine target directory
    targetDir, err := determineTargetDirectory(atmosConfig, componentType, componentConfig)
    if err != nil {
        return errUtils.Build(errUtils.ErrSourceProvision).
            WithCause(err).
            WithExplanation("Failed to determine target directory").
            Err()
    }

    // 4. Check if vendoring needed
    if !needsVendoring(targetDir, sourceSpec) {
        ui.Info(fmt.Sprintf("Component already vendored at %s", targetDir))
        return nil
    }

    // 5. Vendor source
    ui.Info(fmt.Sprintf("Vendoring component from %s to %s", sourceSpec.Uri, targetDir))
    if err := vendorSource(ctx, atmosConfig, sourceSpec, targetDir, authContext); err != nil {
        return errUtils.Build(errUtils.ErrSourceProvision).
            WithCause(err).
            WithExplanation("Failed to vendor component source").
            WithContext("source", sourceSpec.Uri).
            WithContext("target", targetDir).
            WithHint("Verify source URI is accessible and credentials are valid").
            Err()
    }

    ui.Success(fmt.Sprintf("Successfully vendored component to %s", targetDir))
    return nil
}
```

### Reusing Existing Vendor Infrastructure

The source provisioner reuses existing vendor utilities:

```go
// pkg/provisioner/source/vendor.go

import (
    "github.com/cloudposse/atmos/internal/exec"
    "github.com/cloudposse/atmos/pkg/downloader"
)

func vendorSource(
    ctx context.Context,
    atmosConfig *schema.AtmosConfiguration,
    sourceSpec *schema.VendorComponentSource,
    targetDir string,
    authContext *schema.AuthContext,
) error {
    // Reuse existing vendor component logic
    // This handles:
    // - URI normalization
    // - go-getter downloading
    // - OCI registry support
    // - Path filtering
    // - Template evaluation
    return exec.VendorComponentSource(
        atmosConfig,
        *sourceSpec,
        targetDir,
        authContext,
    )
}
```

---

## Precedence Rules

### Source Resolution Precedence

When determining component location:

1. **Local component exists** → Use local (no vendoring)
2. **workdir specified** → Vendor to workdir
3. **metadata.source defined** → Vendor to default path
4. **component.yaml exists** → Use existing vendor config (future: delegate to component provisioner)

### Configuration Inheritance

`metadata.source` follows standard Atmos deep-merge:

```yaml
# stacks/catalog/vpc/defaults.yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        source:
          uri: "github.com/cloudposse/terraform-aws-components//modules/vpc"
          version: "1.0.0"

# stacks/prod.yaml
components:
  terraform:
    vpc:
      metadata:
        inherits: [vpc/defaults]
        source:
          version: "1.2.0"  # Override version only
```

---

## Supported Source Types

All go-getter supported schemes work:

| Scheme | Example |
|--------|---------|
| `git` | `git::https://github.com/org/repo.git//path` |
| `github.com` | `github.com/org/repo//path?ref=v1.0.0` |
| `gitlab.com` | `gitlab.com/org/repo//path?ref=main` |
| `bitbucket.org` | `bitbucket.org/org/repo//path` |
| `s3` | `s3::https://bucket.s3.amazonaws.com/path` |
| `gcs` | `gcs::https://storage.googleapis.com/bucket/path` |
| `http/https` | `https://example.com/module.zip` |
| `oci` | `oci://registry.io/org/module:tag` |
| `file` | `file:///path/to/local/module` |

---

## Error Handling

### Sentinel Errors

```go
// errors/errors.go (additions)

var (
    // ErrSourceProvision indicates source provisioning failed.
    ErrSourceProvision = errors.New("source provisioning failed")

    // ErrSourceNotFound indicates the source could not be found.
    ErrSourceNotFound = errors.New("source not found")

    // ErrSourceAccessDenied indicates access to source was denied.
    ErrSourceAccessDenied = errors.New("source access denied")
)
```

### Error Examples

```text
Error: source provisioning failed: failed to vendor component source

  source: github.com/org/private-repo//modules/vpc?ref=v1.0.0
  target: components/terraform/vpc

Explanation: Source repository is private and requires authentication

Hint: Set GITHUB_TOKEN environment variable or configure git credentials
Hint: For private repos, use SSH: git::ssh://git@github.com/org/repo.git//path

Exit code: 3
```

---

## Configuration Options

### Global Settings (atmos.yaml)

```yaml
# atmos.yaml
provision:
  source:
    # Enable/disable source provisioning globally
    enabled: true

    # Force re-vendor even if directory exists
    force: false

    # Default timeout for download operations
    timeout: 5m
```

### Component-Level Settings

```yaml
components:
  terraform:
    vpc:
      provision:
        source:
          enabled: true    # Enable for this component
          force: false     # Force re-vendor
```

---

## Testing Strategy

### Unit Tests

```go
// pkg/provisioner/source/source_test.go

func TestExtractMetadataSource_StringForm(t *testing.T)
func TestExtractMetadataSource_MapForm(t *testing.T)
func TestExtractMetadataSource_NotPresent(t *testing.T)
func TestResolveSourceSpec_StringToMap(t *testing.T)
func TestResolveSourceSpec_MapPassthrough(t *testing.T)
func TestDetermineTargetDirectory_WithWorkdir(t *testing.T)
func TestDetermineTargetDirectory_DefaultPath(t *testing.T)
func TestNeedsVendoring_DirectoryMissing(t *testing.T)
func TestNeedsVendoring_DirectoryExists(t *testing.T)
func TestProvisionSource_SourceNotConfigured(t *testing.T)
func TestProvisionSource_AlreadyVendored(t *testing.T)
func TestProvisionSource_Success(t *testing.T)
func TestProvisionSource_VendorError(t *testing.T)
```

### Integration Tests

```go
// pkg/provisioner/source/source_integration_test.go

func TestSourceProvisioner_GitHubSource(t *testing.T)
func TestSourceProvisioner_OciSource(t *testing.T)
func TestSourceProvisioner_LocalFileSource(t *testing.T)
func TestSourceProvisioner_WorkdirIntegration(t *testing.T)
func TestSourceProvisioner_InheritanceChain(t *testing.T)
```

---

## Package Structure

Following the terraform registry migration pattern (PR #1813), the source subcommand is structured as a subpackage of `cmd/terraform/`:

```text
cmd/terraform/
  ├── terraform.go           # Main command (existing, adds source subcommand)
  ├── backend/               # Backend subpackage (existing pattern to follow)
  │   ├── backend.go         # GetBackendCommand()
  │   ├── backend_create.go
  │   ├── backend_list.go
  │   └── ...
  └── source/                # NEW: Source subpackage (follows backend pattern)
      ├── source.go          # GetSourceCommand(), sourceCmd parent
      ├── source_test.go     # Unit tests
      ├── create.go          # atmos terraform source create
      ├── update.go          # atmos terraform source update
      ├── list.go            # atmos terraform source list
      ├── describe.go        # atmos terraform source describe
      ├── delete.go          # atmos terraform source delete
      └── cache/             # Cache subcommand
          ├── cache.go       # GetCacheCommand(), cacheCmd parent
          ├── list.go        # atmos terraform source cache list
          ├── prune.go       # atmos terraform source cache prune
          ├── clear.go       # atmos terraform source cache clear
          └── refresh.go     # atmos terraform source cache refresh

cmd/helmfile/
  └── source/                # NEW: Helmfile source subpackage (same structure)
      ├── source.go
      ├── create.go
      └── ...

pkg/component/
  ├── provider.go            # ComponentProvider interface (existing)
  ├── source_provider.go     # NEW: Optional SourceProvider interface
  ├── registry.go            # Registry (existing)
  └── source.go              # NEW: GetSourceProvider, SupportsSource helpers

pkg/provisioner/
  ├── provisioner.go         # Core registry (existing)
  ├── registry.go            # Registration (existing)
  ├── backend/               # Backend provisioner (existing)
  └── source/                # NEW: Source provisioner business logic
      ├── source.go          # Main provisioner implementation
      ├── source_test.go     # Unit tests
      ├── extract.go         # metadata.source extraction
      ├── resolve.go         # Source spec resolution
      ├── vendor.go          # Vendor integration (go-getter fallback)
      ├── target.go          # Target directory logic
      ├── copy.go            # Fast copy using otiai10/copy
      ├── git_worktree.go    # Git worktree cache strategy (Phase 2)
      └── cache.go           # Cache management (Phase 2)

pkg/ui/spinner/              # Existing spinner UI
  └── spinner.go             # ExecWithSpinner, ExecWithSpinnerDynamic
```

### Integration with Terraform Command (terraform.go)

The source subcommand is registered in `cmd/terraform/terraform.go` following the backend pattern:

```go
// cmd/terraform/terraform.go (additions)

import (
    "github.com/cloudposse/atmos/cmd/terraform/source"
)

func init() {
    // ... existing code ...

    // Add generate subcommand from the generate subpackage.
    terraformCmd.AddCommand(generate.GenerateCmd)

    // Add backend subcommand from the backend subpackage.
    terraformCmd.AddCommand(backend.GetBackendCommand())

    // Add source subcommand from the source subpackage.
    terraformCmd.AddCommand(source.GetSourceCommand())

    // ... rest of init ...
}
```

---

## Implementation Plan

### Phase 1: Core Implementation
1. Add `SourceProvider` optional interface to component registry
2. Add `MetadataSource` schema type
3. Implement source provisioner registration
4. Implement metadata.source extraction (string and map forms)
5. Implement target directory resolution
6. Wire up existing vendor utilities

### Phase 2: Integration
1. Add sentinel errors
2. Integrate with terraform.go execution flow
3. Add configuration options (atmos.yaml)
4. Handle authentication (authContext passthrough)
5. Add helmfile source provisioner

### Phase 3: Workdir Integration
1. Detect workdir configuration
2. Prioritize workdir over default paths
3. Handle precedence rules

### Phase 4: Testing & Documentation
1. Unit tests for all functions
2. Integration tests for common scenarios
3. Update CLI documentation
4. Update website docs

---

## Critical Files to Modify

### New Files to Create

**CLI Commands (following backend subpackage pattern from PR #1813):**

1. **`cmd/terraform/source/source.go`** - Source subcommand parent, GetSourceCommand()
2. **`cmd/terraform/source/create.go`** - atmos terraform source create
3. **`cmd/terraform/source/update.go`** - atmos terraform source update
4. **`cmd/terraform/source/list.go`** - atmos terraform source list
5. **`cmd/terraform/source/describe.go`** - atmos terraform source describe
6. **`cmd/terraform/source/delete.go`** - atmos terraform source delete
7. **`cmd/terraform/source/cache/cache.go`** - Cache subcommand parent, GetCacheCommand()
8. **`cmd/terraform/source/cache/list.go`** - atmos terraform source cache list
9. **`cmd/terraform/source/cache/prune.go`** - atmos terraform source cache prune
10. **`cmd/terraform/source/cache/clear.go`** - atmos terraform source cache clear
11. **`cmd/terraform/source/cache/refresh.go`** - atmos terraform source cache refresh

**Helmfile (same structure):**

12. **`cmd/helmfile/source/source.go`** - Source subcommand for helmfile
13. **`cmd/helmfile/source/...`** - Same structure as terraform

**Component Registry Interface:**

14. **`pkg/component/source_provider.go`** - Optional SourceProvider interface
15. **`pkg/component/source.go`** - GetSourceProvider, SupportsSource helpers

**Source Provisioner Business Logic:**

16. **`pkg/provisioner/source/source.go`** - Main provisioner implementation
17. **`pkg/provisioner/source/source_test.go`** - Unit tests
18. **`pkg/provisioner/source/extract.go`** - metadata.source extraction
19. **`pkg/provisioner/source/resolve.go`** - Source spec resolution (string→map)
20. **`pkg/provisioner/source/vendor.go`** - Vendor integration (go-getter fallback)
21. **`pkg/provisioner/source/target.go`** - Target directory logic
22. **`pkg/provisioner/source/copy.go`** - Fast copy using otiai10/copy
23. **`pkg/provisioner/source/git_worktree.go`** - Git worktree cache strategy (Phase 2)
24. **`pkg/provisioner/source/cache.go`** - Cache management (Phase 2)

### Existing Files to Modify

1. **`cmd/terraform/terraform.go`** - Add `terraformCmd.AddCommand(source.GetSourceCommand())`
2. **`cmd/helmfile/helmfile.go`** - Add `helmfileCmd.AddCommand(source.GetSourceCommand())`
3. **`errors/errors.go`** - Add sentinel errors (ErrSourceProvision, ErrSourceNotFound, ErrSourceAccessDenied)
4. **`pkg/datafetcher/schema/`** - Update JSON schemas for metadata.source

**Note:** We reuse `VendorComponentSource` from `pkg/schema/vendor_component.go` - no schema changes needed.

---

## Caching and Performance Strategy

### The Problem

Caching is critical for developer experience. As Knuth noted, it's also one of the hardest problems in computer science. The current go-getter implementation:

- Downloads fresh copies on every vendor operation
- No deduplication across components using the same source
- No incremental updates (full re-download even for minor version bumps)
- Network-bound performance

### Proposed Solution: Git Clone + Worktrees

**Concept:** Use native Git operations instead of go-getter for Git sources. Clone the repository once to a cache directory, then use Git worktrees to check out specific versions.

**Why Worktrees?**
- **Instant checkouts**: Worktrees share the Git object database - checking out a new version is nearly instant
- **Disk efficient**: Only one copy of the repo's object database, regardless of how many versions are checked out
- **Native Git**: Leverages Git's highly optimized internals
- **Incremental fetches**: `git fetch` only downloads new objects

### Architecture

The cache uses the XDG Base Directory Specification via `pkg/xdg/xdg.go`:

```go
// Uses existing XDG infrastructure, scoped by component type
cacheDir, err := xdg.GetXDGCacheDir("sources/terraform", 0o755)
// Returns: ~/.cache/atmos/sources/terraform (Linux/macOS CLI convention)
// Respects: ATMOS_XDG_CACHE_HOME > XDG_CACHE_HOME > default
```

**Cache Directory Structure:**

The cache is organized by component type to avoid conflicts and provide clear separation:

```text
$XDG_CACHE_HOME/atmos/     # ~/.cache/atmos/ by default
  └── sources/
      ├── terraform/                              # Terraform component sources
      │   └── git/
      │       └── github.com/
      │           └── cloudposse/
      │               └── terraform-aws-components/
      │                   ├── .git/               # Bare clone (shared objects)
      │                   └── worktrees/
      │                       ├── v1.2.3--modules-vpc/
      │                       ├── v1.2.3--modules-eks/
      │                       └── v1.3.0--modules-vpc/
      │
      ├── helmfile/                               # Helmfile component sources
      │   └── git/
      │       └── github.com/
      │           └── cloudposse/
      │               └── helmfile-components/
      │                   ├── .git/
      │                   └── worktrees/
      │                       └── v1.0.0--charts-nginx/
      │
      └── packer/                                 # Packer component sources (future)
          └── git/
              └── ...
```

**Why separate by component type?**
- Clear ownership: `atmos terraform source cache clear` only clears terraform cache
- No conflicts: Different component types can use same repo URLs without collision
- Easier debugging: Cache location matches CLI command hierarchy

**Environment Variable Precedence:**
1. `ATMOS_XDG_CACHE_HOME` - Atmos-specific override
2. `XDG_CACHE_HOME` - Standard XDG variable
3. Default: `~/.cache` (Linux/macOS CLI convention)

### Source Type: `git-worktree`

```yaml
# New source type for git-worktree caching
components:
  terraform:
    vpc:
      metadata:
        source:
          type: git-worktree           # NEW: Use git clone + worktree strategy
          uri: "github.com/cloudposse/terraform-aws-components"
          path: "modules/vpc"          # Subdirectory within repo
          version: "1.2.3"             # Tag, branch, or commit SHA
```

### Implementation Flow

```text
1. Check cache exists? (component-type scoped)
   └─ $XDG_CACHE_HOME/atmos/sources/terraform/git/github.com/cloudposse/terraform-aws-components/.git

2. If no cache:
   └─ git clone --bare <uri> <cache-path>

3. Fetch latest refs:
   └─ git fetch --all --tags (only if stale or forced)

4. Check worktree exists for version+path combo?
   └─ worktrees/v1.2.3--modules-vpc/

5. If no worktree:
   a. git worktree add --no-checkout worktrees/v1.2.3--modules-vpc v1.2.3
   b. cd worktrees/v1.2.3--modules-vpc
   c. git sparse-checkout set --cone modules/vpc   # Only checkout subpath!
   d. git checkout

6. Copy to component directory (using otiai10/copy):
   └─ cp.Copy(worktrees/v1.2.3--modules-vpc/modules/vpc, components/terraform/vpc)
```

### Subpath Support via Sparse Checkout

Git worktrees don't natively support subpaths, but **sparse-checkout** (cone mode) solves this elegantly:

```bash
# Create worktree WITHOUT checking out files
git worktree add --no-checkout worktrees/v1.2.3--modules-vpc v1.2.3

# Configure sparse-checkout for just the subpath (cone mode = directory-based, fast)
cd worktrees/v1.2.3--modules-vpc
git sparse-checkout set --cone modules/vpc

# Now checkout only materializes modules/vpc/* to disk
git checkout
```

**Why Sparse Checkout + Cone Mode?**
- **Per-worktree config**: Each worktree can have different sparse patterns (via `extensions.worktreeConfig`)
- **Cone mode performance**: Optimized for directory patterns, faster than gitignore-style patterns
- **Minimal disk usage**: Only requested paths are materialized
- **Native Git**: No custom tooling, uses Git's optimized internals

**Worktree Naming Convention:**
Since the same version might need different subpaths for different components, we include the path in the worktree name:
```
worktrees/v1.2.3--modules-vpc/      # vpc component at v1.2.3
worktrees/v1.2.3--modules-eks/      # eks component at v1.2.3 (same version, different path)
worktrees/v1.3.0--modules-vpc/      # vpc component at v1.3.0
```

Sources: [Git sparse-checkout docs](https://git-scm.com/docs/git-sparse-checkout), [Git worktree docs](https://git-scm.com/docs/git-worktree)

### Performance Comparison

| Operation | go-getter | git-worktree |
|-----------|-----------|--------------|
| First clone (cold cache) | ~30s | ~30s |
| Same version (warm cache) | ~30s (re-download) | <1s (exists check) |
| New version (warm cache) | ~30s (full download) | ~2s (fetch + worktree add) |
| Multiple components, same repo | N × 30s | ~30s + N × <1s |

### GitWorktreeSource Schema

```go
// pkg/provisioner/source/git_worktree.go

type GitWorktreeSource struct {
    // Uri is the Git repository URL (without subpath)
    Uri string `yaml:"uri" json:"uri" mapstructure:"uri"`

    // Path is the subdirectory within the repo to vendor
    Path string `yaml:"path" json:"path" mapstructure:"path"`

    // Version is the tag, branch, or commit SHA
    Version string `yaml:"version" json:"version" mapstructure:"version"`

    // IncludedPaths are glob patterns for files to include (within Path)
    IncludedPaths []string `yaml:"included_paths,omitempty" json:"included_paths,omitempty" mapstructure:"included_paths"`

    // ExcludedPaths are glob patterns for files to exclude (within Path)
    ExcludedPaths []string `yaml:"excluded_paths,omitempty" json:"excluded_paths,omitempty" mapstructure:"excluded_paths"`
}
```

### Cache Management

```yaml
# atmos.yaml
settings:
  cache:
    sources:
      # Path defaults to XDG cache: $XDG_CACHE_HOME/atmos/sources
      # Override with explicit path if needed:
      # path: /custom/cache/path
      ttl: 24h                         # How long before auto-fetch
      max_size: 10GB                   # Max cache size (LRU eviction)
```

**XDG Integration:**

```go
// pkg/provisioner/source/cache.go

import "github.com/cloudposse/atmos/pkg/xdg"

func getCacheDir(componentType string) (string, error) {
    // Uses XDG Base Directory Specification
    // Precedence: ATMOS_XDG_CACHE_HOME > XDG_CACHE_HOME > ~/.cache
    subpath := filepath.Join("sources", componentType)
    return xdg.GetXDGCacheDir(subpath, 0o755)
    // Returns: ~/.cache/atmos/sources/terraform (for terraform)
    // Returns: ~/.cache/atmos/sources/helmfile (for helmfile)
}
```

### Copy Strategy (No Symlinks)

**Important:** Symlinks are explicitly avoided because they don't work with concurrency - multiple component instances may write to the same folder simultaneously.

**Copy Implementation:**

The source provisioner uses a fast copy strategy for all operations, leveraging the existing `otiai10/copy` library already used in the vendor system:

```go
// pkg/provisioner/source/copy.go

import cp "github.com/otiai10/copy"

func copyFromCache(srcPath, dstPath string, opts *CopyOptions) error {
    // Fast copy using otiai10/copy (already in codebase)
    return cp.Copy(srcPath, dstPath, cp.Options{
        // Preserve permissions
        PermissionControl: cp.AddPermission(0),
        // Skip symlinks in source
        OnSymlink: func(src string) cp.SymlinkAction {
            return cp.Skip
        },
        // Skip .git directories
        Skip: func(info os.FileInfo, src, dest string) (bool, error) {
            return info.Name() == ".git", nil
        },
    })
}
```

**Why `otiai10/copy`?**
- Already used in Atmos vendor system (`internal/exec/copy_glob.go`)
- Proven fast and reliable in production
- Supports permission preservation
- Supports filtering (skip .git, symlinks)
- No additional dependencies

**Performance Characteristics:**
- Uses `io.Copy` with buffered I/O (efficient for large files)
- Directory structure created on-the-fly
- Concurrent-safe: each component gets its own copy
- No file locking issues

**When Copying Occurs:**

| Scenario | Action |
|----------|--------|
| Cold cache | Clone → Worktree → Copy |
| Warm cache, new version | Fetch → Worktree → Copy |
| Warm cache, same version | Copy only (instant for small modules) |

**Concurrency Safety:**

```text
Component A (vpc v1.2.3) ──┬── Copy to components/terraform/vpc-a/
                           │
Component B (vpc v1.2.3) ──┴── Copy to components/terraform/vpc-b/
                           │
                           └── Both read from same cached worktree
                               No write conflicts!
```

Each component instance has its own target directory, avoiding the write conflicts that would occur with symlinks.

### Fallback to go-getter

For non-Git sources (S3, HTTP, OCI), fall back to go-getter:

```go
func vendorSource(source *VendorComponentSource) error {
    switch {
    case source.Type == "git-worktree":
        return vendorWithGitWorktree(source)
    case isGitSource(source.Uri) && cacheEnabled():
        // Auto-upgrade plain git to worktree strategy
        return vendorWithGitWorktree(source)
    default:
        // Fall back to go-getter for S3, HTTP, OCI, etc.
        return vendorWithGoGetter(source)
    }
}
```

### Implementation Phases

**Phase 1 (MVP):** Implement source provisioner with go-getter (current PRD scope)

**Phase 2 (Performance):** Add `git-worktree` type with cache
- Bare clone to cache directory
- Worktree management
- Basic cache TTL

**Phase 3 (Polish):** Cache management CLI + auto-upgrade
- `atmos terraform source cache` commands
- Auto-detect git sources and use worktree strategy
- LRU eviction, max size limits

### Out of Scope (Future)

- Distributed cache (shared across team via S3/GCS)
- Content-addressable cache (like Nix/Guix)
- Vendor lockfile with checksums

---

## Future Considerations

### Component Provisioner Relationship

The source provisioner handles `metadata.source`. A future component provisioner might:
- Handle `component.yaml` based vendoring
- Support more complex dependency resolution
- Cache vendored components

### Version Pinning

Consider adding lockfile support:
```yaml
# .atmos/source.lock.yaml
components:
  terraform:
    vpc:
      source: "github.com/cloudposse/terraform-aws-components//modules/vpc"
      version: "1.2.3"
      checksum: "sha256:abc123..."
```

### Caching

Vendored sources are cached following XDG specification:
```yaml
provision:
  source:
    cache:
      enabled: true
      # Uses XDG cache by default: $XDG_CACHE_HOME/atmos/sources
      ttl: 24h
```

---

## Success Metrics

- Source provisioner invocations per day
- Average vendor time (p50, p95)
- Error rate by source type
- Adoption rate (components using metadata.source)

---

## Related Documents

- **[Provisioner System](./provisioner-system.md)** - Generic provisioner infrastructure
- **[Backend Provisioner](./backend-provisioner.md)** - Backend provisioner reference (CRUD pattern to follow)
- **[Terraform Registry Migration](./terraform-registry-migration.md)** - Command structure pattern (PR #1813)
- **[Vendor URI Normalization](./vendor-uri-normalization.md)** - URI handling details
- **[Component Registry Pattern](./component-registry-pattern.md)** - Component provider pattern
- **[XDG Base Directory Specification](./xdg-base-directory-specification.md)** - Cache location conventions
