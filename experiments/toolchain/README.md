# Toolchain - Atmos Tools Prototype

A standalone Go CLI tool that installs CLI binaries using metadata from the Aqua registry. This is a prototype for the [Atmos packages feature](https://github.com/cloudposse/atmos/issues/927).

## Overview

This tool demonstrates how to integrate with the Aqua registry ecosystem while maintaining independence from the Aqua CLI itself. It serves as a proof-of-concept for the Atmos packages feature, showing how to:

- Parse YAML files from the Aqua registry
- Download GitHub release assets
- Extract binaries from various archive formats
- Install binaries to a local directory
- Support multiple concurrent versions
- Provide backward compatibility with `.tool-versions` files

## Design Rationale

### Why a Custom Parser?

We chose to implement a custom Aqua registry parser rather than using Aqua as a Go module dependency for several reasons:

1. **API Stability**: The Aqua author has explicitly stated that their API is not stable for external use. This means any integration would be fragile and subject to breaking changes.

2. **Dependency Control**: By implementing our own parser, we maintain full control over our dependencies and avoid potential conflicts or version lock-in.

3. **Focused Scope**: We only need a subset of Aqua's functionality. A custom parser allows us to implement exactly what we need without the overhead of the full Aqua codebase.

4. **Long-term Maintainability**: Having our own parser means we're not dependent on Aqua's development timeline or breaking changes.

### Limited Subset Support

Our parser supports a focused subset of Aqua registry features:

**Supported Package Types (from Aqua registry):**
- `http` - Direct HTTP downloads (e.g., HashiCorp releases)
- `github_release` - GitHub release assets with version overrides

**Supported Archive Formats:**
- `.zip` - ZIP archives
- `.tar.gz` - Gzip-compressed tarballs
- `.gz` - Single gzip-compressed binaries
- Raw binaries

**Supported Template Functions:**
- `trimV` - Remove 'v' prefix from versions
- `trimPrefix` - Remove prefix from strings
- `trimSuffix` - Remove suffix from strings
- `replace` - String replacement

**Version Override Support:**
- Basic version constraint handling
- Asset template resolution
- Format detection (zip vs tar.gz)

### Why Use Aqua Registry Without Aqua CLI?

1. **Registry Ecosystem**: The Aqua registry is a well-maintained, community-driven collection of package definitions. It's the de facto standard for CLI tool metadata.

2. **Avoiding CLI Dependencies**: We don't want to require users to install Aqua CLI just to use Atmos tools. This keeps the dependency chain minimal.

3. **Remote Integration**: We fetch registry files directly from GitHub, avoiding the need to clone or maintain a local copy of the registry.

4. **Caching**: We implement our own caching layer for registry files and downloaded assets, optimized for our use case.

## Features

- ✅ Parse Aqua registry YAML files remotely
- ✅ Template interpolation for asset URLs
- ✅ Download from GitHub releases and HTTP sources
- ✅ Support for `.tar.gz`, `.zip`, `.gz`, and raw binaries
- ✅ Magic file type detection using `mimetype` library
- ✅ Cache downloaded assets in `~/.cache/installer`
- ✅ Install binaries to `./.tools/bin/`
- ✅ Support multiple concurrent versions
- ✅ Make binaries executable
- ✅ Backward compatibility with `.tool-versions` files
- ✅ Graceful error handling
- ✅ GitHub token authentication for API requests
- ✅ Intelligent authentication (only applies tokens where needed)
- ✅ Local tool configuration with aliases

## Usage

```bash
# Install a specific version of a tool (using full registry path)
toolchain install hashicorp/terraform@1.9.8
toolchain install opentofu/opentofu@1.10.3

# Install a specific version of a tool (using aliases)
toolchain install terraform@1.9.8
toolchain install opentofu@1.10.3
toolchain install tflint@0.44.1

# Install all tools from .tool-versions file
toolchain install

# Uninstall a specific version of a tool (using aliases)
toolchain uninstall terraform@1.9.8
toolchain uninstall opentofu@1.10.3
toolchain uninstall tflint@0.44.1

# Uninstall all tools from .tool-versions file
toolchain uninstall

# List installed tools with sizes and dates
toolchain list

# Check status of tools in .tool-versions
toolchain tool-versions

# Display tool configuration information
toolchain info terraform
toolchain info hashicorp/terraform
toolchain info opentofu

# Run a specific version of a tool
toolchain run terraform@1.9.8 -- --version
toolchain run opentofu@1.10.3 -- --version
```

## Architecture

- `main.go` - CLI entry point using Cobra
- `install.go` - Install command with spinner/progress UI
- `run.go` - Run command for executing specific versions
- `tool_versions.go` - .tool-versions file support
- `list.go` - List installed tools
- `installer.go` - Core installation logic
- `aqua_registry.go` - Custom Aqua registry parser
- `http_client.go` - HTTP client with GitHub token authentication
- `local_config.go` - Local tool configuration management
- `types.go` - Core data structures and types

## Commands

### info
The `info` command displays the rendered YAML configuration for a tool, showing how it's configured and what registry it comes from. It uses "latest" as the default version when none is specified.

**Usage:**
```bash
# Using aliases
toolchain info terraform
toolchain info opentofu

# Using canonical org/repo format
toolchain info hashicorp/terraform
toolchain info opentofu/opentofu
```

**Output includes:**
- Tool name and owner/repo
- Tool type (http, github_release)
- Asset template URLs
- Format information
- Binary name
- Raw YAML configuration for debugging

**Note:** The command automatically uses "latest" version to find the tool configuration, which is consistent with how other commands work when no specific version is provided.

## Registry Integration

The tool integrates with the Aqua registry by:

1. **Remote Fetching**: Downloads registry YAML files directly from GitHub
2. **Caching**: Caches registry files locally to avoid repeated downloads
3. **Parsing**: Parses package definitions and version overrides
4. **Template Resolution**: Resolves asset URLs using template functions
5. **Version Handling**: Supports version constraints and overrides

## Recent Improvements

### Duplicate Prevention
- Fixed issue with duplicate entries in `.tool-versions` files
- Ensures consistent tool name format using aliases
- Prevents conflicts between full registry paths and aliases

### GitHub Token Support
- Added intelligent GitHub token authentication
- Only applies tokens to requests that need authentication
- Supports both `ATMOS_GITHUB_TOKEN` and `GITHUB_TOKEN` environment variables
- Excludes raw content URLs from authentication (they're public)

### HTTP Client Improvements
- Custom HTTP client with configurable timeouts
- Automatic User-Agent headers for GitHub API requests
- Proper error handling and retry logic

## Future Enhancements

- Version resolution (latest, semver ranges)
- Platform-specific overrides
- Dependency management
- Integration with Atmos configuration
- Support for more package types
- Enhanced version constraint parsing

## Requirements

- Go 1.21+
- System tools: `unzip`, `tar` (for archive extraction)

## Installation

```bash
cd experiments/toolchain
go mod tidy
go build -o toolchain
```

## Configuration

The tool automatically:
- Caches registry files in `~/.cache/installer/registries`
- Caches downloaded assets in `~/.cache/installer`
- Installs binaries to `./.tools/bin/`

### GitHub Token Support

The toolchain supports GitHub token authentication for GitHub API requests to improve rate limits and access to private repositories.

#### Environment Variables

The toolchain uses Cobra's environment variable binding to automatically use GitHub tokens from the following environment variables (in order of precedence):

1. `ATMOS_GITHUB_TOKEN` - Primary token for Atmos toolchain
2. `GITHUB_TOKEN` - Fallback token (standard GitHub token variable)

The tokens are bound to a hidden `--github-token` flag that can also be set directly if needed.

#### When Tokens Are Used

GitHub tokens are **only** applied to requests that actually need authentication:

- ✅ **GitHub API requests** (`api.github.com`) - Requires authentication for higher rate limits
- ✅ **GitHub release downloads** (`github.com`) - May benefit from authentication for private repos
- ❌ **Raw content URLs** (`raw.githubusercontent.com`) - Public content, no authentication needed

#### Usage

```bash
# Using ATMOS_GITHUB_TOKEN (recommended)
export ATMOS_GITHUB_TOKEN=your_github_token_here
./toolchain install terraform@1.9.8

# Or using GITHUB_TOKEN
export GITHUB_TOKEN=your_github_token_here
./toolchain install terraform@1.9.8

# Or inline for a single command
ATMOS_GITHUB_TOKEN=your_token_here ./toolchain install terraform@1.9.8
```

#### Benefits

- **Higher rate limits**: Authenticated requests have higher rate limits than anonymous requests
- **Access to private repositories**: If any tools are hosted in private repositories
- **Better reliability**: Reduced likelihood of being rate-limited during bulk operations

#### Token Permissions

For most use cases, a GitHub Personal Access Token with the following permissions is sufficient:

- `public_repo` - For public repository access
- `repo` - For private repository access (if needed)

No additional permissions are required for the toolchain's current functionality.

#### Security Notes

- Store tokens securely and never commit them to version control
- Use environment variables or secure credential managers
- Consider using GitHub's fine-grained personal access tokens for minimal permissions
- Rotate tokens regularly for security best practices

### Tool Aliases

The `tools.yaml` file supports an `aliases` section that maps common tool names to their registry paths. This allows you to use simple tool names in `.tool-versions` files:

```yaml
aliases:
  terraform: hashicorp/terraform
  opentofu: opentofu/opentofu
  helm: helm/helm
  kubectl: kubernetes-sigs/kubectl
  tflint: terraform-linters/tflint
  tfsec: aquasecurity/tfsec
  checkov: bridgecrewio/checkov
  terragrunt: gruntwork-io/terragrunt
  packer: hashicorp/packer
  vault: hashicorp/vault
  consul: hashicorp/consul
  nomad: hashicorp/nomad
  waypoint: hashicorp/waypoint
  boundary: hashicorp/boundary
```

With these aliases, you can use simple tool names in your `.tool-versions` file:

```
terraform 1.9.8
opentofu 1.10.3
tflint 0.44.1
```

The tool will automatically resolve these to their full registry paths.

## Troubleshooting

### Common Issues

#### Rate Limiting
If you encounter GitHub rate limiting errors, set a GitHub token:
```bash
export ATMOS_GITHUB_TOKEN=your_token_here
./toolchain install terraform@1.9.8
```

#### Duplicate Tool Entries
If you see duplicate entries in `.tool-versions`, the toolchain now automatically prevents this. Clean up existing duplicates manually:
```bash
# Remove duplicate entries, keeping only alias format
# terraform 1.9.8 1.11.4  # ✅ Correct
# hashicorp/terraform 1.9.8  # ❌ Remove this
```

#### Permission Denied
If you get permission errors when running installed tools:
```bash
# Make sure the .tools/bin directory is in your PATH
export PATH="$PWD/.tools/bin:$PATH"
```

### Debug Mode
Enable debug logging to see detailed information:
```bash
export DEBUG=1
./toolchain install terraform@1.9.8
```

## Contributing

This is a prototype for the Atmos packages feature. The design decisions and implementation choices are documented here to help guide the eventual integration into the main Atmos codebase.
