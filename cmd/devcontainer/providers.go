package devcontainer

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DefaultConfigProvider uses pkg/config for configuration.
type DefaultConfigProvider struct{}

// LoadAtmosConfig loads the Atmos configuration.
func (d *DefaultConfigProvider) LoadAtmosConfig() (*schema.AtmosConfiguration, error) {
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, fmt.Errorf("failed to load atmos config: %w", err)
	}
	return &config, nil
}

// ListDevcontainers returns all configured devcontainer names, sorted.
func (d *DefaultConfigProvider) ListDevcontainers(config *schema.AtmosConfiguration) ([]string, error) {
	if config == nil || config.Devcontainer == nil {
		return nil, fmt.Errorf("%w: no devcontainers configured", errUtils.ErrDevcontainerNotFound)
	}

	var names []string
	for name := range config.Devcontainer {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// GetDevcontainerConfig retrieves configuration for a specific devcontainer.
func (d *DefaultConfigProvider) GetDevcontainerConfig(
	config *schema.AtmosConfiguration,
	name string,
) (*DevcontainerConfig, error) {
	// Parse devcontainer config from atmos config.
	// For now, return a minimal config. This can be expanded later.
	return &DevcontainerConfig{Name: name}, nil
}

// DockerRuntimeProvider uses pkg/devcontainer for runtime operations.
type DockerRuntimeProvider struct {
	manager *devcontainer.Manager
}

// NewDockerRuntimeProvider creates a new Docker runtime provider.
func NewDockerRuntimeProvider() *DockerRuntimeProvider {
	return &DockerRuntimeProvider{
		manager: devcontainer.NewManager(),
	}
}

// ListRunning returns all running devcontainer instances.
func (d *DockerRuntimeProvider) ListRunning(ctx context.Context) ([]ContainerInfo, error) {
	// Delegate to pkg/devcontainer List method.
	// List writes to UI, doesn't return containers.
	// For now, return empty list. This would need refactoring in pkg/devcontainer.
	return []ContainerInfo{}, nil
}

// Start starts a devcontainer.
func (d *DockerRuntimeProvider) Start(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts StartOptions) error {
	return d.manager.Start(atmosConfig, name, opts.Instance, opts.Identity)
}

// Stop stops a running devcontainer.
func (d *DockerRuntimeProvider) Stop(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts StopOptions) error {
	return d.manager.Stop(atmosConfig, name, opts.Instance, opts.Timeout)
}

// Attach attaches to a running devcontainer.
func (d *DockerRuntimeProvider) Attach(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts AttachOptions) error {
	return d.manager.Attach(atmosConfig, name, opts.Instance, opts.UsePTY)
}

// Exec executes a command in a running devcontainer.
func (d *DockerRuntimeProvider) Exec(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, cmd []string, opts ExecOptions) error {
	return d.manager.Exec(atmosConfig, devcontainer.ExecParams{
		Name:        name,
		Instance:    opts.Instance,
		Interactive: opts.Interactive,
		UsePTY:      opts.UsePTY,
		Command:     cmd,
	})
}

// Logs retrieves logs from a devcontainer.
func (d *DockerRuntimeProvider) Logs(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts LogsOptions) (io.ReadCloser, error) {
	// pkg/devcontainer.Logs writes directly to UI, doesn't return io.ReadCloser.
	// For now, return nil. This would need refactoring in pkg/devcontainer.
	err := d.manager.Logs(atmosConfig, name, opts.Instance, opts.Follow, opts.Tail)
	return nil, err
}

// Remove removes a devcontainer.
func (d *DockerRuntimeProvider) Remove(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts RemoveOptions) error {
	return d.manager.Remove(atmosConfig, name, opts.Instance, opts.Force)
}

// Rebuild rebuilds a devcontainer.
func (d *DockerRuntimeProvider) Rebuild(ctx context.Context, atmosConfig *schema.AtmosConfiguration, name string, opts RebuildOptions) error {
	return d.manager.Rebuild(atmosConfig, name, opts.Instance, opts.Identity, opts.NoPull)
}

// DefaultUIProvider uses terminal for UI operations.
type DefaultUIProvider struct{}

// IsInteractive returns true if running in an interactive terminal.
func (d *DefaultUIProvider) IsInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// Prompt displays a menu and returns the selected item.
func (d *DefaultUIProvider) Prompt(message string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("%w: no options available", errUtils.ErrDevcontainerNotFound)
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

// Confirm asks a yes/no question.
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

// Output returns the writer for normal output (typically stderr for UI).
func (d *DefaultUIProvider) Output() io.Writer {
	return os.Stderr
}

// Error returns the writer for error output.
func (d *DefaultUIProvider) Error() io.Writer {
	return os.Stderr
}
