# Custom Dockerfile Devcontainer Example

This example demonstrates using a custom Dockerfile with Atmos devcontainers. The Dockerfile extends the Geodesic base image and pre-installs Atmos.

## Files

- **`atmos.yaml`** - Configuration showing how to use devcontainers with Dockerfile builds
- **`devcontainer.json`** - Devcontainer configuration using `build` instead of `image`
- **`Dockerfile`** - Custom Dockerfile extending Geodesic with Atmos pre-installed

## Configuration

The `devcontainer.json` uses the `build` section to specify how to build the container image:

```json
{
  "build": {
    "dockerfile": "Dockerfile",
    "context": ".",
    "args": {
      "GEODESIC_VERSION": "latest"
    }
  }
}
```

## Build Arguments

The Dockerfile accepts build arguments for customization:

- **`GEODESIC_VERSION`** - Version of Geodesic to use (default: `latest`)

## Usage

### Quick Start

```bash
# From this directory, build and launch the devcontainer
cd examples/devcontainer-build
atmos devcontainer shell geodesic

# Force rebuild (when Dockerfile changes)
atmos devcontainer shell geodesic --replace
```

### Using with atmos.yaml

The included `atmos.yaml` shows two approaches:

**Option 1: Include devcontainer.json file**
```yaml
devcontainer:
  geodesic:
    spec:
      - !include devcontainer.json
```

**Option 2: Define build configuration inline**
```yaml
devcontainer:
  geodesic-inline:
    spec:
      name: "Geodesic with Atmos"
      build:
        dockerfile: "Dockerfile"
        context: "."
        args:
          GEODESIC_VERSION: "latest"
          ATMOS_VERSION: "latest"
      workspaceFolder: "/workspace"
      workspaceMount: "type=bind,source=${localWorkspaceFolder},target=/workspace"
      containerEnv:
        ATMOS_BASE_PATH: "/workspace"
      remoteUser: "root"
```

### Launch the devcontainer

```bash
# Build and launch
atmos devcontainer shell geodesic

# Force rebuild (when Dockerfile changes)
atmos devcontainer shell geodesic --replace

# List available devcontainers
atmos devcontainer list
```

## What's Included

Inside the container, you'll have:

- **Geodesic** - Full Geodesic shell environment
- **Atmos** - Pre-installed Atmos CLI (`atmos version`)
- **Workspace** - Your project mounted at `/workspace`
- **Environment** - `ATMOS_BASE_PATH=/workspace` pre-configured

## Customization

You can customize the Dockerfile to:

- Install additional tools (terraform, kubectl, helm, etc.)
- Add custom scripts or configuration
- Set environment variables
- Install VS Code extensions
- Configure shell aliases

Example additions:

```dockerfile
# Install additional tools
RUN apk add --no-cache \
    terraform \
    kubectl \
    helm

# Add custom shell configuration
COPY .bashrc /root/.bashrc

# Install VS Code extensions (if using VS Code)
RUN code-server --install-extension hashicorp.terraform
```

## Build Process

When you run `atmos devcontainer shell`, Atmos will:

1. **Build the image** (if not already built or if changed)
   - Uses `docker build` or `podman build`
   - Passes build args from `devcontainer.json`
   - Tags the image as `atmos-devcontainer-geodesic`

2. **Create the container** from the built image

3. **Start and attach** to the container

## Rebuilding

To rebuild the image after changing the Dockerfile:

```bash
# Rebuild the devcontainer
atmos devcontainer rebuild geodesic

# Or use --replace flag with shell command
atmos devcontainer shell geodesic --replace
```

## Benefits of Custom Dockerfiles

- **Reproducibility** - Everyone gets the same tools and versions
- **Speed** - Tools are pre-installed, no setup time
- **Consistency** - Same environment across team members
- **Customization** - Add project-specific tools and configuration
- **Version control** - Dockerfile is versioned with your project
