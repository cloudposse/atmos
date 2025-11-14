# PRD: Atmos Devcontainer Command

## Overview

The `atmos devcontainer` command provides native support for Development Containers, enabling teams to launch consistent, reproducible development environments for Atmos components. This command **replaces the Geodesic shell wrapper script** with a native Atmos implementation, providing first-class support for interactive development containers with port binding, volume mounting, and lifecycle management.

This implementation supports a practical subset of the [Development Containers Specification](https://containers.dev/), focusing on the most common use cases while maintaining simplicity and compatibility with both Docker and Podman.

### Geodesic Replacement

The `atmos devcontainer` command is designed as a **drop-in replacement** for the Geodesic shell wrapper:

**Before (Geodesic):**
```bash
# Launch Geodesic shell
./geodesic.sh

# Inside Geodesic container
terraform plan
atmos terraform apply vpc -s ue2-dev
```

**After (Atmos Devcontainer):**
```bash
# Create and attach to devcontainer named "default"
atmos devcontainer create default --attach

# Inside devcontainer (same experience)
terraform plan
atmos terraform apply vpc -s ue2-dev
```

**Key Improvements over Geodesic:**
1. **Named containers**: Each devcontainer has a unique name
2. **Multiple instances**: Launch the same devcontainer config multiple times with different instance names
3. **Native TUI**: Rich terminal UI with progress indicators
4. **Port binding**: First-class support for exposing services
5. **Reattachment**: Easily reconnect to running containers
6. **Multiple runtimes**: Docker or Podman support

## Goals

1. **Replace Geodesic Shell Wrapper**: Provide a native Atmos alternative to the Geodesic shell wrapper script for launching interactive development containers
2. **Named Devcontainers**: Each devcontainer has a unique name defined in configuration (not tied to component/stack)
3. **Multiple Instances**: Support launching the same devcontainer configuration multiple times with different instance names
4. **Container Runtime Flexibility**: Support both Docker and Podman as container runtimes
5. **Lifecycle Management**: Support creating, starting, stopping, and reattaching to devcontainers
6. **Dynamic Dockerfile Generation**: Build container images dynamically based on configuration
7. **Volume Mount Support**: Enable flexible volume mounting for workspace, dependencies, and configuration
8. **Port Binding**: First-class support for exposing container ports to the host (critical for development workflows)
9. **Interactive TUI**: Use Charmbracelet Bubble Tea for rich terminal UI with progress indicators (vendor-style checkmarks)
10. **Testability**: Use interface-based design with mocks for comprehensive testing without requiring container runtimes

## Non-Goals

1. **Full Devcontainer Spec**: We will NOT implement the entire devcontainer specification (features, lifecycle scripts, complex customizations)
2. **VS Code Integration**: We will NOT provide deep VS Code integration (users can use the official VS Code extension)
3. **Orchestration**: We will NOT support docker-compose or multi-container orchestration
4. **Custom Registry Support**: We will NOT support custom container registries (use Docker/Podman directly for this)
5. **Windows Containers**: We will NOT support Windows containers (Linux containers only)

## Supported Devcontainer Specification Features

We will support a **practical subset** of the devcontainer specification:

### ✅ Supported Features

- `name` - Container display name
- `image` - Pre-built container image
- `build.dockerfile` - Path to Dockerfile
- `build.context` - Build context path
- `build.args` - Build-time arguments
- `workspaceFolder` - Working directory inside container
- `workspaceMount` - Primary workspace volume mount
- `mounts` - Additional volume mounts (array)
- `forwardPorts` - **Port forwarding configuration (first-class support)**
- `portsAttributes` - Port metadata (labels, protocol)
- `runArgs` - Additional docker/podman run arguments
- `containerEnv` - Environment variables for the container
- `remoteUser` - User to run as inside container

### ❌ Unsupported Features

- `features` - Pre-packaged tools/utilities (users should use Dockerfile)
- `postCreateCommand` / `postStartCommand` / lifecycle scripts (users should use Dockerfile ENTRYPOINT/CMD)
- `customizations` - Editor-specific customizations (use VS Code devcontainer extension)
- `hostRequirements` - Host system requirements
- `initializeCommand` / `onCreateCommand` - Host-side scripts

## Architecture

### Architectural Decision: Top-Level Configuration

**Decision:** Devcontainers are configured at the **top-level** of `atmos.yaml` under `devcontainer:`, not under `components.devcontainer:` or in stack configuration.

#### Configuration Location Options Evaluated

We evaluated three possible locations for devcontainer configuration:

##### Option 1: Top-Level `devcontainer:` Section (SELECTED)

```yaml
devcontainer:
  geodesic:
    spec: !include devcontainer.json
```

**Pros:**
- ✅ **Clear separation of concerns** - Development environments vs infrastructure components
- ✅ **Not actually components** - Devcontainers are local dev tools, not cloud resources
- ✅ **Different lifecycle** - Components are provisioned to cloud; devcontainers are local
- ✅ **Simpler mental model** - More discoverable and intuitive for users
- ✅ **Industry alignment** - Matches how VS Code and other tools treat devcontainers
- ✅ **Global scope** - Devcontainers are typically used repository-wide, not per-stack
- ✅ **No inheritance needed** - Unlike terraform/helmfile, devcontainers don't need stack-based overrides

**Cons:**
- ⚠️ **Additional top-level config** - Adds another section to atmos.yaml (minor)

##### Option 2: Stack Configuration

```yaml
# stacks/dev.yaml
devcontainer:
  geodesic:
    spec: ...
```

**Pros:**
- ✅ **Stack-specific customization** - Different stacks could have different tooling
- ✅ **Leverage inheritance** - Could use stack inheritance for shared config

**Cons:**
- ❌ **Massive overkill** - Devcontainers are local dev tools, not per-stack infrastructure
- ❌ **Wrong abstraction** - Stacks represent deployment targets; devcontainers are dev tools
- ❌ **Confusing UX** - Users would need stack when launching: `atmos devcontainer shell geodesic -s dev-stack`
- ❌ **Against industry norms** - No other devcontainer tooling uses environment-based configuration
- ❌ **Not deployment-related** - Devcontainers don't deploy to environments

##### Option 3: Under `components.devcontainer:` (REJECTED - Original Implementation)

```yaml
components:
  devcontainer:
    geodesic:
      spec: !include devcontainer.json
```

**Pros:**
- ✅ **Reuses component machinery** - Leverages existing component loading/validation
- ✅ **Namespace organization** - Groups component-like things together

**Cons:**
- ❌ **Conceptual mismatch** - Devcontainers aren't infrastructure components in Atmos's sense
- ❌ **Misleading** - New users might think devcontainers are infrastructure components
- ❌ **Pollutes component namespace** - `atmos list components` could show devcontainers mixed with terraform/helmfile
- ❌ **Wrong categorization** - Components are **deployed resources**; devcontainers are **development tools**

#### Final Decision

**Selected: Option 1 - Top-Level `devcontainer:` Section**

**Rationale:**
1. **Devcontainers are development tools, not infrastructure components** - They run locally, not in cloud environments
2. **Global scope matches usage** - Devcontainers are repository-wide development environments
3. **Industry alignment** - Matches how VS Code (`.devcontainer/`) and other tools structure devcontainer config
4. **Clear separation** - Development environment config vs infrastructure deployment config are fundamentally different
5. **Better discoverability** - Users looking for devcontainer config won't expect it under `components:`
6. **No release delay** - We want this right for v1.200.0, not needing to refactor later

This decision treats devcontainers as first-class development environment configurations, separate from infrastructure components, which aligns with their actual purpose and usage patterns.

### Devcontainer Registry Pattern

Devcontainers are **identified by name only** (not component/stack). The registry manages named devcontainer configurations:

```go
// pkg/devcontainer/registry.go
type Registry interface {
    // Register a named devcontainer configuration
    Register(name string, config *DevcontainerConfig) error

    // Get a devcontainer configuration by name
    Get(name string) (*DevcontainerConfig, error)

    // List all registered devcontainer configurations
    List() ([]DevcontainerInfo, error)
}

// DevcontainerInfo represents a registered devcontainer
type DevcontainerInfo struct {
    Name        string   // Devcontainer name (e.g., "default", "terraform", "python")
    ConfigFile  string   // Path to configuration file
    Image       string   // Container image (if specified)
    Description string   // Optional description
}
```

### Container Runtime Interface

Abstract container operations behind an interface for testability:

```go
// pkg/devcontainer/runtime.go
type Runtime interface {
    // Lifecycle operations
    Build(ctx context.Context, config *BuildConfig) error
    Create(ctx context.Context, config *CreateConfig) (string, error) // returns container ID
    Start(ctx context.Context, containerID string) error
    Stop(ctx context.Context, containerID string, timeout time.Duration) error
    Remove(ctx context.Context, containerID string, force bool) error

    // State inspection
    Inspect(ctx context.Context, containerID string) (*ContainerInfo, error)
    List(ctx context.Context, filters map[string]string) ([]ContainerInfo, error)

    // Execution
    Exec(ctx context.Context, containerID string, cmd []string, opts *ExecOptions) error
    Attach(ctx context.Context, containerID string, opts *AttachOptions) error
}

// Implementations
type DockerRuntime struct{}
type PodmanRuntime struct{}
```

### Configuration Schema

Devcontainers are configured at the **top-level** of `atmos.yaml` under `devcontainer:`. Each devcontainer has a unique name and is NOT tied to a specific component or stack.

#### Schema Structure

```yaml
devcontainer:
  <name>:
    settings:
      runtime: docker|podman  # Optional, auto-detects if not specified
    spec:
      # Devcontainer specification fields (supports !include with overrides)
```

#### Option 1: Define Inline in atmos.yaml

```yaml
# atmos.yaml
devcontainer:
  # Named devcontainer "default" (replaces Geodesic)
  default:
    settings:
      runtime: docker  # Optional: docker, podman, or omit for auto-detect
    spec:
      name: "Atmos Default"
      image: "cloudposse/geodesic:latest"
      workspaceFolder: "/workspace"
      workspaceMount: "type=bind,source=${WORKSPACE},target=/workspace"
      mounts:
        - "type=bind,source=${HOME}/.aws,target=/root/.aws,readonly"
      forwardPorts:
        - 8080
      containerEnv:
        ATMOS_BASE_PATH: "/workspace"
      remoteUser: "root"

  # Named devcontainer "terraform" for Terraform work
  terraform:
    settings:
      runtime: podman  # Use Podman instead of Docker
    spec:
        name: "Terraform Dev"
        image: "hashicorp/terraform:1.6"
        workspaceFolder: "/workspace"
        forwardPorts:
          - 8080
          - 3000
        mounts:
          - "type=bind,source=${HOME}/.aws,target=/root/.aws,readonly"
```

#### Option 2: Import from devcontainer.json with Overrides

Use the `!include` YAML function with YAML merge syntax (`<<:`) to import and override:

```yaml
# atmos.yaml
devcontainer:
  default:
    settings:
      runtime: docker
    spec:
      <<: !include .devcontainer/devcontainer.json

  terraform:
    settings:
      runtime: podman
    spec:
      # Import base configuration and override specific fields
      <<: !include .devcontainer/terraform.json
      image: hashicorp/terraform:1.6  # Override image
      forwardPorts: [8080, 3000]      # Override ports
      containerEnv:
        TF_PLUGIN_CACHE_DIR: "/root/.terraform.d/plugin-cache"
```

#### Option 3: Simple Include (No Overrides)

```yaml
# atmos.yaml
devcontainer:
  default:
    settings:
      runtime: docker
    spec: !include .devcontainer/devcontainer.json
```

#### Unsupported Fields Handling

When importing `devcontainer.json` files, **unsupported fields are silently ignored** and logged at debug level:

```go
// Example: devcontainer.json with unsupported fields
{
  "name": "vpc-dev",
  "image": "hashicorp/terraform:1.6",
  "features": {                    // ❌ Unsupported - logged and ignored
    "ghcr.io/devcontainers/features/aws-cli:1": {}
  },
  "postCreateCommand": "echo hi",  // ❌ Unsupported - logged and ignored
  "forwardPorts": [8080, 3000],    // ✅ Supported - used
  "customizations": {              // ❌ Unsupported - logged and ignored
    "vscode": {
      "extensions": ["hashicorp.terraform"]
    }
  }
}
```

**Debug log output:**
```
DEBUG Ignoring unsupported devcontainer field field=features component=vpc stack=ue2-dev
DEBUG Ignoring unsupported devcontainer field field=postCreateCommand component=vpc stack=ue2-dev
DEBUG Ignoring unsupported devcontainer field field=customizations component=vpc stack=ue2-dev
```

This allows **seamless compatibility with existing VS Code devcontainer.json files** while only using the subset of features that Atmos supports.

#### Configuration Loading Implementation

The configuration loader (`pkg/devcontainer/config_loader.go`) will:

1. **Load Atmos configuration** from `atmos.yaml`
2. **Extract `devcontainer` section** (already processed by YAML parser with `!include`)
3. **Get specific devcontainer by name**
4. **Extract settings (runtime) and spec**
5. **Filter unsupported spec fields** and log at debug level
6. **Validate required fields** (name, image OR build)
7. **Return typed configuration** struct

```go
// pkg/devcontainer/config_loader.go
type DevcontainerConfig struct {
    Name              string            `json:"name" yaml:"name" mapstructure:"name"`
    Image             string            `json:"image,omitempty" yaml:"image,omitempty" mapstructure:"image"`
    Build             *BuildConfig      `json:"build,omitempty" yaml:"build,omitempty" mapstructure:"build"`
    WorkspaceFolder   string            `json:"workspaceFolder,omitempty" yaml:"workspaceFolder,omitempty" mapstructure:"workspacefolder"`
    WorkspaceMount    string            `json:"workspaceMount,omitempty" yaml:"workspaceMount,omitempty" mapstructure:"workspacemount"`
    Mounts            []string          `json:"mounts,omitempty" yaml:"mounts,omitempty" mapstructure:"mounts"`
    ForwardPorts      []interface{}     `json:"forwardPorts,omitempty" yaml:"forwardPorts,omitempty" mapstructure:"forwardports"`
    PortsAttributes   map[string]PortAttr `json:"portsAttributes,omitempty" yaml:"portsAttributes,omitempty" mapstructure:"portsattributes"`
    ContainerEnv      map[string]string `json:"containerEnv,omitempty" yaml:"containerEnv,omitempty" mapstructure:"containerenv"`
    RunArgs           []string          `json:"runArgs,omitempty" yaml:"runArgs,omitempty" mapstructure:"runargs"`
    RemoteUser        string            `json:"remoteUser,omitempty" yaml:"remoteUser,omitempty" mapstructure:"remoteuser"`
}

type DevcontainerSettings struct {
    Runtime string `yaml:"runtime,omitempty" json:"runtime,omitempty" mapstructure:"runtime"`
}

func LoadDevcontainerConfig(
    atmosConfig *schema.AtmosConfiguration,
    name string,
) (*DevcontainerConfig, *DevcontainerSettings, error) {
    // 1. Get devcontainer section from atmos.yaml
    // This is already processed by YAML parser with !include directives
    if atmosConfig.Components.Devcontainer == nil {
        return nil, nil, fmt.Errorf("%w: no devcontainers configured in atmos.yaml", errUtils.ErrDevcontainerNotFound)
    }

    // 2. Get specific devcontainer by name
    devcontainerData, ok := devcontainers[name].(map[string]interface{})
    if !ok {
        return nil, fmt.Errorf("%w: devcontainer '%s' not found", errUtils.ErrDevcontainerNotFound, name)
    }

    // 3. Filter unsupported fields and convert to typed config
    config := &DevcontainerConfig{}
    if err := filterAndUnmarshal(devcontainerData, config, name); err != nil {
        return nil, err
    }

    // 4. Validate required fields
    if err := validateConfig(config); err != nil {
        return nil, err
    }

    return config, nil
}

// Filter unsupported fields and log them
func filterAndUnmarshal(
    data map[string]interface{},
    config *DevcontainerConfig,
    name string,
) error {
    supportedFields := map[string]bool{
        "name": true, "image": true, "build": true,
        "workspaceFolder": true, "workspaceMount": true, "mounts": true,
        "forwardPorts": true, "portsAttributes": true,
        "containerEnv": true, "runArgs": true, "remoteUser": true,
    }

    // Log unsupported fields
    for key := range data {
        if !supportedFields[key] {
            log.Debug("Ignoring unsupported devcontainer field",
                "field", key,
                "devcontainer", name)
        }
    }

    // Convert to typed struct (only supported fields)
    // Uses existing Atmos YAML/JSON utilities
    return u.UnmarshalYAML(data, config)
}
```

### Container Naming Convention

Containers are named using the pattern: `atmos-devcontainer-{name}-{instance}`

- `name`: Devcontainer name from configuration (e.g., `default`, `terraform`, `python`)
- `instance`: Optional instance identifier (defaults to `default` if not specified)

**Examples:**
```bash
# Default instance
atmos devcontainer create default
# Creates: atmos-devcontainer-default-default

# Named instance
atmos devcontainer create default --instance prod
# Creates: atmos-devcontainer-default-prod

# Same devcontainer, different instances
atmos devcontainer create terraform --instance alice
atmos devcontainer create terraform --instance bob
# Creates: atmos-devcontainer-terraform-alice
#          atmos-devcontainer-terraform-bob
```

**Why instances?** Multiple developers can run the same devcontainer configuration with different instance names, or a single developer can run multiple instances for different purposes (e.g., `dev`, `test`, `debug`).

### Container Labels

All containers will be labeled for easy identification:

```
com.atmos.type=devcontainer
com.atmos.devcontainer.name=default
com.atmos.devcontainer.instance=default
com.atmos.workspace=/path/to/workspace
com.atmos.created=2025-10-21T10:30:00Z
```

## Command Structure

### Main Command

```bash
atmos devcontainer [subcommand] [flags]
```

### Subcommands

#### `atmos devcontainer create`

Create and optionally start a new devcontainer.

```bash
atmos devcontainer create <name> [flags]

Flags:
      --instance string       Instance name (default: "default")
      --build                 Force rebuild of container image
      --no-start              Create but don't start the container
      --attach                Attach to container after creation (implies --start)
      --runtime string        Container runtime: docker|podman (auto-detected)
```

**Examples:**

```bash
# Create and attach to default devcontainer (default instance)
atmos devcontainer create default --attach

# Create with named instance
atmos devcontainer create default --instance prod --attach

# Create but don't start
atmos devcontainer create terraform --no-start

# Force rebuild with named instance
atmos devcontainer create terraform --instance alice --build
```

#### `atmos devcontainer start`

Start an existing stopped devcontainer.

```bash
atmos devcontainer start <name> [flags]

Flags:
      --instance string       Instance name (default: "default")
      --attach                Attach after starting
```

**Examples:**

```bash
# Start default instance
atmos devcontainer start default

# Start named instance and attach
atmos devcontainer start default --instance prod --attach
```

#### `atmos devcontainer stop`

Stop a running devcontainer.

```bash
atmos devcontainer stop <name> [flags]

Flags:
      --instance string       Instance name (default: "default")
      --timeout int           Timeout in seconds (default: 10)
```

**Examples:**

```bash
# Stop default instance
atmos devcontainer stop default

# Stop named instance with custom timeout
atmos devcontainer stop terraform --instance alice --timeout 30
```

#### `atmos devcontainer attach`

Attach to a running devcontainer (launches shell).

```bash
atmos devcontainer attach <name> [flags]

Flags:
      --instance string       Instance name (default: "default")
      --shell string          Shell to use (default: /bin/bash)
      --user string           User to run as (overrides remoteUser)
```

**Examples:**

```bash
# Attach to default instance
atmos devcontainer attach default

# Attach to named instance with zsh
atmos devcontainer attach terraform --instance alice --shell /bin/zsh
```

#### `atmos devcontainer shell`

Launch a shell in a devcontainer (convenience command - alias for `start --attach`).

This command is consistent with other Atmos shell commands (`terraform shell`, `auth shell`) and provides a quick way to launch an interactive development environment.

```bash
atmos devcontainer shell <name> [flags]

Flags:
      --instance string       Instance name (default: "default")
```

**Examples:**

```bash
# Launch shell in default instance (starts if stopped)
atmos devcontainer shell default

# Launch shell in named instance
atmos devcontainer shell terraform --instance alice
```

**Note:** This command will:
1. Start the container if it's stopped
2. Create the container if it doesn't exist
3. Attach to the container with an interactive shell

This makes it the quickest way to get into a devcontainer environment, equivalent to running `atmos devcontainer start <name> --attach`.

#### `atmos devcontainer exec`

Execute a command in a running devcontainer.

```bash
atmos devcontainer exec <name> [flags] -- <command> [args...]

Flags:
      --instance string       Instance name (default: "default")
      --user string           User to run as (overrides remoteUser)
```

**Examples:**

```bash
# Run terraform plan in default instance
atmos devcontainer exec default -- terraform plan

# Run as different user in named instance
atmos devcontainer exec terraform --instance alice --user terraform -- whoami
```

#### `atmos devcontainer list`

List all Atmos devcontainers.

```bash
atmos devcontainer list [flags]

Flags:
      --all                   Show all containers (default: running only)
      --format string         Output format: table|json|yaml (default: table)
      --name string           Filter by devcontainer name
      --instance string       Filter by instance name
```

**Examples:**

```bash
# List running containers
atmos devcontainer list

# List all containers (including stopped)
atmos devcontainer list --all

# List all instances of "default" devcontainer
atmos devcontainer list --name default

# JSON output
atmos devcontainer list --format json
```

#### `atmos devcontainer remove`

Remove a devcontainer.

```bash
atmos devcontainer remove <name> [flags]

Flags:
      --instance string       Instance name (default: "default")
      --force                 Force removal even if running
```

**Examples:**

```bash
# Remove default instance
atmos devcontainer remove default

# Force remove named instance
atmos devcontainer remove terraform --instance alice --force
```

#### `atmos devcontainer config`

Show devcontainer configuration.

```bash
atmos devcontainer config <name> [flags]

Flags:
      --format string         Output format: yaml|json (default: yaml)
```

**Examples:**

```bash
# Show configuration for "default" devcontainer
atmos devcontainer config default

# JSON output
atmos devcontainer config terraform --format json
```

## Terminal UI (TUI) with Charmbracelet

### TUI Requirements

All devcontainer operations will use **Charmbracelet Bubble Tea** for interactive terminal UI, following the vendor model pattern:

1. **TTY Detection**: Check for TTY using `term.IsTTYSupportForStdout()` before rendering TUI
2. **Fallback Mode**: When no TTY is detected (piped commands, CI/CD), fall back to structured logging
3. **Progress Indicators**: Use spinner and progress bar from vendor model
4. **Checkmark Style**: Use `theme.Styles.Checkmark` (✓) for successful operations
5. **X Mark Style**: Use `theme.Styles.XMark` (✗) for failed operations
6. **Consistent Theming**: Use `pkg/ui/theme` colors and styles throughout

### TUI Operations

Each devcontainer operation displays progress with checkmarks:

#### Container Creation Flow
```
⠋ Building image vpc-dev                           [=====>    ] 2/4
✓ Pulled base image hashicorp/terraform:1.6
✓ Built image vpc-dev (sha256:abc123...)
⠋ Creating container atmos-devcontainer-vpc-ue2-dev-a1b2c3d4
```

#### Container Start Flow
```
⠋ Starting container atmos-devcontainer-vpc-ue2-dev-a1b2c3d4
✓ Container started
✓ Port 8080 bound to localhost:8080
✓ Port 3000 bound to localhost:3000
✓ Port 5432 bound to localhost:5432
```

#### Volume Mount Flow
```
⠋ Mounting volumes                                 [===>      ] 1/3
✓ Mounted /Users/erik/workspace/vpc → /workspace
✓ Mounted /Users/erik/.aws → /root/.aws (readonly)
✓ Created volume terraform-cache → /root/.terraform.d
```

### List Command TUI

The `atmos devcontainer list` command follows the look and feel of other Atmos list commands:

```
NAME           INSTANCE    STATUS     PORTS                    CREATED        IMAGE
default        default     Running    8080                     2 hours ago    cloudposse/geodesic:latest
default        prod        Running    8080                     1 day ago      cloudposse/geodesic:latest
terraform      alice       Running    8080, 3000, 5432         5 minutes ago  hashicorp/terraform:1.6
terraform      bob         Stopped    -                        3 days ago     hashicorp/terraform:1.6
python         default     Running    8000                     1 hour ago     python:3.11
```

### Non-TTY Fallback

When no TTY is detected, operations use structured logging instead:

```
INFO  Building image component=vpc-dev
INFO  ✓ image=hashicorp/terraform:1.6 status=pulled
INFO  ✓ image=vpc-dev status=built sha=abc123...
INFO  Creating container name=atmos-devcontainer-vpc-ue2-dev-a1b2c3d4
INFO  ✓ container=atmos-devcontainer-vpc-ue2-dev-a1b2c3d4 status=created
```

### TUI Model Architecture

The TUI model follows the vendor model pattern from `internal/exec/vendor_model.go`:

```go
// pkg/devcontainer/model.go
type operationType int

const (
    opBuild operationType = iota
    opCreate
    opStart
    opMount
    opPortBind
)

type operationStep struct {
    name        string
    description string
    opType      operationType
    err         error
}

type modelDevcontainer struct {
    steps       []operationStep
    index       int
    width       int
    height      int
    spinner     spinner.Model
    progress    progress.Model
    done        bool
    failedSteps int
    atmosConfig *schema.AtmosConfiguration
    isTTY       bool
}

// Implements tea.Model interface
func (m *modelDevcontainer) Init() tea.Cmd
func (m *modelDevcontainer) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m *modelDevcontainer) View() string
```

#### Step Execution Pattern

Each operation (create, start, stop) defines its steps:

```go
// Example: Container creation steps
steps := []operationStep{
    {name: "Pull base image", opType: opBuild},
    {name: "Build image", opType: opBuild},
    {name: "Create container", opType: opCreate},
    {name: "Mount volumes", opType: opMount},
    {name: "Bind ports", opType: opPortBind},
    {name: "Start container", opType: opStart},
}

// Execute with TUI
if err := executeDevcontainerModel(steps, atmosConfig); err != nil {
    return err
}
```

#### Message Types

```go
type completedStepMsg struct {
    err    error
    index  int
    detail string // Additional info (e.g., "sha256:abc123...")
}

type portBoundMsg struct {
    containerPort int
    hostPort      int
    protocol      string
}

type volumeMountedMsg struct {
    source string
    target string
    readonly bool
}
```

## Container Runtime Detection

The command will auto-detect the available container runtime:

1. Check for `ATMOS_DEVCONTAINER_RUNTIME` environment variable
2. Check for `docker` in PATH and verify it's running (`docker info`)
3. Check for `podman` in PATH and verify it's running (`podman info`)
4. Fall back to error if neither is available

## Volume Mount Variable Substitution

Volume mount paths support variable substitution:

- `${componentPath}` - Absolute path to component directory
- `${stackPath}` - Absolute path to stack configuration directory
- `${atmosConfig}` - Path to atmos.yaml
- `${HOME}` - User home directory
- `${WORKSPACE}` - Atmos workspace root
- Any environment variable using `${VAR_NAME}` syntax

## Dockerfile Generation

When `build.dockerfile` is specified but the file doesn't exist, Atmos can optionally generate a basic Dockerfile:

```dockerfile
# Auto-generated by Atmos
ARG BASE_IMAGE=ubuntu:22.04
FROM ${BASE_IMAGE}

# Install common tools
RUN apt-get update && apt-get install -y \
    curl \
    git \
    vim \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /workspace

# Default command
CMD ["/bin/bash"]
```

## Testing Strategy

### Interface Mocking

All external dependencies will be mocked:

```go
//go:generate go run go.uber.org/mock/mockgen@latest -source=runtime.go -destination=mock_runtime_test.go -package=devcontainer

func TestDevcontainerCreate(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockRuntime := NewMockRuntime(ctrl)
    mockRuntime.EXPECT().
        Create(gomock.Any(), gomock.Any()).
        Return("container-id-123", nil)

    // Test logic
}
```

### Test Coverage Requirements

- Unit tests for all public functions (80% coverage minimum)
- Integration tests using mock runtime (no Docker/Podman required)
- CLI command tests using `cmd.NewTestKit(t)`
- Table-driven tests for configuration parsing and validation

### Test Cases

1. **Configuration Loading**:
   - Parse devcontainer config from component metadata (inline YAML)
   - Import `devcontainer.json` using `!include` function
   - Merge imported config with overrides
   - Handle `configFile` path references
   - Ignore unsupported fields with debug logging
2. **Runtime Detection**: Auto-detect Docker vs Podman
3. **Container Lifecycle**: Create, start, stop, remove operations
4. **Reattachment**: Find and reattach to existing containers
5. **Volume Mounts**: Variable substitution and mount creation
6. **Port Forwarding**:
   - Simple port mapping (8080 → 8080:8080)
   - Explicit port mapping (3000:3000)
   - Port attributes (labels, protocols)
7. **Error Handling**: Missing configuration, runtime errors, invalid inputs
8. **Multiple Containers**: Multiple components/stacks running simultaneously
9. **devcontainer.json Compatibility**:
   - Import valid devcontainer.json files
   - Silently ignore unsupported fields
   - Log ignored fields at debug level
   - Support both JSON and YAML formats via `!include`

## Implementation Plan

### Phase 1: Core Infrastructure & TUI (Week 1)

1. Create `pkg/devcontainer/` package structure
2. Define `Runtime` interface and `Registry` interface
3. Implement runtime detection logic (`docker` vs `podman`)
4. Implement TUI model (`pkg/devcontainer/model.go`) following vendor pattern
5. Add TTY detection and non-TTY fallback
6. Add configuration schema to component metadata
7. Implement configuration loading with `!include` support:
   - Parse inline YAML configuration
   - Support `!include` for `devcontainer.json` files
   - Support `configFile` path references
   - Implement unsupported field filtering with debug logging
8. Implement port forwarding logic (`pkg/devcontainer/ports.go`)
9. Generate mocks for testing (`//go:generate mockgen`)

### Phase 2: Container Runtime Implementation (Week 2)

1. Implement Docker runtime (`pkg/devcontainer/docker_runtime.go`)
   - Build, Create, Start, Stop, Remove operations
   - Port binding support
   - Volume mounting
   - Container inspection and listing
2. Implement Podman runtime (`pkg/devcontainer/podman_runtime.go`)
   - Same interface as Docker runtime
   - Handle Podman-specific differences
3. Add container naming and labeling logic
4. Implement volume mount variable substitution
5. Add basic error handling with static errors

### Phase 3: Core Commands with TUI (Week 3)

1. Implement `atmos devcontainer create` command
   - TUI with progress indicators
   - Port binding display
   - Volume mount display
   - Support `--attach` flag
2. Implement `atmos devcontainer start` command
   - TUI for startup sequence
   - Port binding verification
3. Implement `atmos devcontainer attach` command
   - Interactive shell attachment
   - TTY pass-through
4. Implement `atmos devcontainer stop` command
   - Graceful shutdown with timeout
5. Add container reattachment detection

### Phase 4: List, Config, and Management (Week 4)

1. Implement `atmos devcontainer list` command
   - Table formatter (Atmos list style)
   - JSON/YAML output formats
   - Port display in list view
2. Implement `atmos devcontainer exec` command
   - Execute commands in running containers
3. Implement `atmos devcontainer remove` command
   - Force removal support
4. Implement `atmos devcontainer config` command
   - Display resolved configuration
5. Add Dockerfile generation support

### Phase 5: Testing & Documentation (Week 5)

1. Write comprehensive unit tests (80% coverage target)
   - Mock runtime tests (no Docker/Podman required)
   - TUI model tests
   - Configuration parsing tests
2. Write integration tests with mocks
3. Add CLI command tests using `cmd.NewTestKit(t)`
4. Create Docusaurus documentation
   - Command reference pages
   - Port binding examples
   - Geodesic migration guide
5. Add example devcontainer configurations
6. Create blog post announcement

## File Structure

```
pkg/devcontainer/
├── registry.go               # Component registry interface
├── registry_impl.go          # Registry implementation
├── runtime.go                # Runtime interface
├── docker_runtime.go         # Docker implementation
├── podman_runtime.go         # Podman implementation
├── runtime_detector.go       # Auto-detection logic
├── config.go                 # Configuration types
├── config_loader.go          # Load config from component
├── naming.go                 # Container naming conventions
├── mounts.go                 # Volume mount handling
├── ports.go                  # Port forwarding handling
├── variables.go              # Variable substitution
├── model.go                  # Bubble Tea TUI model (vendor-style)
├── list_formatter.go         # List command formatting
├── mock_runtime_test.go      # Generated mocks
└── *_test.go                 # Test files

cmd/devcontainer/
├── devcontainer.go           # Main command (registry pattern)
├── create.go                 # Create subcommand
├── start.go                  # Start subcommand
├── stop.go                   # Stop subcommand
├── attach.go                 # Attach subcommand
├── exec.go                   # Exec subcommand
├── list.go                   # List subcommand
├── remove.go                 # Remove subcommand
├── config.go                 # Config subcommand
└── *_test.go                 # Command tests

pkg/devcontainer/
├── lifecycle.go              # Business logic (Start, Stop, Attach, Exec, List, etc.)
├── operations.go             # Container operations (create, remove, build, pull)
├── identity.go               # Identity injection logic (authentication support)
└── *_test.go                 # Tests

website/docs/cli/commands/devcontainer/
├── index.mdx                 # Overview
├── create.mdx                # Create command docs
├── start.mdx                 # Start command docs
├── stop.mdx                  # Stop command docs
├── attach.mdx                # Attach command docs
├── exec.mdx                  # Exec command docs
├── list.mdx                  # List command docs
├── remove.mdx                # Remove subcommand docs
└── config.mdx                # Config command docs
```

## Success Metrics

1. **Adoption**: 20% of Atmos users use devcontainer command within 3 months
2. **Test Coverage**: 80%+ code coverage for devcontainer package
3. **Documentation**: Complete documentation with examples
4. **Performance**: Container creation <30 seconds for typical components
5. **Reliability**: 95%+ success rate for container operations

## Future Enhancements

1. **Devcontainer Templates**: Pre-built templates for common component types (Terraform, Helmfile, etc.)
2. **Remote Containers**: Support for remote Docker/Podman hosts
3. **Container Snapshots**: Save and restore container states
4. **GPU Support**: Enable GPU passthrough for ML workloads
5. **Custom Networks**: Create and manage custom networks for multi-container scenarios
6. **Health Checks**: Built-in health checking for containers
7. **Auto-sync**: Watch local files and sync to container

## Security Considerations

1. **Volume Mount Safety**: Validate mount paths to prevent directory traversal
2. **Privileged Containers**: Warn when `--privileged` is used
3. **Secret Management**: Support Atmos store integration for secrets
4. **Network Isolation**: Default to isolated networks unless `--network=host`
5. **User Permissions**: Run as non-root by default when possible

## Complete Example: Importing VS Code devcontainer.json

This example shows how to use an existing VS Code `devcontainer.json` file with Atmos:

**Project structure:**
```
components/terraform/vpc/
├── .devcontainer/
│   └── devcontainer.json          # VS Code devcontainer config
├── component.yaml                 # Atmos component metadata
├── main.tf
└── variables.tf
```

**Existing devcontainer.json (VS Code):**
```json
{
  "name": "Terraform VPC Dev",
  "image": "hashicorp/terraform:1.6",
  "features": {
    "ghcr.io/devcontainers/features/aws-cli:1": {}
  },
  "forwardPorts": [8080, 3000],
  "portsAttributes": {
    "8080": {
      "label": "Web Server",
      "protocol": "http"
    }
  },
  "mounts": [
    "source=${localEnv:HOME}/.aws,target=/root/.aws,type=bind,readonly"
  ],
  "customizations": {
    "vscode": {
      "extensions": ["hashicorp.terraform"]
    }
  },
  "postCreateCommand": "terraform init"
}
```

**Atmos component.yaml (imports devcontainer.json):**
```yaml
# components/terraform/vpc/component.yaml
metadata:
  component: vpc
  type: terraform

  # Import VS Code devcontainer.json with !include
  devcontainer: !include .devcontainer/devcontainer.json

vars:
  enabled: true
  name: vpc
```

**What happens when you run `atmos devcontainer create vpc -s ue2-dev`:**

1. ✅ Atmos loads `component.yaml`
2. ✅ `!include` directive loads `.devcontainer/devcontainer.json`
3. ✅ Supported fields are used:
   - `name: "Terraform VPC Dev"`
   - `image: "hashicorp/terraform:1.6"`
   - `forwardPorts: [8080, 3000]`
   - `portsAttributes` for port 8080
   - `mounts` for AWS credentials
4. ⚠️ Unsupported fields are logged and ignored:
   - `features` (logged: "Ignoring unsupported devcontainer field field=features")
   - `customizations` (logged: "Ignoring unsupported devcontainer field field=customizations")
   - `postCreateCommand` (logged: "Ignoring unsupported devcontainer field field=postCreateCommand")
5. ✅ Container created with working configuration

**Result:** Your existing VS Code devcontainer.json works seamlessly with Atmos, using the subset of supported features!

## References

- [Development Containers Specification](https://containers.dev/)
- [Docker CLI Reference](https://docs.docker.com/engine/reference/commandline/cli/)
- [Podman CLI Reference](https://docs.podman.io/en/latest/Commands.html)
- [Atmos Component Registry Pattern](docs/prd/component-registry-pattern.md)
- [Atmos Command Registry Pattern](docs/prd/command-registry-pattern.md)
- [Atmos YAML Functions](https://atmos.tools/core-concepts/stacks/templates) - `!include` documentation
