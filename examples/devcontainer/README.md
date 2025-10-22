# Devcontainer Example

This example demonstrates how to configure devcontainers in Atmos as a replacement for the Geodesic shell wrapper.

## Files

- **`devcontainer.json`** - Geodesic devcontainer configuration (VS Code compatible)
- **`atmos.yaml`** - Atmos configuration with multiple devcontainer definitions

## Usage

### Using the Default Geodesic Devcontainer

```bash
# Create and attach to the default devcontainer (Geodesic)
atmos devcontainer create default --attach

# Inside the container
terraform plan
atmos terraform apply vpc -s ue2-dev
```

### Using Named Devcontainers

```bash
# Create Terraform-specific devcontainer
atmos devcontainer create terraform --attach

# Create Python devcontainer
atmos devcontainer create python --attach
```

### Using Multiple Instances

```bash
# Create multiple instances of the same devcontainer
atmos devcontainer create default --instance alice --attach
atmos devcontainer create default --instance bob --attach

# List all running devcontainers
atmos devcontainer list
```

## Configuration Methods

### Method 1: Import existing devcontainer.json

```yaml
devcontainers:
  default: !include devcontainer.json
```

This imports the VS Code-compatible `devcontainer.json` file. Unsupported fields (like `features`, `customizations`, `postCreateCommand`) are silently ignored and logged at debug level.

### Method 2: Define inline in atmos.yaml

```yaml
devcontainers:
  terraform:
    name: "Terraform Dev"
    image: "hashicorp/terraform:1.6"
    forwardPorts: [8080, 3000]
    # ... more configuration
```

### Method 3: Merge imported config with overrides

```yaml
devcontainers:
  custom:
    <<: !include devcontainer.json
    # Override specific fields
    forwardPorts: [9000, 9001]
```

## Supported Fields

- ✅ `name` - Container display name
- ✅ `image` - Pre-built container image
- ✅ `build` - Build configuration (dockerfile, context, args)
- ✅ `workspaceFolder` - Working directory inside container
- ✅ `workspaceMount` - Primary workspace volume mount
- ✅ `mounts` - Additional volume mounts
- ✅ **`forwardPorts`** - Port forwarding (critical for dev workflows)
- ✅ **`portsAttributes`** - Port metadata (labels, protocols)
- ✅ `containerEnv` - Environment variables
- ✅ `runArgs` - Additional docker/podman arguments
- ✅ `remoteUser` - User to run as

## Unsupported Fields (Ignored)

- ❌ `features` - Use Dockerfile instead
- ❌ `customizations` - Use VS Code extension instead
- ❌ `postCreateCommand` / lifecycle scripts - Use Dockerfile ENTRYPOINT/CMD

## Port Forwarding Examples

### Simple Port Mapping

```yaml
forwardPorts:
  - 8080  # Maps 8080:8080
  - 3000  # Maps 3000:3000
```

### Explicit Port Mapping

```yaml
forwardPorts:
  - "8080:8080"  # Explicit host:container
  - "3001:3000"  # Map host 3001 to container 3000
```

### With Port Attributes

```yaml
forwardPorts:
  - 8080

portsAttributes:
  "8080":
    label: "Web Server"
    protocol: "http"
```

## Commands

```bash
# Create and attach
atmos devcontainer create <name> --attach

# Start existing container
atmos devcontainer start <name>

# Stop running container
atmos devcontainer stop <name>

# Attach to running container
atmos devcontainer attach <name>

# Execute command in container
atmos devcontainer exec <name> -- terraform plan

# List containers
atmos devcontainer list

# Remove container
atmos devcontainer remove <name>

# Show configuration
atmos devcontainer config <name>
```

## Container Naming

Containers are named: `atmos-devcontainer-{name}-{instance}`

Examples:
- `atmos-devcontainer-default-default`
- `atmos-devcontainer-terraform-alice`
- `atmos-devcontainer-python-bob`
