# Toolchain Package

Developer documentation for the Atmos toolchain implementation.

## Overview

The `toolchain` package provides programmatic tool version management for Atmos, enabling:

- Installing CLI tools (terraform, helm, kubectl, etc.) with version pinning
- Executing tools with specific versions via `atmos toolchain exec`
- Managing multiple concurrent versions of the same tool
- Integration with Aqua registry for package definitions (1000+ tools)
- Support for `.tool-versions` files (ASDF-compatible format)
- Automatic tool installation based on component, workflow, and command dependencies

## Implementation Status

### ✅ Implemented Features

- **Core Commands**: add, remove, set, install, uninstall, clean, list, get, info, exec, which, path
- **Registry Integration**: Aqua registry with search and list capabilities
- **Tool Dependencies**: Component-level, workflow, and custom command dependencies with auto-install
- **XDG Compliance**: Uses XDG Base Directory Specification for cache
- **GitHub Integration**: Automatic GitHub token authentication for API calls
- **ASDF Compatibility**: Full compatibility with `.tool-versions` files

### 🚧 Planned Features

- **Lockfile Support**: `.tool-versions.lock` for reproducible builds with checksums
- **Version Constraints**: Semantic version ranges (e.g., `^1.9.0`, `~> 1.10.0`)
- **Custom Registries**: Support for private tool registries
- **Performance Optimizations**: Parallel downloads and improved caching

## Package Structure

```
toolchain/
├── aqua_registry.go        # Aqua registry integration and package resolution
├── atmos_registry.go       # Atmos-specific registry (placeholder)
├── github.go               # GitHub API client for release discovery
├── installer.go            # Core installation logic and binary extraction
├── tool_versions.go        # .tool-versions file parsing and management
├── types.go                # Core data structures and YAML types
├── add.go                  # Add tools to .tool-versions
├── remove.go               # Remove tools from .tool-versions
├── set.go                  # Set default tool version
├── install.go              # Install tools
├── uninstall.go            # Uninstall tools
├── clean.go                # Clean tools and cache
├── list.go                 # List installed tools
├── get.go                  # Get available versions
├── info.go                 # Tool information
├── exec.go                 # Execute tools
├── which.go                # Tool lookup and path resolution
├── path.go                 # PATH management
├── registry/               # Registry implementations
│   ├── aqua.go            # Aqua registry client
│   ├── interface.go       # Registry interface
│   └── types.go           # Registry types
├── *_test.go               # Unit tests
└── README.md               # This file
```

## Architecture

### Registry Pattern

The toolchain uses the Aqua registry format but implements its own parser rather than depending on Aqua's Go modules:

**Why reimplement?**
- Aqua's Go modules are **not stable** for external use (author's explicit statement)
- Focused use case: Atmos integration, not standalone shell experience
- Dependency control: No lock-in to Aqua's development timeline
- Extensibility: Can add Atmos-specific features

**Why use Aqua registry format?**
- Community standard with hundreds of pre-configured tools
- Well-tested YAML format
- Remote integration: Fetch registry files directly from GitHub

### Key Components

#### AquaRegistry (`aqua_registry.go`)
- Fetches package definitions from Aqua registry (remote YAML files)
- Caches registry files locally (`~/.cache/tools-cache/`)
- Resolves tool aliases to canonical `owner/repo` format
- Parses version overrides and asset templates

#### Installer (`installer.go`)
- Downloads binaries from GitHub releases or HTTP URLs
- Extracts archives (`.tar.gz`, `.zip`, `.gz`, raw binaries)
- Installs to `.tools/bin/` with versioned subdirectories
- Makes binaries executable
- Supports concurrent versions: `.tools/bin/owner/repo/version/binary`

#### ToolVersions (`tool_versions.go`)
- Parses `.tool-versions` files (ASDF format)
- Manages tool version declarations
- Supports comments and blank lines
- Thread-safe read/write operations

#### Registry System (`registry/`)
- Interface-based registry pattern for extensibility
- Aqua registry implementation for 1000+ tools
- Supports search, list, and tool resolution
- Future support for custom registries

## Integration Points

### Atmos Configuration

The toolchain integrates with Atmos configuration (`atmos.yaml`):

```yaml
toolchain:
  tools_dir: .tools              # Where to install binaries
  file_path: .tool-versions      # Tool version declarations
```

### Component Dependencies

Tool dependencies can be declared at multiple levels in stack configuration with proper inheritance:

```yaml
# Global dependencies (applies to all components)
dependencies:
  tools:
    aws-cli: "2.0.0"
    jq: "latest"

# Component-type dependencies (applies to all terraform components)
terraform:
  dependencies:
    tools:
      terraform: "1.9.8"
      tflint: "0.54.0"

# Component instance dependencies (specific component)
components:
  terraform:
    vpc:
      dependencies:
        tools:
          terraform: "1.9.8"
          checkov: "latest"
```

**Status**: ✅ Implemented for components, workflows, and custom commands

**Implementation**: See `pkg/dependencies/resolver.go` for the dependency resolution logic. When you run `atmos terraform plan`, Atmos automatically:
1. Resolves tool dependencies from stack configuration (with inheritance)
2. Installs missing tools
3. Updates PATH to include installed tools
4. Executes the component with the correct tool versions

## Usage Patterns

### Installing Tools

```go
import "github.com/cloudposse/atmos/toolchain"

// Install specific version
err := toolchain.RunInstall("terraform@1.9.8", false, false)

// Install all from .tool-versions
err := toolchain.RunInstall("", false, false)
```

### Executing Tools

```go
// Exec replaces current process with tool binary
err := toolchain.RunExec("terraform@1.9.8", []string{"plan"})

// Which prints path to tool binary
err := toolchain.RunWhich("terraform")
```

### Managing Tools

```go
// Add tool to .tool-versions
err := toolchain.RunAdd("terraform@1.9.8")

// Remove tool from .tool-versions
err := toolchain.RunRemove("terraform")

// List installed tools
err := toolchain.RunList()

// Get available versions
err := toolchain.ListToolVersions(true, 10, "terraform")

// Clean tools and cache
err := toolchain.CleanToolsAndCaches(toolsDir, cacheDir, tempDir)
```

## Supported Package Types

### GitHub Releases (`github_release`)

Downloads assets from GitHub releases:

```yaml
type: github_release
repo_owner: hashicorp
repo_name: terraform
asset: terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip
```

### HTTP Downloads (`http`)

Direct HTTP downloads with templating:

```yaml
type: http
url: https://releases.hashicorp.com/terraform/{{.Version}}/terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip
```

### Supported Archive Formats

- `.tar.gz` - Gzip-compressed tarballs
- `.zip` - ZIP archives
- `.gz` - Single gzip-compressed binaries
- Raw binaries (no archive)

### Template Functions

- `trimV` - Remove 'v' prefix from versions
- `trimPrefix` - Remove prefix from strings
- `trimSuffix` - Remove suffix from strings
- `replace` - String replacement

## Testing

### Test Coverage

Run tests:
```bash
make test                    # Quick tests
make testacc                 # Full test suite
make testacc-cover          # With coverage report
```

Or directly:
```bash
go test ./toolchain/...
go test ./toolchain/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Helpers

- Uses `t.TempDir()` for isolated test environments
- I/O context initialization via `InitTestIOContext(t)`
- Mock registries with controlled responses
- Table-driven tests for comprehensive coverage

### Example Test

```go
func TestInstallTool(t *testing.T) {
    tempDir := t.TempDir()

    // Initialize I/O context for UI functions
    InitTestIOContext(t)

    SetAtmosConfig(&schema.AtmosConfiguration{
        Toolchain: schema.Toolchain{
            InstallPath: tempDir,
            VersionsFile: filepath.Join(tempDir, ".tool-versions"),
        },
    })

    err := RunInstall("terraform@1.9.8", false, false)
    require.NoError(t, err)

    binaryPath := filepath.Join(tempDir, "hashicorp", "terraform", "1.9.8", "terraform")
    assert.FileExists(t, binaryPath)
}
```

## HTTP Client

Uses centralized `pkg/http` package for all HTTP operations:

```go
import httpClient "github.com/cloudposse/atmos/pkg/http"

client := httpClient.NewDefaultClient(
    httpClient.WithGitHubToken(httpClient.GetGitHubTokenFromEnv()),
)
```

**Features:**
- Automatic GitHub token authentication
- Configurable timeouts
- Proper User-Agent headers
- Rate limit handling

**Token sources** (priority order):
1. `ATMOS_GITHUB_TOKEN` environment variable
2. `GITHUB_TOKEN` environment variable

## Error Handling

Uses static errors from `errors/errors.go`:

```go
import errUtils "github.com/cloudposse/atmos/pkg/errors"

if err != nil {
    return fmt.Errorf("%w: failed to install tool: %w", errUtils.ErrToolchainInstall, err)
}
```

**Check errors with:**
```go
if errors.Is(err, errUtils.ErrToolchainInstall) {
    // Handle installation error
}
```

## Performance Tracking

All public functions use performance tracking:

```go
func InstallExec(tool string) error {
    defer perf.Track(atmosConfig, "toolchain.InstallExec")()

    // Implementation
}
```

## Configuration Files

### `.tool-versions` (ASDF Format)

Declares tool versions for a project:

```
# Tools for this project
terraform 1.9.8
helm 3.17.0
kubectl 1.32.0
```

**Format:**
- One tool per line: `<tool-name> <version> [version2...]`
- Comments start with `#`
- Blank lines ignored
- Multiple versions supported (space-separated)

**Tool Name Resolution:**
- Exact matches: `hashicorp/terraform` → `hashicorp/terraform`
- Alias resolution: `terraform` → resolved via Aqua registry to `hashicorp/terraform`
- Short names automatically resolved to canonical registry paths

## Available Commands

All commands are available via `atmos toolchain`:

### Core Commands
- `add <tool@version>` - Add tool to .tool-versions
- `remove <tool>` - Remove tool from .tool-versions
- `set <tool> <version>` - Set default tool version
- `install [tool@version]` - Install tool(s)
- `uninstall [tool@version]` - Uninstall tool(s)
- `clean` - Remove all tools and cache
- `list` - List installed tools with status
- `get [tool]` - Get available versions
- `info <tool>` - Show tool information
- `exec <tool@version> -- <args>` - Execute tool with specific version
- `which <tool>` - Show path to tool binary
- `path` - Show toolchain PATH entries

### Registry Commands
- `registry list [registry-name]` - List registries or tools in registry
- `registry search <query>` - Search for tools across registries
- `search <query>` - Alias to `registry search`

## Development Patterns

### Adding a New Command

1. Create command file in `cmd/toolchain/`
2. Implement business logic in `toolchain/`
3. Add tests in `toolchain/*_test.go`
4. Add documentation in `website/docs/cli/commands/toolchain/`
5. Update PRD in `docs/prd/toolchain-implementation.md`

### Adding Support for New Package Type

1. Update `types.go` with new package type constant
2. Extend `installer.go` with new download logic
3. Add parsing logic in registry implementation
4. Add tests for new package type

## Dependencies

- `gopkg.in/yaml.v3` - YAML parsing
- `github.com/gabriel-vasile/mimetype` - File type detection
- `github.com/cloudposse/atmos/pkg/http` - HTTP client
- `github.com/cloudposse/atmos/pkg/schema` - Atmos configuration types

## Future Enhancements

See `docs/prd/tool-dependencies-integration.md` for detailed roadmap:

- **Lockfile Support**: `.tool-versions.lock` for reproducible builds with checksums
- **Version Constraints**: Semantic version ranges (e.g., `^1.9.0`, `~> 1.10.0`)
- **Custom Registries**: Support for private tool registries beyond Aqua
- **Performance Optimizations**: Parallel downloads and improved caching
- **Auto-Update**: Automatic tool updates when new versions are available

## References

- **Implementation PRD**: `docs/prd/toolchain-implementation.md`
- **Dependencies PRD**: `docs/prd/tool-dependencies-integration.md`
- **Commands**: `cmd/toolchain/`
- **Dependency Resolver**: `pkg/dependencies/`
- **HTTP Client**: `pkg/http/`
- **Aqua Registry**: https://github.com/aquaproj/aqua-registry
- **Documentation**: `website/docs/cli/commands/toolchain/`
