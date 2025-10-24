# Toolchain Package

Developer documentation for the Atmos toolchain implementation.

## Overview

The `toolchain` package provides programmatic tool version management for Atmos, enabling:

- Installing CLI tools (terraform, helm, kubectl, etc.) with version pinning
- Executing tools with specific versions via `atmos toolchain exec`
- Managing multiple concurrent versions of the same tool
- Integration with Aqua registry for package definitions
- Support for `.tool-versions` files (ASDF-compatible format)

## Package Structure

```
toolchain/
├── aqua_registry.go        # Aqua registry integration and package resolution
├── github.go               # GitHub API client for release discovery
├── installer.go            # Core installation logic and binary extraction
├── local_config.go         # Local tools.yaml configuration management
├── tool_versions.go        # .tool-versions file parsing and management
├── types.go                # Core data structures and YAML types
├── which.go                # Tool lookup and path resolution
├── *_test.go               # Unit tests (76.3% coverage)
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

#### LocalConfigManager (`local_config.go`)
- Manages `tools.yaml` for tool aliases
- Maps friendly names to registry paths (`terraform` → `hashicorp/terraform`)
- Prevents duplicate entries in `.tool-versions`

## Integration Points

### Atmos Configuration

The toolchain integrates with Atmos configuration (`atmos.yaml`):

```yaml
toolchain:
  tools_dir: .tools              # Where to install binaries
  file_path: .tool-versions      # Tool version declarations
```

### Component Dependencies (Planned)

Future feature to declare tool dependencies at component or stack level:

```yaml
# Stack-level (applies to all components)
dependencies:
  tools:
    terraform: "~> 1.10.0"
    tflint: "^0.54.0"

components:
  terraform:
    vpc:
      # Component-level (overrides stack-level)
      dependencies:
        tools:
          terraform: "1.10.3"
          checkov: "latest"
```

**Status**: 🚧 Not implemented (see `docs/prd/toolchain-implementation.md`)

## Usage Patterns

### Installing Tools

```go
import "github.com/cloudposse/atmos/toolchain"

// Install specific version
err := toolchain.InstallExec("terraform@1.10.3")

// Install all from .tool-versions
err := toolchain.InstallExec("")
```

### Executing Tools

```go
// Exec replaces current process with tool binary
err := toolchain.ExecExec("terraform@1.10.3", []string{"--version"})

// Which prints path to tool binary
err := toolchain.WhichExec("terraform@1.10.3")
```

### Listing Installed Tools

```go
err := toolchain.ListExec()
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

### Coverage: 76.3%

**Target**: 80-90%

Run tests:
```bash
go test ./toolchain/...
```

With coverage:
```bash
go test ./toolchain/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Helpers

- `NewMockAquaRegistry()` - Mock registry for testing
- `NewMockInstaller()` - Mock installer for testing
- Uses `t.TempDir()` for isolated test environments

### Example Test

```go
func TestInstallTool(t *testing.T) {
    tempDir := t.TempDir()
    SetAtmosConfig(&schema.AtmosConfiguration{
        Toolchain: schema.Toolchain{
            ToolsDir: tempDir,
            FilePath: filepath.Join(tempDir, ".tool-versions"),
        },
    })

    err := InstallExec("terraform@1.10.3")
    require.NoError(t, err)

    binaryPath := filepath.Join(tempDir, "bin", "hashicorp", "terraform", "1.10.3", "terraform")
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
terraform 1.10.3
helm 3.17.0
kubectl 1.32.0
```

**Format:**
- One tool per line: `<tool-name> <version> [version2...]`
- Comments start with `#`
- Blank lines ignored
- Multiple versions supported (space-separated)

### `tools.yaml` (Local Aliases)

Maps friendly names to registry paths:

```yaml
aliases:
  terraform: hashicorp/terraform
  helm: helm/helm
  kubectl: kubernetes-sigs/kubectl
```

**Location:** `./.tools/tools.yaml` (created automatically)

## Common Patterns

### Adding a New Command

1. Create command file in `cmd/toolchain/`
2. Implement business logic in `toolchain/`
3. Add tests in `toolchain/*_test.go`
4. Update PRD in `docs/prd/toolchain-implementation.md`

### Adding Support for New Package Type

1. Update `types.go` with new package type constant
2. Extend `installer.go` with new download logic
3. Add parsing logic in `aqua_registry.go`
4. Add tests for new package type

## Dependencies

- `gopkg.in/yaml.v3` - YAML parsing
- `github.com/gabriel-vasile/mimetype` - File type detection
- `github.com/cloudposse/atmos/pkg/http` - HTTP client
- `github.com/cloudposse/atmos/pkg/schema` - Atmos configuration types

## Future Enhancements

See `docs/prd/toolchain-implementation.md` for detailed roadmap:

- **Component Dependencies**: Tool requirements at component/stack level
- **Atmos Self-Management**: Auto-exec based on `.tool-versions`
- **Configurable Registries**: Support custom registries beyond Aqua
- **Enhanced Test Coverage**: Reach 80-90% coverage target

## References

- **PRD**: `docs/prd/toolchain-implementation.md`
- **Commands**: `cmd/toolchain/`
- **HTTP Client**: `pkg/http/`
- **Aqua Registry**: https://github.com/aquaproj/aqua-registry
