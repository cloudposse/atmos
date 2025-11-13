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
}
