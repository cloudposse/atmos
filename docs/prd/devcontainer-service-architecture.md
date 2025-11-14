# DevContainer Service Architecture

## Problem with One-Off Structs

**Bad:** Creating specialized structs for each tiny problem
```go
type NameResolver struct { ... }      // Just for getting names
type InstanceFinder struct { ... }    // Just for finding instances
type ConfigValidator struct { ... }   // Just for validation
```

This leads to:
- Proliferation of single-purpose types
- No cohesive architecture
- Hard to understand the system as a whole

## Solution: Service Layer Architecture

Create a **DevContainerService** that handles ALL devcontainer operations with injected dependencies.

### Core Architecture

```go
// Service is the main entry point for all devcontainer operations.
// It coordinates between config, runtime, and UI concerns.
type Service struct {
    // Core dependencies
    config   ConfigProvider
    runtime  RuntimeProvider
    ui       UIProvider

    // Derived state
    atmosConfig *schema.AtmosConfiguration
}

// ConfigProvider abstracts configuration loading and parsing.
type ConfigProvider interface {
    LoadAtmosConfig() (*schema.AtmosConfiguration, error)
    ListDevcontainers(config *schema.AtmosConfiguration) ([]string, error)
    GetDevcontainerConfig(config *schema.AtmosConfiguration, name string) (*DevcontainerConfig, error)
}

// RuntimeProvider abstracts container runtime operations (Docker/Podman).
type RuntimeProvider interface {
    ListRunning(ctx context.Context) ([]ContainerInfo, error)
    Start(ctx context.Context, opts StartOptions) error
    Stop(ctx context.Context, name string, timeout int) error
    Attach(ctx context.Context, name string, opts AttachOptions) error
    Exec(ctx context.Context, name string, cmd []string, opts ExecOptions) error
    Logs(ctx context.Context, name string, opts LogsOptions) (io.ReadCloser, error)
    Remove(ctx context.Context, name string, force bool) error
}

// UIProvider abstracts user interaction (prompts, output, TTY detection).
type UIProvider interface {
    IsInteractive() bool
    Prompt(message string, options []string) (string, error)
    Confirm(message string) (bool, error)
    Output() io.Writer
    Error() io.Writer
}
```

### Why This is Better

1. **Domain-Driven Design**
   - Service represents the devcontainer domain
   - Interfaces represent core concerns (config, runtime, UI)
   - Not tied to one specific operation

2. **Reusable Across All Commands**
   - `attach`, `exec`, `start`, `stop`, `logs`, etc. all use the same service
   - Common operations (name resolution, config loading) centralized
   - Single place to add cross-cutting concerns (logging, metrics)

3. **Clear Boundaries**
   - Config concerns separate from runtime concerns
   - UI concerns separate from business logic
   - Easy to understand "what depends on what"

4. **Extensible**
   - Want to add Podman support? Implement `RuntimeProvider`
   - Want to add remote config? Implement `ConfigProvider`
   - Want to add CLI/TUI/Web UI? Implement `UIProvider`

### Implementation

#### 1. Define Interfaces (`cmd/devcontainer/interfaces.go`)

```go
package devcontainer

import (
    "context"
    "io"

    "github.com/cloudposse/atmos/pkg/schema"
)

// ConfigProvider handles configuration loading and parsing.
type ConfigProvider interface {
    // LoadAtmosConfig loads the Atmos configuration.
    LoadAtmosConfig() (*schema.AtmosConfiguration, error)

    // ListDevcontainers returns all configured devcontainer names, sorted.
    ListDevcontainers(config *schema.AtmosConfiguration) ([]string, error)

    // GetDevcontainerConfig retrieves configuration for a specific devcontainer.
    GetDevcontainerConfig(config *schema.AtmosConfiguration, name string) (*DevcontainerConfig, error)
}

// RuntimeProvider abstracts container runtime operations.
type RuntimeProvider interface {
    // ListRunning returns all running devcontainer instances.
    ListRunning(ctx context.Context) ([]ContainerInfo, error)

    // Start starts a devcontainer.
    Start(ctx context.Context, name string, opts StartOptions) error

    // Stop stops a running devcontainer.
    Stop(ctx context.Context, name string, timeout int) error

    // Attach attaches to a running devcontainer.
    Attach(ctx context.Context, name string, opts AttachOptions) error

    // Exec executes a command in a running devcontainer.
    Exec(ctx context.Context, name string, cmd []string, opts ExecOptions) error

    // Logs retrieves logs from a devcontainer.
    Logs(ctx context.Context, name string, opts LogsOptions) (io.ReadCloser, error)

    // Remove removes a devcontainer.
    Remove(ctx context.Context, name string, force bool) error

    // Rebuild rebuilds a devcontainer.
    Rebuild(ctx context.Context, name string, opts RebuildOptions) error
}

// UIProvider handles user interaction.
type UIProvider interface {
    // IsInteractive returns true if running in an interactive terminal.
    IsInteractive() bool

    // Prompt displays a menu and returns the selected item.
    Prompt(message string, options []string) (string, error)

    // Confirm asks a yes/no question.
    Confirm(message string) (bool, error)

    // Output returns the writer for normal output (typically stderr for UI).
    Output() io.Writer

    // Error returns the writer for error output.
    Error() io.Writer
}

// ContainerInfo contains information about a running container.
type ContainerInfo struct {
    Name     string
    Image    string
    Status   string
    Instance string
}

// DevcontainerConfig represents parsed devcontainer configuration.
type DevcontainerConfig struct {
    Name      string
    Image     string
    BuildArgs map[string]string
    Mounts    []string
    // ... other config fields
}
```

#### 2. Create Service (`cmd/devcontainer/service.go`)

```go
package devcontainer

import (
    "context"
    "fmt"

    errUtils "github.com/cloudposse/atmos/errors"
    "github.com/cloudposse/atmos/pkg/schema"
)

// Service coordinates devcontainer operations.
type Service struct {
    config      ConfigProvider
    runtime     RuntimeProvider
    ui          UIProvider
    atmosConfig *schema.AtmosConfiguration
}

// NewService creates a service with default providers.
func NewService() *Service {
    return &Service{
        config:  &DefaultConfigProvider{},
        runtime: &DockerRuntimeProvider{},
        ui:      &DefaultUIProvider{},
    }
}

// NewTestableService creates a service with injectable providers (for tests).
func NewTestableService(
    config ConfigProvider,
    runtime RuntimeProvider,
    ui UIProvider,
) *Service {
    return &Service{
        config:  config,
        runtime: runtime,
        ui:      ui,
    }
}

// Initialize loads the Atmos configuration.
// Call this once during startup.
func (s *Service) Initialize() error {
    config, err := s.config.LoadAtmosConfig()
    if err != nil {
        return fmt.Errorf("failed to initialize devcontainer service: %w", err)
    }
    s.atmosConfig = config
    return nil
}

// ResolveDevcontainerName gets devcontainer name from args or prompts user.
// This is a common operation used by multiple commands.
func (s *Service) ResolveDevcontainerName(ctx context.Context, args []string) (string, error) {
    // If name provided in args, use it.
    if len(args) > 0 && args[0] != "" {
        return args[0], nil
    }

    // Check if interactive.
    if !s.ui.IsInteractive() {
        return "", fmt.Errorf("%w: devcontainer name required in non-interactive mode",
            errUtils.ErrDevcontainerNameEmpty)
    }

    // Get available devcontainers.
    devcontainers, err := s.config.ListDevcontainers(s.atmosConfig)
    if err != nil {
        return "", err
    }

    if len(devcontainers) == 0 {
        return "", fmt.Errorf("%w: no devcontainers configured",
            errUtils.ErrDevcontainerNotFound)
    }

    // Prompt user.
    selected, err := s.ui.Prompt("Select a devcontainer:", devcontainers)
    if err != nil {
        return "", err
    }

    fmt.Fprintf(s.ui.Output(), "\nSelected devcontainer: %s\n\n", selected)
    return selected, nil
}

// Attach attaches to a running devcontainer.
func (s *Service) Attach(ctx context.Context, name string, opts AttachOptions) error {
    return s.runtime.Attach(ctx, name, opts)
}

// Start starts a devcontainer.
func (s *Service) Start(ctx context.Context, name string, opts StartOptions) error {
    // Get devcontainer config.
    config, err := s.config.GetDevcontainerConfig(s.atmosConfig, name)
    if err != nil {
        return err
    }

    // Start via runtime.
    if err := s.runtime.Start(ctx, name, opts); err != nil {
        return err
    }

    // Optionally attach.
    if opts.Attach {
        return s.runtime.Attach(ctx, name, AttachOptions{
            Instance: opts.Instance,
        })
    }

    return nil
}

// Stop stops a running devcontainer.
func (s *Service) Stop(ctx context.Context, name string, timeout int) error {
    return s.runtime.Stop(ctx, name, timeout)
}

// List lists all running devcontainers.
func (s *Service) List(ctx context.Context) ([]ContainerInfo, error) {
    return s.runtime.ListRunning(ctx)
}

// ... other operations (Exec, Logs, Remove, Rebuild)
```

#### 3. Default Implementations (`cmd/devcontainer/providers.go`)

```go
package devcontainer

import (
    "context"
    "fmt"
    "io"
    "os"
    "sort"

    "github.com/charmbracelet/huh"
    "github.com/mattn/go-isatty"

    cfg "github.com/cloudposse/atmos/pkg/config"
    "github.com/cloudposse/atmos/pkg/devcontainer"
    errUtils "github.com/cloudposse/atmos/errors"
    "github.com/cloudposse/atmos/pkg/schema"
)

// DefaultConfigProvider uses pkg/config for configuration.
type DefaultConfigProvider struct{}

func (d *DefaultConfigProvider) LoadAtmosConfig() (*schema.AtmosConfiguration, error) {
    config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
    if err != nil {
        return nil, fmt.Errorf("failed to load atmos config: %w", err)
    }
    return &config, nil
}

func (d *DefaultConfigProvider) ListDevcontainers(config *schema.AtmosConfiguration) ([]string, error) {
    if config == nil || config.Devcontainer == nil {
        return nil, fmt.Errorf("%w: no devcontainers configured",
            errUtils.ErrDevcontainerNotFound)
    }

    var names []string
    for name := range config.Devcontainer {
        names = append(names, name)
    }
    sort.Strings(names)
    return names, nil
}

func (d *DefaultConfigProvider) GetDevcontainerConfig(
    config *schema.AtmosConfiguration,
    name string,
) (*DevcontainerConfig, error) {
    // Parse devcontainer config from atmos config.
    // This is where you'd parse the devcontainer.json equivalent from atmos.yaml.
    return &DevcontainerConfig{Name: name}, nil
}

// DockerRuntimeProvider uses pkg/devcontainer for runtime operations.
type DockerRuntimeProvider struct{}

func (d *DockerRuntimeProvider) ListRunning(ctx context.Context) ([]ContainerInfo, error) {
    // Delegate to pkg/devcontainer
    return devcontainer.List(ctx, nil)
}

func (d *DockerRuntimeProvider) Start(ctx context.Context, name string, opts StartOptions) error {
    return devcontainer.Start(ctx, nil, name, opts)
}

func (d *DockerRuntimeProvider) Attach(ctx context.Context, name string, opts AttachOptions) error {
    return devcontainer.Attach(ctx, nil, name, opts)
}

// ... implement other RuntimeProvider methods

// DefaultUIProvider uses terminal for UI operations.
type DefaultUIProvider struct{}

func (d *DefaultUIProvider) IsInteractive() bool {
    return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

func (d *DefaultUIProvider) Prompt(message string, options []string) (string, error) {
    if len(options) == 0 {
        return "", fmt.Errorf("%w: no options available",
            errUtils.ErrDevcontainerNotFound)
    }

    var selected string
    form := huh.NewForm(
        huh.NewGroup(
            huh.NewSelect[string]().
                Title(message).
                Options(huh.NewOptions(options...)...).
                Value(&selected),
        ),
    )

    if err := form.Run(); err != nil {
        return "", fmt.Errorf("prompt failed: %w", err)
    }

    return selected, nil
}

func (d *DefaultUIProvider) Confirm(message string) (bool, error) {
    var confirmed bool
    form := huh.NewForm(
        huh.NewGroup(
            huh.NewConfirm().
                Title(message).
                Value(&confirmed),
        ),
    )

    if err := form.Run(); err != nil {
        return false, err
    }

    return confirmed, nil
}

func (d *DefaultUIProvider) Output() io.Writer {
    return os.Stderr
}

func (d *DefaultUIProvider) Error() io.Writer {
    return os.Stderr
}
```

#### 4. Update Commands to Use Service

```go
package devcontainer

import (
    "github.com/spf13/cobra"
    // ...
)

var (
    service *Service  // Shared service instance
)

func init() {
    // Initialize service with default providers.
    service = NewService()

    // Load config once at startup.
    if err := service.Initialize(); err != nil {
        // Handle error (could use CheckErrorPrintAndExit or log)
    }
}

var attachCmd = &cobra.Command{
    Use:   "attach [devcontainer-name]",
    Short: "Attach to a running devcontainer",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Resolve name (common operation).
        name, err := service.ResolveDevcontainerName(cmd.Context(), args)
        if err != nil {
            return err
        }

        // Parse command-specific options.
        opts, err := parseAttachOptions(cmd, viper.GetViper(), args)
        if err != nil {
            return err
        }

        // Delegate to service.
        return service.Attach(cmd.Context(), name, AttachOptions{
            Instance: opts.Instance,
            UsePTY:   opts.UsePTY,
        })
    },
}

var startCmd = &cobra.Command{
    Use:   "start [devcontainer-name]",
    Short: "Start a devcontainer",
    RunE: func(cmd *cobra.Command, args []string) error {
        name, err := service.ResolveDevcontainerName(cmd.Context(), args)
        if err != nil {
            return err
        }

        opts, err := parseStartOptions(cmd, viper.GetViper(), args)
        if err != nil {
            return err
        }

        return service.Start(cmd.Context(), name, StartOptions{
            Instance: opts.Instance,
            Attach:   opts.Attach,
        })
    },
}

// All other commands follow same pattern
```

#### 5. Comprehensive Testing

```go
package devcontainer

import (
    "context"
    "errors"
    "io"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    errUtils "github.com/cloudposse/atmos/errors"
    "github.com/cloudposse/atmos/pkg/schema"
)

// Mock providers for testing.

type mockConfigProvider struct {
    atmosConfig   *schema.AtmosConfiguration
    loadError     error
    devcontainers []string
    listError     error
}

func (m *mockConfigProvider) LoadAtmosConfig() (*schema.AtmosConfiguration, error) {
    return m.atmosConfig, m.loadError
}

func (m *mockConfigProvider) ListDevcontainers(config *schema.AtmosConfiguration) ([]string, error) {
    return m.devcontainers, m.listError
}

func (m *mockConfigProvider) GetDevcontainerConfig(
    config *schema.AtmosConfiguration,
    name string,
) (*DevcontainerConfig, error) {
    return &DevcontainerConfig{Name: name}, nil
}

type mockRuntimeProvider struct {
    startError  error
    attachError error
    stopError   error
}

func (m *mockRuntimeProvider) Start(ctx context.Context, name string, opts StartOptions) error {
    return m.startError
}

func (m *mockRuntimeProvider) Attach(ctx context.Context, name string, opts AttachOptions) error {
    return m.attachError
}

func (m *mockRuntimeProvider) Stop(ctx context.Context, name string, timeout int) error {
    return m.stopError
}

// ... other methods

type mockUIProvider struct {
    interactive   bool
    promptResult  string
    promptError   error
    confirmResult bool
}

func (m *mockUIProvider) IsInteractive() bool {
    return m.interactive
}

func (m *mockUIProvider) Prompt(message string, options []string) (string, error) {
    return m.promptResult, m.promptError
}

func (m *mockUIProvider) Confirm(message string) (bool, error) {
    return m.confirmResult, nil
}

func (m *mockUIProvider) Output() io.Writer {
    return io.Discard
}

func (m *mockUIProvider) Error() io.Writer {
    return io.Discard
}

// Tests

func TestService_ResolveDevcontainerName(t *testing.T) {
    tests := []struct {
        name          string
        args          []string
        config        *mockConfigProvider
        ui            *mockUIProvider
        expectedName  string
        expectedError error
    }{
        {
            name: "name in args",
            args: []string{"geodesic"},
            config: &mockConfigProvider{
                atmosConfig: &schema.AtmosConfiguration{},
            },
            ui:           &mockUIProvider{interactive: true},
            expectedName: "geodesic",
        },
        {
            name: "non-interactive no args",
            args: []string{},
            config: &mockConfigProvider{
                atmosConfig: &schema.AtmosConfiguration{},
            },
            ui:            &mockUIProvider{interactive: false},
            expectedError: errUtils.ErrDevcontainerNameEmpty,
        },
        {
            name: "interactive prompt success",
            args: []string{},
            config: &mockConfigProvider{
                atmosConfig:   &schema.AtmosConfiguration{},
                devcontainers: []string{"geodesic", "terraform"},
            },
            ui: &mockUIProvider{
                interactive:  true,
                promptResult: "geodesic",
            },
            expectedName: "geodesic",
        },
        {
            name: "prompt fails",
            args: []string{},
            config: &mockConfigProvider{
                atmosConfig:   &schema.AtmosConfiguration{},
                devcontainers: []string{"geodesic"},
            },
            ui: &mockUIProvider{
                interactive: true,
                promptError: errors.New("user cancelled"),
            },
            expectedError: errors.New("user cancelled"),
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            service := NewTestableService(tt.config, nil, tt.ui)
            service.atmosConfig = tt.config.atmosConfig

            name, err := service.ResolveDevcontainerName(context.Background(), tt.args)

            if tt.expectedError != nil {
                require.Error(t, err)
                if errors.Is(tt.expectedError, err) {
                    assert.ErrorIs(t, err, tt.expectedError)
                } else {
                    assert.Contains(t, err.Error(), tt.expectedError.Error())
                }
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expectedName, name)
            }
        })
    }
}

func TestService_Start(t *testing.T) {
    tests := []struct {
        name          string
        devName       string
        opts          StartOptions
        runtime       *mockRuntimeProvider
        expectError   bool
    }{
        {
            name:    "start without attach",
            devName: "geodesic",
            opts:    StartOptions{Attach: false},
            runtime: &mockRuntimeProvider{},
        },
        {
            name:    "start with attach",
            devName: "geodesic",
            opts:    StartOptions{Attach: true},
            runtime: &mockRuntimeProvider{},
        },
        {
            name:    "start fails",
            devName: "geodesic",
            opts:    StartOptions{},
            runtime: &mockRuntimeProvider{
                startError: errors.New("docker error"),
            },
            expectError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            config := &mockConfigProvider{
                atmosConfig: &schema.AtmosConfiguration{},
            }
            service := NewTestableService(config, tt.runtime, nil)
            service.atmosConfig = config.atmosConfig

            err := service.Start(context.Background(), tt.devName, tt.opts)

            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

## Benefits of Service Architecture

### 1. Not One-Off
- **Serves entire devcontainer domain**, not just name resolution
- **Reusable across all commands** (attach, start, stop, exec, logs, etc.)
- **Cohesive**: All operations go through one service

### 2. Clear Separation of Concerns
- **ConfigProvider**: Everything about configuration
- **RuntimeProvider**: Everything about container operations
- **UIProvider**: Everything about user interaction
- **Service**: Coordinates between them

### 3. Easy to Test
```go
// Test any operation by mocking its dependencies
service := NewTestableService(
    mockConfig,    // Control config behavior
    mockRuntime,   // Control runtime behavior
    mockUI,        // Control UI behavior
)
```

### 4. Easy to Extend
- Want Podman? → Implement `RuntimeProvider`
- Want remote config? → Implement `ConfigProvider`
- Want web UI? → Implement `UIProvider`
- Want Kubernetes? → Implement new `RuntimeProvider`

### 5. Follows Existing Patterns
- Like `pkg/store/` with multiple store providers
- Like `pkg/auth/` with multiple auth providers
- Like `pkg/container/` with Docker/Podman abstraction

## Migration Path

1. **Create interfaces** (no breaking changes)
2. **Create default implementations** (wrap existing code)
3. **Create service** (new entry point)
4. **Update commands one by one** (gradual migration)
5. **Remove old helpers** (after all commands migrated)

## Comparison

| Approach | Scope | Reusability | Testability | Maintenance |
|----------|-------|-------------|-------------|-------------|
| NameResolver | One operation | Low | Medium | Many structs |
| **Service** | **Entire domain** | **High** | **High** | **One struct** |
