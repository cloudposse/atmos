package exec

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// containerParams holds parameters for container operations.
type containerParams struct {
	ctx           context.Context
	runtime       container.Runtime
	config        *devcontainer.Config
	containerName string
	name          string
	instance      string
}

// runWithSpinner runs an operation with a spinner UI.
// ProgressMsg is shown while operation is running (e.g., "Starting container").
// CompletedMsg is shown when operation completes successfully (e.g., "Started container").
func runWithSpinner(progressMsg, completedMsg string, operation func() error) error {
	// Check if TTY is available.
	isTTY := term.IsTTYSupportForStdout()

	if !isTTY {
		// No TTY - just run the operation and show simple output on one line.
		_ = ui.Writef("%s... ", progressMsg)
		err := operation()
		if err != nil {
			_ = ui.Writeln("")
			return err
		}
		_ = ui.Writeln(theme.Styles.Checkmark.String())
		return nil
	}

	// TTY available - use spinner.
	model := newDevcontainerSpinner(progressMsg, completedMsg)

	// Use inline mode - output to stderr, no alternate screen.
	p := tea.NewProgram(
		model,
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	)

	// Run operation in background.
	go func() {
		err := operation()
		p.Send(devcontainerOpCompleteMsg{err: err})
	}()

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("spinner error: %w", err)
	}

	if m, ok := finalModel.(devcontainerSpinnerModel); ok && m.err != nil {
		return m.err
	}

	return nil
}

// createAndStartNewContainer creates and starts a new container.
func createAndStartNewContainer(params *containerParams) error {
	// Build image if build configuration is specified.
	if err := buildImageIfNeeded(params.ctx, params.runtime, params.config, params.name); err != nil {
		return err
	}

	containerID, err := createContainer(params)
	if err != nil {
		return err
	}

	if err := startContainer(params.ctx, params.runtime, containerID, params.containerName); err != nil {
		return err
	}

	// Display container information.
	displayContainerInfo(params.config, params.containerName)

	return nil
}

// startExistingContainer starts an existing container if it's not running.
func startExistingContainer(ctx context.Context, runtime container.Runtime, containerInfo *container.Info, containerName string) error {
	if isContainerRunning(containerInfo.Status) {
		_ = ui.Infof("Container %s is already running", containerName)
		return nil
	}

	return runWithSpinner(
		fmt.Sprintf("Starting container %s", containerName),
		fmt.Sprintf("Started container %s", containerName),
		func() error {
			if err := runtime.Start(ctx, containerInfo.ID); err != nil {
				return fmt.Errorf("%w: failed to start container: %w", errUtils.ErrContainerRuntimeOperation, err)
			}
			return nil
		})
}

// stopAndRemoveContainer stops and removes a container if it exists.
func stopAndRemoveContainer(ctx context.Context, runtime container.Runtime, containerName string) error {
	containerInfo, err := runtime.Inspect(ctx, containerName)
	if err != nil {
		// Container doesn't exist - nothing to stop/remove.
		return nil //nolint:nilerr // intentionally ignoring error when container doesn't exist
	}

	if err := stopContainerIfRunning(ctx, runtime, containerInfo); err != nil {
		return err
	}

	return removeContainer(ctx, runtime, containerInfo, containerName)
}

// stopContainerIfRunning stops a container if it's running.
func stopContainerIfRunning(ctx context.Context, runtime container.Runtime, containerInfo *container.Info) error {
	if !isContainerRunning(containerInfo.Status) {
		return nil
	}

	return runWithSpinner(
		fmt.Sprintf("Stopping container %s", containerInfo.Name),
		fmt.Sprintf("Stopped container %s", containerInfo.Name),
		func() error {
			if err := runtime.Stop(ctx, containerInfo.ID, 10*time.Second); err != nil {
				return fmt.Errorf("%w: failed to stop container: %w", errUtils.ErrContainerRuntimeOperation, err)
			}
			return nil
		})
}

// removeContainer removes a container.
func removeContainer(ctx context.Context, runtime container.Runtime, containerInfo *container.Info, containerName string) error {
	return runWithSpinner(
		fmt.Sprintf("Removing container %s", containerName),
		fmt.Sprintf("Removed container %s", containerName),
		func() error {
			if err := runtime.Remove(ctx, containerInfo.ID, true); err != nil {
				return fmt.Errorf("%w: failed to remove container: %w", errUtils.ErrContainerRuntimeOperation, err)
			}
			return nil
		})
}

// pullImageIfNeeded pulls an image unless noPull is true or image is empty.
func pullImageIfNeeded(ctx context.Context, runtime container.Runtime, image string, noPull bool) error {
	if noPull || image == "" {
		return nil
	}

	return runWithSpinner(
		fmt.Sprintf("Pulling image %s", image),
		fmt.Sprintf("Pulled image %s", image),
		func() error {
			if err := runtime.Pull(ctx, image); err != nil {
				return fmt.Errorf("%w: failed to pull image: %w", errUtils.ErrContainerRuntimeOperation, err)
			}
			return nil
		})
}

// createContainer creates a new container.
func createContainer(params *containerParams) (string, error) {
	var containerID string

	err := runWithSpinner(
		fmt.Sprintf("Creating container %s", params.containerName),
		fmt.Sprintf("Created container %s", params.containerName),
		func() error {
			createConfig := devcontainer.ToCreateConfig(params.config, params.containerName, params.name, params.instance)

			id, err := params.runtime.Create(params.ctx, createConfig)
			if err != nil {
				return fmt.Errorf("%w: failed to create container: %w", errUtils.ErrContainerRuntimeOperation, err)
			}
			containerID = id
			return nil
		})

	return containerID, err
}

// startContainer starts a container.
func startContainer(ctx context.Context, runtime container.Runtime, containerID, containerName string) error {
	return runWithSpinner(
		fmt.Sprintf("Starting container %s", containerName),
		fmt.Sprintf("Started container %s", containerName),
		func() error {
			if err := runtime.Start(ctx, containerID); err != nil {
				return fmt.Errorf("%w: failed to start container: %w", errUtils.ErrContainerRuntimeOperation, err)
			}
			return nil
		})
}

// isContainerRunning checks if a container status indicates it's running.
func isContainerRunning(status string) bool {
	// Container status values can be "running", "Running", "Up", etc.
	// We check for common running indicators.
	return status == "running" || status == "Running" || status == "Up"
}

// buildImageIfNeeded builds a container image if build configuration is specified.
func buildImageIfNeeded(ctx context.Context, runtime container.Runtime, config *devcontainer.Config, devcontainerName string) error {
	// If no build config, image must be specified (already validated).
	if config.Build == nil {
		return nil
	}

	// Generate image name based on devcontainer name.
	imageName := fmt.Sprintf("atmos-devcontainer-%s", devcontainerName)

	// Build the image.
	return runWithSpinner(
		fmt.Sprintf("Building image %s", imageName),
		fmt.Sprintf("Built image %s", imageName),
		func() error {
			buildConfig := &container.BuildConfig{
				Context:    config.Build.Context,
				Dockerfile: config.Build.Dockerfile,
				Tags:       []string{imageName},
				Args:       config.Build.Args,
			}

			if err := runtime.Build(ctx, buildConfig); err != nil {
				return fmt.Errorf("%w: failed to build image: %w", errUtils.ErrContainerRuntimeOperation, err)
			}

			// Update config to use the built image name.
			config.Image = imageName

			return nil
		})
}

// displayContainerInfo displays key information about the container in a user-friendly format.
func displayContainerInfo(config *devcontainer.Config, containerName string) {
	var info []string

	// Show image.
	if config.Image != "" {
		info = append(info, fmt.Sprintf("Image: %s", config.Image))
	}

	// Show workspace mount.
	if config.WorkspaceFolder != "" {
		cwd, _ := os.Getwd()
		if cwd != "" {
			info = append(info, fmt.Sprintf("Workspace: %s → %s", cwd, config.WorkspaceFolder))
		} else {
			info = append(info, fmt.Sprintf("Workspace folder: %s", config.WorkspaceFolder))
		}
	}

	// Show forwarded ports.
	if len(config.ForwardPorts) > 0 {
		var ports []string
		for _, port := range config.ForwardPorts {
			switch v := port.(type) {
			case int:
				ports = append(ports, fmt.Sprintf("%d", v))
			case float64:
				ports = append(ports, fmt.Sprintf("%d", int(v)))
			case string:
				ports = append(ports, v)
			}
		}
		if len(ports) > 0 {
			info = append(info, fmt.Sprintf("Ports: %s", strings.Join(ports, ", ")))
		}
	}

	// Display info if we have any.
	if len(info) > 0 {
		_ = ui.Infof("\n%s\n", strings.Join(info, "\n"))
	}
}
