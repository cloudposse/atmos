# PRD: Metadata Inheritance

## Status

**Implemented**

## Problem Statement

### Current Behavior

The `metadata` section in Atmos component configurations is **not inherited** from base components. This is explicitly documented in the codebase:

```go
// Base component metadata.
// This is per component, not deep-merged and not inherited from base components and globals.
```

When a component uses `metadata.inherits` to inherit configuration from a base component, the following sections ARE inherited and deep-merged:
- `vars`
- `settings`
- `env`
- `backend`
- `providers`
- `hooks`

But `metadata` is excluded entirely - each component must define its own metadata.

### Problems This Causes

#### 1. Versioned Component State Management

When using versioned component folders (a recommended pattern for component versioning), users must manually configure `workspace_key_prefix` to prevent Terraform state loss during version upgrades:

```yaml
# components/terraform/vpc/v1/
# components/terraform/vpc/v2/

vpc-prod:
  metadata:
    component: vpc/v2  # Points to versioned folder
  backend:
    s3:
      workspace_key_prefix: vpc  # MUST be set manually to avoid state loss!
```

If `workspace_key_prefix` is not explicitly set, Atmos auto-generates it from `metadata.component`, resulting in `vpc-v2`. When upgrading to `vpc/v3`, the key becomes `vpc-v3`, causing Terraform to look for state in a different location - effectively losing the existing state.

**This is a silent, catastrophic failure mode.**

#### 2. No DRY Pattern for Metadata

Users cannot define metadata patterns once in a base component and have derived components inherit them:

```yaml
# Cannot do this today:
vpc/defaults:
  metadata:
    type: abstract
    terraform_workspace_pattern: "{tenant}-{environment}-{stage}"
    locked: true

vpc-prod:
  metadata:
    inherits: [vpc/defaults]
  # Does NOT inherit terraform_workspace_pattern or locked
  # Must repeat in every component
```

#### 3. Inconsistent Mental Model

Users expect inheritance to work consistently across all configuration sections. Having `metadata` as a special case that doesn't inherit creates confusion and requires explicit documentation of the exception.

## Proposed Solution

### Overview

1. Add new configuration option `stacks.inherit.metadata` to control metadata inheritance behavior
2. Make metadata inheritable by default (with one exception: `metadata.inherits` itself)
3. Add new `metadata.name` field for logical component identity
4. Use `metadata.name` for `workspace_key_prefix` auto-generation when available

### Configuration

Add a new `inherit` section to the `stacks` configuration:

```yaml
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  inherit:
    metadata: true  # NEW - default: true
```

To disable metadata inheritance (for backwards compatibility):

```yaml
stacks:
  inherit:
    metadata: false
```

### Inheritance Behavior When Enabled

When `stacks.inherit.metadata: true` (the default):

| Field | Inherited? | Notes |
|-------|-----------|-------|
| `metadata.type` | **Partial** | Inherited UNLESS value is `abstract` (base component designation) |
| `metadata.enabled` | Yes | Can override in derived component |
| `metadata.component` | Yes | Can override in derived component |
| `metadata.name` | Yes | NEW field - logical component identity |
| `metadata.terraform_workspace` | Yes | Can override in derived component |
| `metadata.terraform_workspace_pattern` | Yes | Can override in derived component |
| `metadata.locked` | Yes | Can override in derived component |
| `metadata.custom` | Yes | Deep-merged like other sections |
| `metadata.inherits` | **No** | Meta-property - defines what to inherit FROM |

**Special exclusions from inheritance:**
- `metadata.inherits` - This is the meta-property that defines inheritance relationships. Inheriting it would create confusing circular dependencies.
- `metadata.type: abstract` - The `abstract` type is a base component designation that indicates the component is not deployable. This should not be inherited by child components, which are concrete implementations. Other `type` values (e.g., `real`, custom types) ARE inherited normally.

### New Field: `metadata.name`

Add a new metadata field `name` that represents the **logical component identity**:

```yaml
vpc/defaults:
  metadata:
    type: abstract
    name: vpc           # Logical identity
    component: vpc/v2   # Physical path to Terraform code
```

**Purpose**: The `name` field provides a stable identifier for the component that:
1. Is used for `workspace_key_prefix` auto-generation
2. Remains constant across version changes
3. Can be inherited from base components

### workspace_key_prefix Auto-Generation

Update the auto-generation logic for `workspace_key_prefix` (and equivalent fields for GCS/Azure):

**Current logic:**
```go
workspaceKeyPrefix := component           // Atmos component name
if baseComponentName != "" {
    workspaceKeyPrefix = baseComponentName  // metadata.component overrides
}
```

**New logic:**
```go
workspaceKeyPrefix := component           // Atmos component name
if metadataName != "" {
    workspaceKeyPrefix = metadataName     // metadata.name takes priority
} else if baseComponentName != "" {
    workspaceKeyPrefix = baseComponentName  // metadata.component fallback
}
```

**Priority order for workspace_key_prefix:**
1. Explicit `backend.s3.workspace_key_prefix` (user override)
2. `metadata.name` (logical identity)
3. `metadata.component` (physical path)
4. Atmos component name (YAML key)

## Use Cases

### Use Case 1: Versioned Components

```yaml
# catalog/vpc-defaults.yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
        name: vpc           # Logical identity - stable across versions
        component: vpc/v2   # Current version

# stacks/prod/us-east-1.yaml
components:
  terraform:
    vpc-prod:
      metadata:
        inherits:
          - vpc/defaults
      vars:
        cidr_block: "10.0.0.0/16"
```

**Result:**
- `vpc-prod` inherits `name: vpc` and `component: vpc/v2`
- `workspace_key_prefix` auto-generates as `vpc` (from `metadata.name`)
- Upgrading to `vpc/v3` requires changing only `vpc/defaults` - all derived components follow
- State remains at `vpc/` path - no migration needed

### Use Case 2: Release Tracks

```yaml
# catalog/vpc-stable.yaml
components:
  terraform:
    vpc/stable:
      metadata:
        type: abstract
        name: vpc
        component: stable/vpc  # Track-centric layout

# catalog/vpc-beta.yaml
components:
  terraform:
    vpc/beta:
      metadata:
        type: abstract
        name: vpc
        component: beta/vpc

# stacks/dev.yaml - uses beta track
components:
  terraform:
    vpc-dev:
      metadata:
        inherits:
          - vpc/beta
```

### Use Case 3: Governance Patterns

```yaml
# catalog/production-defaults.yaml
components:
  terraform:
    production/defaults:
      metadata:
        type: abstract
        locked: true
        terraform_workspace_pattern: "prod-{tenant}-{environment}-{stage}"

# All production components inherit locked: true
vpc-prod:
  metadata:
    inherits:
      - production/defaults
      - vpc/defaults
```

### Use Case 4: Multiple Instances of Same Component

```yaml
# catalog/vpc-defaults.yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
        component: vpc  # Points to single Terraform component

# stacks/prod.yaml
components:
  terraform:
    vpc/primary:
      metadata:
        name: vpc-primary    # Different logical identity
        inherits:
          - vpc/defaults
      vars:
        cidr_block: "10.0.0.0/16"

    vpc/secondary:
      metadata:
        name: vpc-secondary  # Different logical identity
        inherits:
          - vpc/defaults
      vars:
        cidr_block: "10.1.0.0/16"
```

**Result:**
- Both use same Terraform code (`component: vpc`)
- Different state paths (`vpc-primary/`, `vpc-secondary/`)
- Shared configuration from `vpc/defaults`

## Breaking Change

### What Changes

When `stacks.inherit.metadata: true` (the new default):
- Metadata fields will be inherited from base components
- Components that previously had isolated metadata will now inherit from their base

### Impact Assessment

**Low risk of breakage** because:
1. Most users don't set metadata in abstract base components (since it wasn't inherited before)
2. Derived components can override any inherited value
3. The most common metadata fields (`component`, `inherits`) are typically set per-component anyway
4. `metadata.type: abstract` is automatically excluded from inheritance to prevent child components from becoming abstract

**Potential issues:**
- A base component with `enabled: false` would disable derived components (if not overridden)
- A base component with `locked: true` would lock derived components (if not overridden)

### Migration Path

For users who experience issues:

```yaml
# Disable metadata inheritance to restore previous behavior
stacks:
  inherit:
    metadata: false
```

Or override specific fields in derived components:

```yaml
vpc-prod:
  metadata:
    inherits:
      - vpc/defaults
    enabled: true   # Override inherited enabled
    locked: false   # Override inherited locked
```

## Implementation

### Schema Changes

1. Add `Inherit` struct to `Stacks` in `pkg/schema/schema.go`:
   ```go
   type Stacks struct {
       BasePath      string   `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
       IncludedPaths []string `yaml:"included_paths" json:"included_paths" mapstructure:"included_paths"`
       ExcludedPaths []string `yaml:"excluded_paths" json:"excluded_paths" mapstructure:"excluded_paths"`
       NamePattern   string   `yaml:"name_pattern" json:"name_pattern" mapstructure:"name_pattern"`
       NameTemplate  string   `yaml:"name_template" json:"name_template" mapstructure:"name_template"`
       Inherit       StacksInherit `yaml:"inherit" json:"inherit" mapstructure:"inherit"`
   }

   type StacksInherit struct {
       Metadata *bool `yaml:"metadata" json:"metadata" mapstructure:"metadata"`
   }
   ```

2. Add `Name` to metadata handling (stored in `map[string]any`)

### Stack Processing Changes

1. Modify `internal/exec/stack_processor_utils.go`:
   - Check `atmosConfig.Stacks.Inherit.Metadata` setting
   - When true, include metadata in inheritance processing
   - Exclude `metadata.inherits` from being inherited.

2. Modify `internal/exec/stack_processor_merge.go`:
   - Add metadata merging logic (similar to vars/settings)
   - Base metadata + Component metadata with override

3. Modify `internal/exec/stack_processor_backend.go`:
   - Update `setS3BackendDefaults()` to check `metadata.name`
   - Update `setGCSBackendDefaults()` similarly
   - Update `setAzureBackendKey()` similarly

### JSON Schema Changes

Update `pkg/datafetcher/schema/`:
1. Add `inherit` to stacks schema
2. Add `name` to component metadata schema

### Documentation Changes

1. Update version management docs to show `metadata.name` pattern
2. Update component inheritance docs to reflect metadata inheritance
3. Add migration guide for the breaking change
4. Fix incorrect `settings.workspace_key_prefix` examples (should be `backend.s3.workspace_key_prefix`)

## Future Extensions

The `stacks.inherit` section provides a foundation for future inheritance controls:

```yaml
stacks:
  inherit:
    metadata: true
    # Potential future options:
    # vars: true          # Already true by default
    # settings: true      # Already true by default
    # env: true           # Already true by default
    # backend: true       # Already true by default
```

This could allow users to disable inheritance of specific sections if needed, though current use cases don't require this.

## Success Criteria

1. Users can define `metadata.name` in base components and have it inherited
2. `workspace_key_prefix` auto-generates correctly from `metadata.name`
3. Versioned component upgrades don't require state migration when `metadata.name` is used
4. Existing configurations continue to work (with documented behavior change)
5. Users can opt-out via `stacks.inherit.metadata: false`

## References

- [Component Inheritance Documentation](https://atmos.tools/core-concepts/stacks/inheritance)
- [Terraform Backends Documentation](https://atmos.tools/core-concepts/components/terraform/backends)
- [Version Management Design Patterns](https://atmos.tools/design-patterns/version-management)
- Related code: `internal/exec/stack_processor_process_stacks_helpers_inheritance.go`
