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

Tools defined in `.tool-versions` are automatically available in custom commands and workflows. This example includes working demonstrations.

### The `.tool-versions` File

This example includes a `.tool-versions` file with `jq` and `yq`:

```
jq 1.7.1
yq 4.45.1
```

### Custom Commands Demo

Custom commands in `atmos.yaml` automatically have access to toolchain tools:

```bash
# Show which jq binary is being used (should be from .tools/)
atmos toolchain-demo which-jq

# Show which yq binary is being used
atmos toolchain-demo which-yq

# Show versions of both tools
atmos toolchain-demo versions

# Demonstrate jq processing JSON
atmos toolchain-demo jq-demo

# Demonstrate yq processing YAML
atmos toolchain-demo yq-demo
```

### Workflows Demo

Workflows in `stacks/workflows/toolchain-demo.yaml` also have automatic access:

```bash
# Show paths to toolchain binaries
atmos workflow which-tools -f toolchain-demo

# Show tool versions
atmos workflow tool-versions -f toolchain-demo

# Run jq demo
atmos workflow jq-demo -f toolchain-demo

# Run yq demo
atmos workflow yq-demo -f toolchain-demo

# Run complete integration demo
atmos workflow full-demo -f toolchain-demo

# Workflow with explicit dependencies
atmos workflow with-dependencies -f toolchain-demo
```

### How It Works

1. **Automatic Tool Discovery**: When a custom command or workflow runs, Atmos reads `.tool-versions`
2. **Tool Installation**: Missing tools are automatically installed to `.tools/`
3. **PATH Update**: The `.tools/bin/` directories are prepended to PATH
4. **Execution**: Commands and workflow steps execute with toolchain tools available

### Explicit Dependencies

You can also declare explicit dependencies that override `.tool-versions`:

**Custom Command:**
```yaml
commands:
  - name: my-command
    dependencies:
      tools:
        jq: "^1.7.0"
        terraform: "~> 1.10.0"
    steps:
      - jq --version
```

**Workflow:**
```yaml
workflows:
  my-workflow:
    dependencies:
      tools:
        jq: "1.7.1"
    steps:
      - name: check-jq
        command: jq --version
        type: shell
```

## Related Documentation

- [Aqua Registry Documentation](https://aquaproj.github.io/docs/reference/registry/)
- [Atmos Toolchain Documentation](https://atmos.tools/cli/commands/toolchain/)
- [Custom Commands Documentation](https://atmos.tools/cli/configuration/commands/)
- [Workflows Documentation](https://atmos.tools/workflows/)
