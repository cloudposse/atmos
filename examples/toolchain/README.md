# Toolchain Configuration Example

This example demonstrates how to configure Atmos toolchain with tool registries, and how toolchain-managed tools integrate automatically with custom commands and workflows.

## Overview

The toolchain system enables:
- **Private/Corporate Tools**: Maintain internal tool registries
- **Air-Gapped Deployments**: Mirror registries for offline environments
- **Security & Compliance**: Control tool sources and versions
- **Registry Precedence**: Local/corporate registries override public ones
- **Automatic Integration**: Tools from `.tool-versions` are available in custom commands and workflows

## Configuration

### Registry Types

#### Aqua Registry (`type: aqua`)

The `type` field specifies the **registry format/schema**, not the transport protocol. Currently, only Aqua registry format is fully supported.

**Registry Patterns:**

Atmos supports two patterns for Aqua registries:

1. **Single Index File** (like official Aqua registry):
   ```yaml
   toolchain:
     registries:
       - name: custom
         type: aqua
         source: file://./custom-registry/registry.yaml  # Points to index file
         priority: 100
   ```
   - Detection: Source ends with `.yaml` or `.yml`
   - All packages in one file
   - Recommended for custom/corporate registries

2. **Per-Package Directory** (separate file per package):
   ```yaml
   toolchain:
     registries:
       - name: custom
         type: aqua
         source: file://./custom-registry/pkgs/  # Points to directory
         priority: 100
   ```
   - Detection: Source doesn't end with `.yaml`/`.yml`
   - Each package has its own `registry.yaml` at `{source}/{owner}/{repo}/registry.yaml`
   - Each registry.yaml file contains ONE package definition
   - Used by official Aqua registry for per-tool lookups

**Official Aqua Registry:**
```yaml
toolchain:
  registries:
    - name: aqua-public
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

**Note**: GitHub URLs are automatically converted to raw URLs internally (tree → raw, blob → raw).

**Key Points**:
- `type: aqua` means the registry uses Aqua's manifest format/schema
- `source` is the URL/path to either an index file or directory
- Pattern is auto-detected based on file extension
- **Best practice**: Use single index file for custom registries

#### Inline Registry (`type: atmos`)

Define tools directly in `atmos.yaml` without external files:

```yaml
toolchain:
  registries:
    - name: my-tools
      type: atmos
      priority: 150
      tools:
        stedolan/jq:
          type: github_release
          url: "jq-{{.OS}}-{{.Arch}}"
          binary_name: jq
        mikefarah/yq:
          type: github_release
          url: "yq_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
```

**Use Cases**:
- Quick tool definitions without creating separate files
- Prototyping custom tools
- Small number of internal tools
- Simple overrides for specific versions

**Template Variables**:
- `{{.OS}}` - Operating system (darwin, linux, windows)
- `{{.Arch}}` - Architecture (amd64, arm64, 386)
- `{{.Version}}` - Tool version
- `{{trimV .Version}}` - Version without 'v' prefix
- `{{.RepoOwner}}` / `{{.RepoName}}` - Repository info
- `{{.Format}}` - Archive format.

See [atmos.yaml](./atmos.yaml) for a complete example showing inline registry combined with other registry types.

#### Future Registry Types

- Custom registry formats may be added in the future

### Priority System

Registries are checked in priority order (highest to lowest):

1. **Configured registries** (by priority value, highest first)
2. **Error if tool not found in any registry**

Example:

```yaml
toolchain:
  registries:
    - name: corporate
      priority: 100  # Checked first

    - name: mirror
      priority: 50   # Checked second

    - name: public
      priority: 10   # Checked last
```

## Use Cases

### Corporate Registry with Public Fallback

```yaml
# Use corporate Aqua-format registry with public fallback
toolchain:
  versions_file: .tool-versions

  registries:
    - name: acme-corp
      type: aqua  # Aqua format
      source: https://tools.acme.example.com/registry/pkgs
      priority: 100

    - name: aqua-public
      type: aqua  # Aqua format, official source
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

### Air-Gapped Environment

```yaml
# Only use internal mirror (no internet access)
toolchain:
  registries:
    - name: internal-mirror
      type: aqua  # Aqua format
      source: https://registry.internal.example.com/pkgs
      priority: 100
```

### Multiple Registries with Redundancy

```yaml
toolchain:
  versions_file: .tool-versions

  registries:
    # Corporate registry (checked first)
    - name: corporate
      type: aqua
      source: https://tools.corp.example.com/pkgs
      priority: 100

    # Internal mirror (backup)
    - name: mirror
      type: aqua
      source: https://mirror.corp.example.com/pkgs
      priority: 50

    # Public Aqua registry (final fallback)
    - name: aqua-public
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

## Backward Compatibility

No configuration changes required for existing setups:

```yaml
# No toolchain.registries configured
# Automatically uses standard Aqua registry (current behavior)
```

Explicit standard registry:

```yaml
toolchain:
  registries:
    - type: aqua  # Explicitly use standard Aqua registry
```

## Testing

Test your configuration:

```bash
# List installed tools
atmos toolchain list

# Install a tool to verify registry resolution
atmos toolchain install terraform@1.13.4

# Check installed tool location
atmos toolchain which terraform

# Set a different version as default
atmos toolchain set terraform@1.10.3
```

## Troubleshooting

### Tool Not Found

If a tool is not found:
1. Verify the tool name uses `owner/repo` format (e.g., `hashicorp/terraform`)
2. Verify registry URLs are accessible
3. Ensure GitHub token is set if using private registries: `export GITHUB_TOKEN=...`
4. Check registry priority configuration
5. Verify the tool exists in at least one configured registry

### Registry Unavailable

Registries are checked in order. If a high-priority registry is unavailable, lower-priority registries will be tried automatically.

## Toolchain Integration with Custom Commands and Workflows

Tools defined in `.tool-versions` are automatically available in custom commands and workflows. This example demonstrates four different integration patterns.

### The `.tool-versions` File

This example includes a `.tool-versions` file defining project-wide tool defaults:

```
jq 1.7.1
yq 4.45.1
```

### Custom Commands Demo

Each example demonstrates a different toolchain feature:

```bash
# 1. Implicit dependencies - uses tools from .tool-versions
atmos toolchain-demo which

# 2. Exact version pinning - override with specific version
atmos toolchain-demo pinned

# 3. SemVer constraints - allow compatible version ranges
atmos toolchain-demo constrained

# 4. Multi-tool pipeline - combine tools together
atmos toolchain-demo convert
```

### Workflows Demo

Workflows demonstrate the same patterns:

```bash
# 1. Implicit dependencies - uses tools from .tool-versions
atmos workflow which -f toolchain-demo

# 2. Exact version pinning
atmos workflow pinned -f toolchain-demo

# 3. SemVer constraints
atmos workflow constrained -f toolchain-demo

# 4. Multi-tool pipeline
atmos workflow convert -f toolchain-demo
```

### Version Constraint Patterns

| Pattern | Example | Meaning |
|---------|---------|---------|
| Exact | `1.7.1` | Only version 1.7.1 |
| Caret | `^1.7.0` | Any 1.x.x (compatible) |
| Tilde | `~> 4.40.0` | Any 4.40.x (patch only) |
| Latest | `latest` | Most recent version |

### How It Works

1. **Automatic Tool Discovery**: Atmos reads `.tool-versions` when commands/workflows run
2. **Dependency Merging**: Explicit `dependencies` override `.tool-versions`
3. **Tool Installation**: Missing tools are automatically installed to `.tools/`
4. **PATH Update**: Toolchain binaries are prepended to PATH

## Related Documentation

- [Aqua Registry Documentation](https://aquaproj.github.io/docs/reference/registry/)
- [Atmos Toolchain Documentation](https://atmos.tools/cli/commands/toolchain/)
- [Custom Commands Documentation](https://atmos.tools/cli/configuration/commands/)
- [Workflows Documentation](https://atmos.tools/workflows/)
