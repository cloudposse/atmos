# Component Dependencies

**Status**: ✅ Implemented

**Last Updated**: 2026-01-02

**Related PRDs**: [Tool Dependencies Integration](./tool-dependencies-integration.md)

## Overview

The `dependencies.components` section in stack configurations defines relationships between Atmos components. It enables:
- Execution order control (deploy VPC before subnets)
- CI/CD orchestration (Spacelift/Atlantis dependency ordering)
- Impact analysis (`atmos describe affected` detects changes)
- File/folder monitoring (trigger redeployments when external files change)

## Schema

### ComponentDependency Structure

```go
// ComponentDependency represents a dependency entry in dependencies.components.
type ComponentDependency struct {
    // Component instance name (required for component-type dependencies).
    // This is the name under components.<kind>.<name>, not the Terraform module path.
    Component string `yaml:"component,omitempty" json:"component,omitempty" mapstructure:"component"`

    // Stack name (optional, defaults to current stack). Supports Go templates.
    Stack string `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`

    // Kind specifies the dependency type: terraform, helmfile, packer, file, folder, or plugin type.
    // Defaults to the declaring component's type for component dependencies.
    Kind string `yaml:"kind,omitempty" json:"kind,omitempty" mapstructure:"kind"`

    // Path for file or folder dependencies (required when kind is "file" or "folder").
    Path string `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`
}
```

### Field Descriptions

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `component` | string | Yes* | - | Component instance name (under `components.<kind>.<name>`) |
| `stack` | string | No | current stack | Target stack (supports templates) |
| `kind` | string | No | declaring component's type | `terraform`, `helmfile`, `packer`, `file`, `folder`, or registered plugin type |
| `path` | string | Yes** | - | Path for `file` or `folder` kind dependencies |

*Required when `kind` is a component type (terraform, helmfile, packer, plugin).
**Required when `kind` is `file` or `folder`.

### Kind Values

| Kind | Description | Required Fields |
|------|-------------|-----------------|
| `terraform` | Terraform component dependency | `component`, optional `stack` |
| `helmfile` | Helmfile component dependency | `component`, optional `stack` |
| `packer` | Packer component dependency | `component`, optional `stack` |
| `file` | File path dependency | `path` |
| `folder` | Folder path dependency | `path` |
| `<plugin>` | Custom plugin component type | `component`, optional `stack` |

## Examples

### Same-Stack Dependencies (Default Kind)

```yaml
components:
  terraform:
    subnet:
      dependencies:
        components:
          - component: vpc           # kind defaults to "terraform"
          - component: security-group
```

### Cross-Stack Dependencies

```yaml
components:
  terraform:
    app:
      dependencies:
        components:
          - component: vpc
          - component: shared-vpc
            stack: acme-ue1-network
          - component: rds
            stack: "{{ .vars.tenant }}-{{ .vars.environment }}-prod"
```

### Cross-Type Dependencies

```yaml
components:
  terraform:
    app:
      dependencies:
        components:
          - component: vpc                    # terraform (default)
          - component: nginx-ingress
            kind: helmfile                    # helmfile component
            stack: platform-stack
          - component: base-ami
            kind: packer
```

### File/Folder Dependencies

```yaml
components:
  terraform:
    lambda:
      dependencies:
        components:
          - component: vpc
          - kind: file
            path: configs/lambda-settings.json
          - kind: folder
            path: src/lambda/handler
```

## Key Behaviors

### Kind Default Behavior

When `kind` is omitted on a component dependency:
- A terraform component's dependencies default to `kind: terraform`
- A helmfile component's dependencies default to `kind: helmfile`
- A packer component's dependencies default to `kind: packer`

### Merge Behavior

`dependencies.components` uses **append merge** behavior when `list_merge_strategy: append` is configured in `atmos.yaml`. Child stacks add their dependencies to parent dependencies.

```yaml
# Parent stack
dependencies:
  components:
    - component: account-settings

# Child stack (inherits parent)
components:
  terraform:
    vpc:
      dependencies:
        components:
          - component: network-baseline  # APPENDED to parent's dependencies

# Result: vpc depends on both account-settings AND network-baseline
```

### Validation Rules

1. For component dependencies (`kind` is terraform, helmfile, packer, or plugin type):
   - `component` field is required
   - `path` field is ignored

2. For path dependencies (`kind: file` or `kind: folder`):
   - `path` field is required
   - `component` field is ignored

## Helper Methods

The `ComponentDependency` struct provides helper methods:

```go
// IsFileDependency returns true if this is a file dependency.
func (d *ComponentDependency) IsFileDependency() bool {
    return d.Kind == "file"
}

// IsFolderDependency returns true if this is a folder dependency.
func (d *ComponentDependency) IsFolderDependency() bool {
    return d.Kind == "folder"
}

// IsComponentDependency returns true if this is a component dependency (not file or folder).
func (d *ComponentDependency) IsComponentDependency() bool {
    return d.Kind != "file" && d.Kind != "folder"
}
```

## Implementation Details

### Dependency Resolution (`describe_dependents.go`)

The `getComponentDependencies()` function:
1. Checks `dependencies.components` first (preferred location)
2. Falls back to `settings.depends_on` (legacy location)
3. Returns dependencies with source indicator for matching logic

### File/Folder Detection (`describe_affected_components.go`)

The `getFileFolderDependencies()` function:
1. Filters dependencies by `IsFileDependency()` or `IsFolderDependency()`
2. Uses `dep.Path` for the file/folder location
3. Supports legacy `file`/`folder` fields for backward compatibility

## Migration from `settings.depends_on`

The legacy `settings.depends_on` format continues to work. When migrating:

| Old Format | New Format |
|------------|------------|
| `settings.depends_on` | `dependencies.components` |
| Map with numeric keys | List |
| `namespace`, `tenant`, `environment`, `stage` fields | `stack` field with templates |
| `file` and `folder` fields | `kind: file` or `kind: folder` with `path` field |

### Migration Example

**Before:**
```yaml
settings:
  depends_on:
    1:
      component: vpc
    2:
      component: rds
      stage: prod
    3:
      file: configs/app.json
```

**After:**
```yaml
dependencies:
  components:
    - component: vpc
    - component: rds
      stack: "{{ .vars.tenant }}-{{ .vars.environment }}-prod"
    - kind: file
      path: configs/app.json
```

## Related Documentation

- Component Dependencies User Guide — `website/docs/stacks/dependencies/components.mdx`
- describe dependents — `website/docs/cli/commands/describe/dependents.mdx`
- describe affected — `website/docs/cli/commands/describe/affected.mdx`
- [Tool Dependencies Integration](./tool-dependencies-integration.md)
