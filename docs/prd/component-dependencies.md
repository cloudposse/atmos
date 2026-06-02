# Component Dependencies

**Status**: ✅ Implemented (v1 shipped in v1.210.0; v2 surface added in v1.211.0 — see "v2 Surface" section below)

**Last Updated**: 2026-05-05

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
3. Supports legacy `file`/`folder` fields for backward compatibility.

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

## v2 Surface (additive, no breaking change)

The original v1 shape that shipped in v1.210.0 layered three design smells:

1. **Container/contents mismatch.** The container is named `dependencies.components` (a noun for the *category*) but its entries can be `kind: file`/`kind: folder` — files and folders are not components.
2. **Two entry shapes mashed together.** Component entries use a value-bearing key (`component: vpc`); path entries use a discriminator pattern (`kind: file` + `path:`). Half-discriminated, half-typed-by-key.
3. **`kind` overload.** `kind` at `components.<kind>.<name>` means *component type* (terraform/helmfile/packer). `kind` inside a dependency entry adds `file`/`folder`. Same word, overlapping but different domains.

Hard renames are off the table — the v1 surface ships in a public release. The v2 surface is **purely additive** and reconciles the smells without breaking any existing YAML.

### v2 input surface

```yaml
dependencies:
  tools:                       # unchanged (map of tool → version)
    terraform: "1.9.8"

  components:                  # ONLY component-to-component deps
    - name: vpc                # canonical (preferred over `component:`)
      stack: prod
    - name: nginx
      kind: helmfile
      stack: platform-stack

  files:                       # NEW sibling key, replaces inline `kind: file`
    - configs/lambda-settings.json

  folders:                     # NEW sibling key, replaces inline `kind: folder`
    - src/lambda/handler
```

### Backward-compatibility rules (parsed identically to v2)

- `component:` continues to parse as the canonical struct field on `ComponentDependency`. The new `name:` field is an input-side alias.
- `kind: file` / `kind: folder` entries inside `dependencies.components[]` continue to parse and behave identically.
- `settings.depends_on` legacy path is unchanged.

### Normalization (`Dependencies.Normalize`)

After mapstructure decoding of any `dependencies` section, callers MUST invoke `Dependencies.Normalize`. The normalizer:

1. **Resolves the `name` ↔ `component` alias** on every `Components[]` entry. If both are set to the same non-empty value, the alias is cleared. If both are set to *different* non-empty values, returns `schema.ErrComponentDependencyNameConflict`.
2. **Validates inline path-based entries.** Any `Components[i]` with `Kind` ∈ {`file`, `folder`} that lacks `Path` returns `schema.ErrComponentDependencyMissingPath`.
3. **Mirrors `Files` / `Folders` into `Components[]`** as synthetic entries `{Kind: "file"|"folder", Path: ...}`. Downstream code paths that filter `Components[]` by kind continue to see all path-based dependencies regardless of where they were declared.
4. **Backfills typed slices** by promoting any inline file/folder entries from `Components[]` into `Files` / `Folders`. After Normalize, both views are complete and consistent (deduplicated by path).

Net effect: a single internal representation exists post-Normalize, and downstream code paths (`getFileFolderDependencies`, `getComponentDependencies`, `isComponentDependentFolderOrFileChangedIndexed`) work unchanged.

### Sentinel errors

Defined in `pkg/schema/dependencies.go` (locally, to avoid an `errors → pkg/perf → pkg/schema` import cycle):

- `schema.ErrComponentDependencyNameConflict` — both `name` and `component` set to different values on the same entry.
- `schema.ErrComponentDependencyMissingPath` — inline `kind: file/folder` entry without a `path:` field.

### Schema changes

The JSON manifest schema (`website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json` and its mirrored copy under `tests/fixtures/schemas/`) gains:

- `dependencies.files` / `dependencies.folders` keys with `dependencies_files` / `dependencies_folders` definitions.
- `dependencies_component_entry.properties.name` alongside `component`.
- An updated `anyOf` accepting `name`, `component`, OR the legacy `kind: file/folder + path` shape.

`additionalProperties: false` is preserved; the schema still rejects unknown keys.

### Out-of-scope follow-ups

Tracked separately:

- Decide whether/when to emit a soft-deprecation log when `kind: file/folder` is seen inside `components[]`.
- Consider a v2 schema namespace if more shape changes accumulate.
- Same alias treatment for legacy `settings.depends_on` is intentionally NOT in scope.

## Related Documentation

- Component Dependencies User Guide — `website/docs/stacks/dependencies/components.mdx`
- describe dependents — `website/docs/cli/commands/describe/dependents.mdx`
- describe affected — `website/docs/cli/commands/describe/affected.mdx`
- [Tool Dependencies Integration](./tool-dependencies-integration.md)
