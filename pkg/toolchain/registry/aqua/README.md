# Aqua Registry Integration

This package provides Atmos integration with the [Aqua project](https://aquaproj.github.io/)'s comprehensive tool registry.

## Why We Re-implemented a Subset of Aqua

While Aqua is an excellent tool management solution, we chose to implement a custom subset rather than use Aqua directly for the following reasons:

### 1. **No Stable Interface Commitment**

The Aqua project hasn't committed to a stable Go API or library interface. Aqua is primarily designed as a standalone CLI tool, not as a reusable library. Integrating Aqua directly would mean:
- Depending on internal implementation details that could change without notice
- Potential breakage with Aqua updates
- Tight coupling to Aqua's release cycle

### 2. **Optimized for Atmos Integration**

Our implementation is specifically tailored for Atmos's needs:
- **Lightweight**: We only implement the features needed for Atmos tool management
- **Integrated**: Deep integration with Atmos's configuration system and workflows
- **Flexible**: Custom version resolution logic that fits Atmos's multi-environment model
- **Local-first**: Support for local tool definitions and version constraints

### 3. **Different Project Visions**

While Aqua and Atmos share the goal of simplifying tool management, our visions differ:

**Aqua's Vision:**
- Comprehensive tool manager for any development environment
- Standalone binary with its own configuration format
- Focus on reproducible environments through version pinning
- Broad tool support through community registry

**Atmos's Vision:**
- Infrastructure orchestration platform with integrated tool management
- Tool management as one feature among many (stacks, components, workflows)
- Focus on cloud infrastructure tools (Terraform, Helmfile, etc.)
- Configuration integrated with Atmos's existing YAML-based config system

### 4. **Leveraging Aqua's Registry Metadata**

Despite implementing our own tool installer, we heavily leverage Aqua's excellent work:
- **Aqua Registry**: We fetch tool metadata from the [aqua-registry](https://github.com/aquaproj/aqua-registry)
- **Tool Definitions**: Asset URLs, version constraints, and platform-specific overrides
- **Community Contributions**: Benefit from the Aqua community's registry contributions

This gives us the best of both worlds - a custom implementation optimized for Atmos while benefiting from Aqua's comprehensive and well-maintained tool registry.

## Architecture

Our implementation follows the **Registry Pattern** to allow for extensibility:

```
toolchain/
├── registry/              # Registry abstraction layer
│   ├── registry.go        # ToolRegistry interface + shared types
│   └── aqua/              # Aqua registry implementation
│       ├── aqua.go        # Implementation of ToolRegistry interface
│       └── aqua_test.go   # Comprehensive tests
└── ...                    # Toolchain installer and commands
```

This design allows us to:
- Add alternative registries in the future (local-only, custom URLs, etc.)
- Test the toolchain independently of registry implementations
- Swap registry backends without changing toolchain code

## Features

### Aqua Registry Integration
- Fetches tool metadata from https://github.com/aquaproj/aqua-registry
- Supports both `packages` (Aqua format) and `tools` (legacy format) YAML structures
- Caches registry files locally to reduce network requests

### Local Configuration Override
- Supports `tools.yaml` for local tool definitions
- Version constraints with semver matching
- Aliasing tools for shorter names (e.g., `tf` → `hashicorp/terraform`)

### Version Resolution
- Latest version fetching from GitHub releases
- Semver constraint matching for version-specific asset templates
- Platform-specific overrides (GOOS/GOARCH)

### Asset URL Templates
- Go template support for dynamic URL construction
- Template functions: `trimV`, `OS`, `Arch`, `Version`
- Support for both HTTP direct downloads and GitHub releases

## Example Usage

```go
// Create registry client
registry := aqua.NewAquaRegistry()

// Load local overrides
if err := registry.LoadLocalConfig("tools.yaml"); err != nil {
    log.Fatal(err)
}

// Get tool metadata
tool, err := registry.GetTool("hashicorp", "terraform")
if err != nil {
    log.Fatal(err)
}

// Get latest version
version, err := registry.GetLatestVersion("hashicorp", "terraform")
if err != nil {
    log.Fatal(err)
}

// Build download URL
url, err := registry.BuildAssetURL(tool, version)
if err != nil {
    log.Fatal(err)
}
```

## Testing

The package includes comprehensive unit tests that mock HTTP responses:
```bash
go test ./toolchain/registry/aqua -v
```

Tests cover:
- Local configuration loading
- Remote registry fetching with fallback
- Version resolution and constraints
- Asset URL template rendering
- Platform-specific overrides
- Error handling and edge cases

## Future Enhancements

Potential improvements while maintaining our custom implementation:
- Support for additional registries (asdf, homebrew formulas, etc.)
- Tool verification via checksums (already supported in registry metadata)
- Air-gapped registry mirroring for offline use
- Custom registry URLs for private/corporate registries

## Credits

This implementation was inspired by and leverages the excellent work of:
- [Aqua Project](https://aquaproj.github.io/) - Tool registry and metadata
- [aqua-registry](https://github.com/aquaproj/aqua-registry) - Comprehensive tool definitions
