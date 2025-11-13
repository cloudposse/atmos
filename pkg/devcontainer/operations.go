package devcontainer

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

const (
	// defaultContainerStopTimeout is the default timeout for stopping containers.
	defaultContainerStopTimeout = 10 * time.Second
)

// containerParams holds parameters for container operations.
type containerParams struct {
	ctx           context.Context
	runtime       container.Runtime
	config        *Config
	containerName string
	name          string
	instance      string
}

// runWithSpinner is a wrapper for spinner.ExecWithSpinner for backwards compatibility.
func runWithSpinner(progressMsg, completedMsg string, operation func() error) error {
	return spinner.ExecWithSpinner(progressMsg, completedMsg, operation)
}

// createAndStartNewContainer creates and starts a new container.
func createAndStartNewContainer(params *containerParams) error {
	// Build image if build configuration is specified.
	if err := buildImageIfNeeded(params.ctx, params.runtime, params.config, params.name); err != nil {
		return errUtils.Build(err).
			WithContext("devcontainer_name", params.name).
			WithContext("container_name", params.containerName).
			Err()
	}

	containerID, err := createContainer(params)
	if err != nil {
		return errUtils.Build(err).
			WithContext("devcontainer_name", params.name).
			WithContext("container_name", params.containerName).
			Err()
	}

	if err := startContainer(params.ctx, params.runtime, containerID, params.containerName); err != nil {
		return errUtils.Build(err).
			WithContext("devcontainer_name", params.name).
			WithContext("container_name", params.containerName).
			WithContext("container_id", containerID).
			Err()
	}

	// Display container information.
	displayContainerInfo(params.config)

	return nil
}

// stopAndRemoveContainer stops and removes a container if it exists.
func stopAndRemoveContainer(ctx context.Context, runtime container.Runtime, containerName string) error {
	containerInfo, err := runtime.Inspect(ctx, containerName)
	if err != nil {
		// Container doesn't exist - nothing to stop/remove.
		return nil //nolint:nilerr // intentionally ignoring error when container doesn't exist
	}

	if err := stopContainerIfRunning(ctx, runtime, containerInfo); err != nil {
		return errUtils.Build(err).
			WithContext("container_name", containerName).
			WithContext("container_id", containerInfo.ID).
			Err()
	}

	return errUtils.Build(removeContainer(ctx, runtime, containerInfo, containerName)).
		WithContext("container_name", containerName).
		WithContext("container_id", containerInfo.ID).
		Err()
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
			if err := runtime.Stop(ctx, containerInfo.ID, defaultContainerStopTimeout); err != nil {
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to stop container `%s` (ID: %s)", containerInfo.Name, containerInfo.ID).
					WithHint("Check that the container runtime daemon is running").
					WithHintf("Run `atmos devcontainer logs %s` to check container logs", extractDevcontainerName(containerInfo.Name)).
					WithHint("The container may be stuck or require a forced removal").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
					WithContext("container_name", containerInfo.Name).
					WithContext("container_id", containerInfo.ID).
					WithExitCode(3).
					Err()
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
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to remove container `%s` (ID: %s)", containerName, containerInfo.ID).
					WithHint("Check that the container runtime daemon is running").
					WithHintf("Run `atmos devcontainer logs %s` to check container logs", extractDevcontainerName(containerName)).
					WithHint("If the container is running, stop it first with `atmos devcontainer stop`").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
					WithContext("container_name", containerName).
					WithContext("container_id", containerInfo.ID).
					WithExitCode(3).
					Err()
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
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to pull container image `%s`", image).
					WithHintf("Verify that the image name `%s` is correct and accessible", image).
					WithHint("Check that you have network connectivity and proper registry credentials").
					WithHint("If using a private registry, ensure you are logged in with `docker login` or `podman login`").
					WithHint("See Docker Hub: https://hub.docker.com/").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
					WithContext("image", image).
					WithExitCode(3).
					Err()
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
			createConfig := ToCreateConfig(params.config, params.containerName, params.name, params.instance)

			id, err := params.runtime.Create(params.ctx, createConfig)
			if err != nil {
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to create container `%s`", params.containerName).
					WithHintf("Verify that the image `%s` exists or can be pulled", params.config.Image).
					WithHint("Check that the container runtime daemon is running").
					WithHint("Run `docker images` or `podman images` to see available images").
					WithHint("See DevContainer spec: https://containers.dev/implementors/json_reference/").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
					WithContext("container_name", params.containerName).
					WithContext("devcontainer_name", params.name).
					WithContext("image", params.config.Image).
					WithExitCode(3).
					Err()
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
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to start container `%s` (ID: %s)", containerName, containerID).
					WithHint("If the container already exists, use `--replace` flag to remove and recreate it").
					WithHint("Check that the container runtime daemon is running").
					WithHintf("Run `atmos devcontainer config %s` to see devcontainer configuration", extractDevcontainerName(containerName)).
					WithHint("The container may have configuration issues preventing startup").
					WithHintf("Check container logs with `atmos devcontainer logs %s`", extractDevcontainerName(containerName)).
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
					WithContext("container_name", containerName).
					WithContext("container_id", containerID).
					WithExitCode(3).
					Err()
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
func buildImageIfNeeded(ctx context.Context, runtime container.Runtime, config *Config, devcontainerName string) error {
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
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to build container image `%s` from Dockerfile", imageName).
					WithHintf("Verify that the Dockerfile exists at `%s`", config.Build.Dockerfile).
					WithHintf("Verify that the build context path `%s` is correct", config.Build.Context).
					WithHint("Check that the container runtime daemon is running").
					WithHint("Review the Dockerfile for syntax errors or invalid instructions").
					WithHint("See DevContainer build spec: https://containers.dev/implementors/json_reference/#build-properties").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
					WithExample(`components:
  devcontainer:
    my-dev:
      spec:
        build:
          context: .
          dockerfile: Dockerfile
          args:
            VARIANT: "1.24"`).
					WithContext("devcontainer_name", devcontainerName).
					WithContext("image_name", imageName).
					WithContext("dockerfile", config.Build.Dockerfile).
					WithContext("context", config.Build.Context).
					WithExitCode(3).
					Err()
			}

			// Update config to use the built image name.
			config.Image = imageName

			return nil
		})
}

// displayContainerInfo displays key information about the container in a user-friendly format.
func displayContainerInfo(config *Config) {
	var info []string

	// Show image.
	if config.Image != "" {
		info = append(info, fmt.Sprintf("Image: %s", config.Image))
	}

	// Show workspace mount.
	if workspaceInfo := formatWorkspaceInfo(config.WorkspaceFolder); workspaceInfo != "" {
		info = append(info, workspaceInfo)
	}

	// Show forwarded ports.
	if portsInfo := formatPortsInfo(config.ForwardPorts); portsInfo != "" {
		info = append(info, portsInfo)
	}

	// Display info if we have any.
	if len(info) > 0 {
		_ = ui.Infof("\n%s\n", strings.Join(info, "\n"))
	}
}

// formatWorkspaceInfo formats the workspace folder information.
func formatWorkspaceInfo(workspaceFolder string) string {
	if workspaceFolder == "" {
		return ""
	}
	cwd, _ := os.Getwd()
	if cwd != "" {
		return fmt.Sprintf("Workspace: %s → %s", cwd, workspaceFolder)
	}
	return fmt.Sprintf("Workspace folder: %s", workspaceFolder)
}

// formatPortsInfo formats the forwarded ports information.
func formatPortsInfo(forwardPorts []interface{}) string {
	if len(forwardPorts) == 0 {
		return ""
	}

	var ports []string
	for _, port := range forwardPorts {
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
		return fmt.Sprintf("Ports: %s", strings.Join(ports, ", "))
	}
	return ""
}

// extractDevcontainerName extracts the devcontainer name from a full container name.
// Container names follow the format: atmos-devcontainer.<name>.<instance>[-<suffix>].
// For example: "atmos-devcontainer.geodesic.default-2" returns "geodesic".
func extractDevcontainerName(containerName string) string {
	// Remove "atmos-devcontainer." prefix.
	const prefix = "atmos-devcontainer."
	if !strings.HasPrefix(containerName, prefix) {
		return containerName
	}

	remainder := strings.TrimPrefix(containerName, prefix)

	// Split by "." to get name part.
	parts := strings.Split(remainder, ".")
	if len(parts) > 0 {
		return parts[0]
	}

	return containerName
}
