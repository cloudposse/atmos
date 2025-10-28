# Custom Toolchain Registry Example

This directory demonstrates how to create a custom Aqua-format registry for Atmos toolchain using a **single index file**.

## Structure

```
custom-registry/
├── README.md
└── registry.yaml     # Single index file with all tool definitions
```

## Registry Patterns

Atmos supports two registry patterns:

### 1. Single Index File (Recommended for Custom Registries)

A single `registry.yaml` file containing all packages - **just like the official Aqua registry**:

```yaml
# Source points directly to the file
source: file://./custom-registry/registry.yaml
source: https://example.com/registry.yaml
```

**Detection**: If source ends with `.yaml` or `.yml`, it's treated as a single index file.

### 2. Per-Package Directory (Separate File Per Package)

Each package (tool) has its own `registry.yaml` file:

```yaml
# Source points to a directory
source: file://./custom-registry/pkgs/
source: https://example.com/pkgs/
```

**Package files located at:**
- `{source}/{owner}/{repo}/registry.yaml` - one package definition per file
- `{source}/{repo}/registry.yaml` - fallback pattern

**Detection**: If source doesn't end with `.yaml`/`.yml`, it's treated as a per-package directory.

**Key difference**: Single index = multiple packages in one file. Per-package = one package per file.

**Recommendation**: Use the single index file pattern for custom registries - it's simpler to maintain and matches how the official Aqua registry works.

## Registry Format

A single `registry.yaml` file contains all tool definitions:

```yaml
packages:
  # First tool
  - name: owner/tool1
    type: github_release
    repo_owner: owner
    repo_name: tool1
    asset: "tool1-{{.OS}}-{{.Arch}}"
    description: First tool
    files:
      - name: tool1
        src: "tool1-{{.OS}}-{{.Arch}}"

  # Second tool
  - name: owner/tool2
    type: github_release
    repo_owner: owner
    repo_name: tool2
    asset: "tool2_{{.OS}}_{{.Arch}}"
    description: Second tool
    files:
      - name: tool2
        src: "tool2_{{.OS}}_{{.Arch}}"
```

**Key Fields**:
- `name`: Full tool identifier (owner/repo)
- `type`: Source type (github_release, http, etc.)
- `asset`: GitHub release asset pattern
- `files`: Mapping of downloaded files to executable names
- `replacements`: OS/arch name mappings for asset patterns

## Use Cases

### 1. Corporate Tools
Define internal or proprietary tools not available in public registries:

```yaml
packages:
  - type: github_release
    repo_owner: acme-corp
    repo_name: internal-cli
    asset: "cli-{{.OS}}-{{.Arch}}"
```

### 2. Version Pinning
Override public registry versions with specific tested versions:

```yaml
packages:
  - type: github_release
    repo_owner: hashicorp
    repo_name: terraform
    # Only allow specific tested versions
    version_constraint: semver("1.5.x || 1.6.x")
```

### 3. Custom Asset Patterns
Handle tools with non-standard release patterns:

```yaml
packages:
  - type: github_release
    repo_owner: kubernetes
    repo_name: kubectl
    # Custom asset naming that differs from standard
    asset: "kubectl-{{.Version}}-{{.OS}}-{{.Arch}}.tar.gz"
```

### 4. Mirror for Air-Gapped Environments
Point to internal mirrors of public tools:

```yaml
packages:
  - type: http
    url: "https://internal-mirror.corp.com/tools/{{.Tool}}/{{.Version}}/{{.Tool}}-{{.OS}}-{{.Arch}}"
```

## Testing

Reference this registry in your `atmos.yaml`:

```yaml
toolchain:
  registries:
    - name: my-custom-registry
      type: aqua
      source: file://./custom-registry/pkgs
      priority: 100
```

Install tools from this registry:

```bash
# Install jq from custom registry
atmos toolchain install jq@1.7.1

# Install yq from custom registry
atmos toolchain install yq@4.44.1

# Verify tool source
atmos toolchain which jq
```

## Creating Your Own Registry

1. **Create registry file** (`my-registry/registry.yaml`):
   ```yaml
   packages:
     - name: owner/tool
       type: github_release
       repo_owner: owner
       repo_name: tool
       asset: "tool-{{.OS}}-{{.Arch}}"
       files:
         - name: tool
           src: "tool-{{.OS}}-{{.Arch}}"
   ```

2. **Configure in atmos.yaml**:
   ```yaml
   toolchain:
     registries:
       - name: my-registry
         type: aqua
         source: file://./my-registry/registry.yaml
         priority: 100
   ```

3. **Test installation**:
   ```bash
   atmos toolchain install tool@1.0.0
   ```

## Hosting Options

### Local File System
```yaml
source: file://./custom-registry/pkgs
source: file:///absolute/path/to/registry/pkgs
```

### HTTP/HTTPS Server
```yaml
source: https://registry.example.com/pkgs
```

### GitHub Repository
```yaml
source: https://github.com/org/registry/tree/main/pkgs
# Automatically converted to:
# https://raw.githubusercontent.com/org/registry/main/pkgs
```

### Git Repository (any provider)
```yaml
source: https://gitlab.com/org/registry/-/tree/main/pkgs
```

## Best Practices

1. **Version Constraints**: Use semantic version constraints to limit allowed versions
2. **Testing**: Test tool definitions across platforms (Linux, macOS, Windows)
3. **Documentation**: Document custom tools and their purpose in your registry
4. **Priority**: Set appropriate priority (higher = checked first)
5. **Fallback**: Always include aqua-public registry as fallback
6. **Security**: For corporate registries, use authentication and HTTPS
7. **Mirroring**: Consider mirroring public registries for reliability

## Reference

- [Aqua Registry Format Documentation](https://aquaproj.github.io/docs/reference/registry/)
- [Aqua Registry Config Spec](https://aquaproj.github.io/docs/reference/registry-config/)
- [Atmos Toolchain Documentation](https://atmos.tools/cli/commands/toolchain/)
