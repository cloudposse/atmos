package devcontainer

import (
	"context"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Stop stops a devcontainer.
func (m *Manager) Stop(atmosConfig *schema.AtmosConfiguration, name, instance string, timeout int) error {
	defer perf.Track(atmosConfig, "devcontainer.Stop")()

	ctx := context.Background()

	// Load settings to get runtime.
	_, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
	if err != nil {
		return errUtils.Build(err).
			WithExplanationf("Failed to load devcontainer configuration for `%s`", name).
			WithHintf("Verify that the devcontainer is defined in `atmos.yaml` under `components.devcontainer.%s`", name).
			WithHint("Run `atmos devcontainer list` to see all available devcontainers").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
			WithContext("devcontainer_name", name).
			WithExitCode(2).
			Err()
	}

	// Detect runtime.
	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return errUtils.Build(err).
			WithExplanation("Failed to detect or initialize container runtime").
			WithHint("Ensure Docker or Podman is installed and running").
			WithHint("Run `docker info` or `podman info` to verify the runtime is accessible").
			WithHint("See Docker installation: https://docs.docker.com/get-docker/").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
			WithContext("devcontainer_name", name).
			WithContext("runtime", settings.Runtime).
			WithExitCode(3).
			Err()
	}

	// Generate container name.
	containerName, err := GenerateContainerName(name, instance)
	if err != nil {
		return errUtils.Build(err).
			WithExplanationf("Failed to generate valid container name from devcontainer `%s` and instance `%s`", name, instance).
			WithHint("Container names must be lowercase alphanumeric with hyphens only").
			WithHint("Ensure the devcontainer name and instance follow naming conventions").
			WithHint("See Docker naming: https://docs.docker.com/engine/reference/commandline/create/#name").
			WithContext("devcontainer_name", name).
			WithContext("instance", instance).
			WithExitCode(2).
			Err()
	}

	// Check if container exists.
	filters := map[string]string{
		"name": containerName,
	}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
			WithCause(err).
			WithExplanationf("Failed to list containers with name `%s`", containerName).
			WithHint("Verify that the container runtime is accessible and running").
			WithHint("Run `docker ps -a` or `podman ps -a` to check container status").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
			WithContext("devcontainer_name", name).
			WithContext("container_name", containerName).
			WithContext("runtime", settings.Runtime).
			WithExitCode(3).
			Err()
	}

	if len(containers) == 0 {
		return errUtils.Build(errUtils.ErrDevcontainerNotFound).
			WithExplanationf("Container `%s` does not exist", containerName).
			WithHint("The container may have already been removed or never created").
			WithHint("Run `atmos devcontainer list` to see running containers").
			WithHintf("Use `atmos devcontainer start %s` to create and start the container", name).
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
			WithContext("devcontainer_name", name).
			WithContext("container_name", containerName).
			WithContext("instance", instance).
			WithExitCode(1).
			Err()
	}

	container := containers[0]

	// Check if already stopped.
	if !strings.Contains(strings.ToLower(container.Status), "running") {
		_ = ui.Infof("Container %s is already stopped", containerName)
		return nil
	}

	// Stop the container with spinner.
	return runWithSpinner(
		fmt.Sprintf("Stopping container %s", containerName),
		fmt.Sprintf("Stopped container %s", containerName),
		func() error {
			stopTimeout := time.Duration(timeout) * time.Second
			if err := runtime.Stop(ctx, container.ID, stopTimeout); err != nil {
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to stop container `%s` (ID: %s)", containerName, container.ID).
					WithHintf("The container may be stuck or the timeout (%d seconds) may be too short", timeout).
					WithHint("Check that the container runtime daemon is running").
					WithHintf("Run `docker inspect %s` or `podman inspect %s` to see container state", containerName, containerName).
					WithHint("Try increasing the timeout with `--timeout` flag").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
					WithContext("container_name", containerName).
					WithContext("container_id", container.ID).
					WithContext("timeout_seconds", fmt.Sprintf("%d", timeout)).
					WithExitCode(3).
					Err()
			}
			return nil
		})
}
