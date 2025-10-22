package exec

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/devcontainer"
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
func runWithSpinner(message string, operation func() error) error {
	// Check if TTY is available.
	isTTY := term.IsTTYSupportForStdout()

	if !isTTY {
		// No TTY - just run the operation and show simple output on one line.
		fmt.Fprintf(os.Stderr, "%s... ", message)
		err := operation()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n")
			return err
		}
		fmt.Fprintf(os.Stderr, "%s\n", theme.Styles.Checkmark.String())
		return nil
	}

	// TTY available - use spinner.
	model := newDevcontainerSpinner(message)

	// Use inline mode - output to stderr, no alternate screen.
	p := tea.NewProgram(
		model,
		tea.WithOutput(os.Stderr),
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
	containerID, err := createContainer(params)
	if err != nil {
		return err
	}

	return startContainer(params.ctx, params.runtime, containerID, params.containerName)
}

// startExistingContainer starts an existing container if it's not running.
func startExistingContainer(ctx context.Context, runtime container.Runtime, containerInfo *container.Info, containerName string) error {
	if isContainerRunning(containerInfo.Status) {
		fmt.Fprintf(os.Stderr, "Container %s is already running\n", containerName)
		return nil
	}

	return runWithSpinner(fmt.Sprintf("Starting container %s", containerName), func() error {
		if err := runtime.Start(ctx, containerInfo.ID); err != nil {
			return fmt.Errorf("%w: failed to start container: %v", errUtils.ErrContainerRuntimeOperation, err)
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

	return runWithSpinner(fmt.Sprintf("Stopping container %s", containerInfo.Name), func() error {
		if err := runtime.Stop(ctx, containerInfo.ID, 10); err != nil {
			return fmt.Errorf("%w: failed to stop container: %v", errUtils.ErrContainerRuntimeOperation, err)
		}
		return nil
	})
}

// removeContainer removes a container.
func removeContainer(ctx context.Context, runtime container.Runtime, containerInfo *container.Info, containerName string) error {
	return runWithSpinner(fmt.Sprintf("Removing container %s", containerName), func() error {
		if err := runtime.Remove(ctx, containerInfo.ID, true); err != nil {
			return fmt.Errorf("%w: failed to remove container: %v", errUtils.ErrContainerRuntimeOperation, err)
		}
		return nil
	})
}

// pullImageIfNeeded pulls an image unless noPull is true or image is empty.
func pullImageIfNeeded(ctx context.Context, runtime container.Runtime, image string, noPull bool) error {
	if noPull || image == "" {
		return nil
	}

	return runWithSpinner(fmt.Sprintf("Pulling image %s", image), func() error {
		if err := runtime.Pull(ctx, image); err != nil {
			return fmt.Errorf("%w: failed to pull image: %v", errUtils.ErrContainerRuntimeOperation, err)
		}
		return nil
	})
}

// createContainer creates a new container.
func createContainer(params *containerParams) (string, error) {
	var containerID string

	err := runWithSpinner(fmt.Sprintf("Creating container %s", params.containerName), func() error {
		createConfig := devcontainer.ToCreateConfig(params.config, params.containerName, params.name, params.instance)

		id, err := params.runtime.Create(params.ctx, createConfig)
		if err != nil {
			return fmt.Errorf("%w: failed to create container: %v", errUtils.ErrContainerRuntimeOperation, err)
		}
		containerID = id
		return nil
	})

	return containerID, err
}

// startContainer starts a container.
func startContainer(ctx context.Context, runtime container.Runtime, containerID, containerName string) error {
	return runWithSpinner(fmt.Sprintf("Starting container %s", containerName), func() error {
		if err := runtime.Start(ctx, containerID); err != nil {
			return fmt.Errorf("%w: failed to start container: %v", errUtils.ErrContainerRuntimeOperation, err)
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
