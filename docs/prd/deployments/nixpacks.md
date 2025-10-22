# Atmos Deployments - Nixpacks Integration

This document describes the integration of nixpacks for container builds in Atmos deployments.

## Overview

Nixpacks is a build system that creates container images from source code with zero configuration. It automatically detects the language/framework and creates an optimized container image.

**Supported Languages**: Go, Node.js, Python, Rust, Ruby, PHP, Java, .NET, and more

## Nixpack Component Type

Nixpack becomes a new first-class component type in Atmos, alongside Terraform, Helmfile, and Lambda.

### Configuration Example

```yaml
deployment:
  name: api
  components:
    nixpack:
      api:
        metadata:
          labels:
            service: api
            tier: backend
          depends_on:
            - terraform/ecr/api
        vars:
          source: "./services/api"          # Path to source code
          install_cmd: "go mod download"    # Optional override
          build_cmd: "go build -o main ."   # Optional override
          start_cmd: "./main"               # Optional override
          pkgs: ["ffmpeg", "imagemagick"]   # Additional Nix packages
          apt_pkgs: ["curl"]                # Additional apt packages
          image:
            registry: "123456789012.dkr.ecr.us-east-1.amazonaws.com"
            name: "api"
            tag: "{{ git.sha }}"           # Advisory; digest used in rollout
        settings:
          nixpack:
            publish: true                   # Push to registry (default: true)
            dockerfile_escape: false        # Use Dockerfile if present
            sbom:
              require: true
              formats: ["cyclonedx-json", "spdx-json"]
```

## Build Process

```bash
# Build containers for dev target
atmos deployment build payment-service --target dev

# Build process:
# 1. Detect language/framework from source code
# 2. Generate build plan (install, build, start commands)
# 3. Build container image using nixpacks Go SDK
# 4. Generate SBOM (Software Bill of Materials)
# 5. Push image to registry
# 6. Output: sha256:abc123... (content-addressable digest)
```

## Nixpacks Go SDK Integration

Atmos uses the nixpacks Go SDK directly - no shelling out to nixpacks CLI:

```go
// pkg/nixpack/builder.go
package nixpack

import (
    "context"
    "github.com/railwayapp/nixpacks/pkg/nixpacks"
)

type Builder struct {
    config *BuildConfig
}

func (b *Builder) Build(ctx context.Context) (*BuildResult, error) {
    // Create nixpacks options
    opts := nixpacks.BuildOptions{
        Path:        b.config.Source,
        Name:        b.config.Image.Name,
        Tags:        []string{b.config.Image.Tag},
        Out:         b.config.OutputDir,
        Pkgs:        b.config.Pkgs,
        AptPkgs:     b.config.AptPkgs,
        InstallCmd:  b.config.InstallCmd,
        BuildCmd:    b.config.BuildCmd,
        StartCmd:    b.config.StartCmd,
    }

    // Run build
    result, err := nixpacks.Build(ctx, opts)
    if err != nil {
        return nil, fmt.Errorf("nixpacks build failed: %w", err)
    }

    // Generate SBOM
    sbom, err := b.generateSBOM(ctx, result)

    return &BuildResult{
        Digest:   result.ImageDigest,
        SBOM:     sbom,
        Provider: result.DetectedProvider,  // "go", "node", "python", etc.
    }, nil
}
```

## Auto-Detection

Nixpacks automatically detects the language and framework:

| Language   | Detection Method | Example Files |
|------------|------------------|---------------|
| Go         | `go.mod` file    | `go.mod`, `main.go` |
| Node.js    | `package.json`   | `package.json`, `index.js` |
| Python     | `requirements.txt`, `Pipfile`, `pyproject.toml` | `requirements.txt` |
| Rust       | `Cargo.toml`     | `Cargo.toml`, `src/main.rs` |
| Ruby       | `Gemfile`        | `Gemfile`, `config.ru` |
| PHP        | `composer.json`  | `composer.json`, `index.php` |

## Dockerfile Escape Hatch

If nixpacks doesn't meet requirements, use a Dockerfile:

```yaml
components:
  nixpack:
    api:
      vars:
        source: "./services/api"
      settings:
        nixpack:
          dockerfile_escape: true  # Use Dockerfile if present
```

When enabled, Atmos checks for `Dockerfile` in source directory. If present, uses Docker build instead of nixpacks.

## Image Digest Binding

Built container digests are automatically bound to Terraform/Helm components:

```yaml
components:
  nixpack:
    api:
      vars:
        image:
          name: "api"

  terraform:
    ecs/taskdef-api:
      metadata:
        labels:
          service: api  # Binds to nixpack component via label
      vars:
        container:
          name: "api"
          # Atmos automatically injects:
          # image = "123456789012.dkr.ecr.us-east-1.amazonaws.com/api@sha256:abc123..."
```

## Reproducible Builds

Nixpacks ensures reproducible builds:
- Same source code → same container image (same digest)
- Works identically locally and in CI
- Nix package manager provides hermetic builds
- Locked dependencies via `go.mod`, `package-lock.json`, etc.

## CLI Commands

```bash
# Build all nixpack components for target
atmos deployment build api --target dev

# Build specific component
atmos deployment build api --target dev --component api

# Build without pushing to registry
atmos deployment build api --target dev --skip-push

# Show build plan without building
atmos deployment build api --target dev --plan
```

## See Also

- **[overview.md](./overview.md)** - Core concepts and build stage
- **[configuration.md](./configuration.md)** - Nixpack component schema
- **[sbom.md](./sbom.md)** - SBOM generation from nixpack builds
- **[cli-commands.md](./cli-commands.md)** - Complete command reference
