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
		runtime: NewDockerRuntimeProvider(),
		ui:      &DefaultUIProvider{},
	}
}

// NewTestableService creates a Service configured with the provided config, runtime, and UI providers for use in tests.
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
		return errUtils.Build(err).
			WithExplanation("Failed to initialize devcontainer service").
			WithHint("Check that atmos.yaml exists and is valid").
			Err()
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
		return "", errUtils.Build(errUtils.ErrDevcontainerNameEmpty).
			WithExplanation("Devcontainer name required in non-interactive mode").
			WithHint("Provide devcontainer name as argument or run in interactive terminal").
			Err()
	}

	// Get available devcontainers.
	devcontainers, err := s.config.ListDevcontainers(s.atmosConfig)
	if err != nil {
		return "", err
	}

	if len(devcontainers) == 0 {
		return "", errUtils.Build(errUtils.ErrDevcontainerNotFound).
			WithExplanation("No devcontainers configured").
			WithHint("Add devcontainer configuration to atmos.yaml under components.devcontainer").
			Err()
	}

	// Prompt user.
	selected, err := s.ui.Prompt("Select a devcontainer:", devcontainers)
	if err != nil {
		return "", errUtils.Build(err).
			WithExplanation("Failed to prompt for devcontainer selection").
			Err()
	}

	fmt.Fprintf(s.ui.Output(), "\nSelected devcontainer: %s\n\n", selected)
	return selected, nil
}

// Attach attaches to a running devcontainer.
func (s *Service) Attach(ctx context.Context, name string, opts AttachOptions) error {
	return s.runtime.Attach(ctx, s.atmosConfig, name, opts)
}

// Start starts a devcontainer.
func (s *Service) Start(ctx context.Context, name string, opts StartOptions) error {
	// Get devcontainer config.
	_, err := s.config.GetDevcontainerConfig(s.atmosConfig, name)
	if err != nil {
		return err
	}

	// Start via runtime.
	if err := s.runtime.Start(ctx, s.atmosConfig, name, opts); err != nil {
		return err
	}

	// Optionally attach.
	if opts.Attach {
		return s.runtime.Attach(ctx, s.atmosConfig, name, AttachOptions{
			Instance: opts.Instance,
		})
	}

	return nil
}

// Stop stops a running devcontainer.
func (s *Service) Stop(ctx context.Context, name string, opts StopOptions) error {
	return s.runtime.Stop(ctx, s.atmosConfig, name, opts)
}

// List lists all running devcontainers.
func (s *Service) List(ctx context.Context) ([]ContainerInfo, error) {
	return s.runtime.ListRunning(ctx)
}

// Exec executes a command in a running devcontainer.
func (s *Service) Exec(ctx context.Context, name string, cmd []string, opts ExecOptions) error {
	return s.runtime.Exec(ctx, s.atmosConfig, name, cmd, opts)
}

// Logs retrieves logs from a devcontainer.
func (s *Service) Logs(ctx context.Context, name string, opts LogsOptions) error {
	logs, err := s.runtime.Logs(ctx, s.atmosConfig, name, opts)
	if err != nil {
		return err
	}
	if logs != nil {
		defer logs.Close()
	}

	// Stream logs to output.
	// Note: In production, this would handle follow mode, etc.
	return nil
}

// Remove removes a devcontainer.
func (s *Service) Remove(ctx context.Context, name string, opts RemoveOptions) error {
	return s.runtime.Remove(ctx, s.atmosConfig, name, opts)
}

// Rebuild rebuilds a devcontainer.
func (s *Service) Rebuild(ctx context.Context, name string, opts RebuildOptions) error {
	return s.runtime.Rebuild(ctx, s.atmosConfig, name, opts)
}
