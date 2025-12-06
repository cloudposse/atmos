# PRD: Workspace Key Prefix Calculation with Logical Component Name

## Status

Implemented.

## Problem Statement

### Current Behavior

When `workspace_key_prefix` (S3), `prefix` (GCS), or `key` (Azure) is not explicitly set, Atmos auto-generates it using this logic:

```go
workspaceKeyPrefix := component           // Atmos component name (YAML key)
if baseComponentName != "" {
    workspaceKeyPrefix = baseComponentName  // metadata.component overrides
}
backend["workspace_key_prefix"] = strings.ReplaceAll(workspaceKeyPrefix, "/", "-")
```

**Translation:**
- If `metadata.component` is set, use it
- Otherwise, use the Atmos component name (the YAML key)
- Replace `/` with `-` for filesystem safety

### The Problem

When using versioned component folders, `metadata.component` includes the version path:

```yaml
vpc-prod:
  metadata:
    component: vpc/v2  # Points to versioned folder
```

**Result:** `workspace_key_prefix` = `vpc-v2`

When upgrading to `vpc/v3`:
- New `workspace_key_prefix` = `vpc-v3`
- Terraform looks for state at new path
- **Existing state is "lost"** (still exists but Terraform can't find it)

This is a silent, catastrophic failure that requires manual state migration to fix.

## Proposed Solution

### Overview

Introduce `metadata.name` as the **logical component identity** that takes priority in workspace key calculation. If not defined, fall back to current behavior (use `metadata.component` or Atmos component name).

### New Field: `metadata.name`

A new metadata field representing the stable, logical identity of a component:

```yaml
vpc-prod:
  metadata:
    name: vpc           # Logical identity (stable)
    component: vpc/v2   # Physical path (version-aware)
```

**Characteristics:**
- Optional field
- When set, used for `workspace_key_prefix` auto-generation
- Inheritable from base components (see metadata-inheritance.md)
- Represents "what this component IS" vs "where its code lives"

### Updated Calculation Logic

**New priority order:**

| Priority | Source | Example Value | Result |
|----------|--------|---------------|--------|
| 1 | Explicit backend config | `backend.s3.workspace_key_prefix: custom` | `custom` |
| 2 | `metadata.name` | `metadata.name: vpc` | `vpc` |
| 3 | `metadata.component` | `metadata.component: vpc/v2` | `vpc-v2` |
| 4 | Atmos component name | YAML key: `vpc-prod` | `vpc-prod` |

**Pseudocode:**

```go
func deriveWorkspaceKeyPrefix(component string, metadata map[string]any, backend map[string]any) string {
    // Priority 1: Explicit backend configuration
    if prefix, ok := backend["workspace_key_prefix"].(string); ok && prefix != "" {
        return prefix
    }

    // Priority 2: metadata.name (logical identity)
    if name, ok := metadata["name"].(string); ok && name != "" {
        return strings.ReplaceAll(name, "/", "-")
    }

    // Priority 3: metadata.component (physical path)
    if baseComponent, ok := metadata["component"].(string); ok && baseComponent != "" {
        return strings.ReplaceAll(baseComponent, "/", "-")
    }

    // Priority 4: Atmos component name
    return strings.ReplaceAll(component, "/", "-")
}
```

### Backwards Compatibility

**This change is fully backwards compatible:**

1. If `metadata.name` is not set → falls back to current behavior
2. Existing configurations without `metadata.name` work exactly as before
3. Users opt-in by adding `metadata.name` to their components

**No breaking change. No configuration flag needed.**

## Use Cases

### Use Case 1: Versioned Component Folders

**Before (problematic):**
```yaml
vpc-prod:
  metadata:
    component: vpc/v2
  # workspace_key_prefix = "vpc-v2" ← includes version!
```

**After (stable):**
```yaml
vpc-prod:
  metadata:
    name: vpc           # Logical identity
    component: vpc/v2   # Physical path
  # workspace_key_prefix = "vpc" ← stable across versions
```

**Upgrade path:**
```yaml
# Just change the physical path
vpc-prod:
  metadata:
    name: vpc
    component: vpc/v3   # Updated version
  # workspace_key_prefix still = "vpc" ← no state migration!
```

### Use Case 2: Release Tracks

```yaml
# Development uses beta track
vpc-dev:
  metadata:
    name: vpc
    component: beta/vpc
  # workspace_key_prefix = "vpc"

# Production uses stable track
vpc-prod:
  metadata:
    name: vpc
    component: stable/vpc
  # workspace_key_prefix = "vpc"
```

Both environments share the logical identity `vpc` but use different code paths.

### Use Case 3: Inherited from Base Component

With metadata inheritance enabled (see `metadata-inheritance.md`):

```yaml
# catalog/vpc-defaults.yaml
vpc/defaults:
  metadata:
    type: abstract
    name: vpc           # Defined once
    component: vpc/v2   # Current version

# stacks/prod.yaml
vpc-prod:
  metadata:
    inherits:
      - vpc/defaults
  # Inherits name: vpc, component: vpc/v2
  # workspace_key_prefix = "vpc"
```

Upgrading all environments to v3 requires changing only `vpc/defaults`.

### Use Case 4: Multiple Instances

```yaml
# Two VPCs in same stack
vpc/primary:
  metadata:
    name: vpc-primary     # Unique logical identity
    component: vpc        # Same Terraform code
  # workspace_key_prefix = "vpc-primary"

vpc/secondary:
  metadata:
    name: vpc-secondary   # Unique logical identity
    component: vpc        # Same Terraform code
  # workspace_key_prefix = "vpc-secondary"
```

### Use Case 5: No metadata.name (Backwards Compatible)

```yaml
# Existing configuration - no changes needed
vpc:
  metadata:
    component: vpc
  # workspace_key_prefix = "vpc" (from metadata.component)

# Or without metadata.component
vpc:
  vars:
    name: my-vpc
  # workspace_key_prefix = "vpc" (from Atmos component name)
```

## Implementation

### Code Changes

**File: `internal/exec/stack_processor_backend.go`**

Update `setS3BackendDefaults()`:

```go
func setS3BackendDefaults(backend map[string]any, component string, baseComponentName string, metadata map[string]any) {
    if p, ok := backend["workspace_key_prefix"].(string); !ok || p == "" {
        workspaceKeyPrefix := component

        // Priority: metadata.name > metadata.component > Atmos component name
        if name, ok := metadata["name"].(string); ok && name != "" {
            workspaceKeyPrefix = name
        } else if baseComponentName != "" {
            workspaceKeyPrefix = baseComponentName
        }

        backend["workspace_key_prefix"] = strings.ReplaceAll(workspaceKeyPrefix, "/", "-")
    }
}
```

Update `setGCSBackendDefaults()` similarly for `prefix`.

Update `setAzureBackendKey()` similarly for `key`.

**Function signature changes:**

Current:
```go
func setS3BackendDefaults(backend map[string]any, component string, baseComponentName string)
```

New:
```go
func setS3BackendDefaults(backend map[string]any, component string, baseComponentName string, metadata map[string]any)
```

### Caller Updates

Update `processTerraformBackend()` to pass metadata to the backend default functions.

### Schema Changes

Add `name` to the component metadata JSON schema in `pkg/datafetcher/schema/stacks/stack-config/1.0.json`:

```json
{
  "metadata": {
    "type": "object",
    "properties": {
      "name": {
        "type": "string",
        "description": "Logical component identity used for workspace key prefix auto-generation"
      },
      // ... existing fields
    }
  }
}
```

### Tests

1. **Unit tests for `setS3BackendDefaults()`:**
   - With `metadata.name` set → uses name
   - Without `metadata.name`, with `metadata.component` → uses component
   - Without both → uses Atmos component name
   - With explicit `workspace_key_prefix` → uses explicit value (priority 1)

2. **Integration tests:**
   - Versioned component folder scenario
   - Inherited `metadata.name` scenario
   - Multiple instances scenario

## Documentation Changes

1. **Update Terraform Backends docs:**
   - Document `metadata.name` as preferred way to set logical identity
   - Update workspace_key_prefix auto-generation explanation

2. **Update Version Management docs:**
   - Add `metadata.name` to all versioning pattern examples
   - Remove manual `workspace_key_prefix` workarounds
   - Highlight that this solves the state stability problem

3. **Fix existing documentation errors:**
   - Some docs show `settings.workspace_key_prefix` which doesn't exist
   - Should be `backend.s3.workspace_key_prefix` or `metadata.name`

## Migration Guide

### For New Projects

Use `metadata.name` from the start:

```yaml
components:
  terraform:
    vpc:
      metadata:
        name: vpc
        component: vpc/v1
```

### For Existing Projects with Versioned Components

**If currently using explicit `workspace_key_prefix`:**

```yaml
# Before
vpc:
  metadata:
    component: vpc/v2
  backend:
    s3:
      workspace_key_prefix: vpc  # Manual workaround

# After - remove manual workaround
vpc:
  metadata:
    name: vpc                    # New field
    component: vpc/v2
  # backend.s3.workspace_key_prefix no longer needed
```

**If NOT using explicit `workspace_key_prefix` (state already versioned):**

This requires careful migration:

1. Add `metadata.name` with the CURRENT versioned value to preserve state:
   ```yaml
   vpc:
     metadata:
       name: vpc-v2              # Match current state path!
       component: vpc/v2
   ```

2. Or perform Terraform state migration:
   ```bash
   # Move state from vpc-v2/ to vpc/
   terraform state mv ...
   ```

3. Then update to stable name:
   ```yaml
   vpc:
     metadata:
       name: vpc                 # Now stable
       component: vpc/v2
   ```

## Success Criteria

1. `metadata.name` is used for `workspace_key_prefix` when set
2. Fallback to current behavior when `metadata.name` is not set
3. No breaking changes for existing configurations
4. Version upgrades don't require state migration when `metadata.name` is used
5. All backend types (S3, GCS, Azure) support this

## Dependencies

- **metadata-inheritance.md**: For `metadata.name` to be inherited from base components, the metadata inheritance feature must be enabled (`stacks.inherit.metadata: true`)

## References

- [PRD: Metadata Inheritance](./metadata-inheritance.md)
- [Terraform Backends Documentation](https://atmos.tools/core-concepts/components/terraform/backends)
- [Version Management Design Patterns](https://atmos.tools/design-patterns/version-management)
- Related code: `internal/exec/stack_processor_backend.go`
