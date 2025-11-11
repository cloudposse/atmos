package devcontainer

import (
	"context"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Remove removes a devcontainer by name and instance.
// The operation is idempotent - returns nil if the container does not exist.
//
// Reloads configuration, detects the container runtime, and generates the container name.
// Fails if the container is running unless force is true. When force is true, stops a
// running container before removal. Returns relevant errors for runtime or config failures.
//
// Parameters:
//   - atmosConfig: Atmos configuration for performance tracking
//   - name: Devcontainer name from configuration
//   - instance: Instance identifier (e.g., "default", "prod")
//   - force: If true, stops running containers before removal; if false, fails on running containers
func (m *Manager) Remove(atmosConfig *schema.AtmosConfiguration, name, instance string, force bool) error {
	defer perf.Track(atmosConfig, "devcontainer.Remove")()

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

	// Initialize container runtime.
	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
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

	ctx := context.Background()

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
	containerInfo, err := runtime.Inspect(ctx, containerName)
	if err != nil {
		// Only treat "not found" as success; propagate other errors.
		if errors.Is(err, errUtils.ErrContainerNotFound) {
			return nil
		}
		return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
			WithExplanationf("Failed to inspect container `%s`", containerName).
			WithHint("Check that the container runtime daemon is running").
			WithHint("Run `docker ps -a` or `podman ps -a` to see all containers").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
			WithContext("devcontainer_name", name).
			WithContext("container_name", containerName).
			WithContext("runtime", settings.Runtime).
			WithExitCode(3).
			Err()
	}

	// Stop container if running and force=false.
	if isContainerRunning(containerInfo.Status) && !force {
		return errUtils.Build(errUtils.ErrContainerRunning).
			WithExplanationf("Container `%s` is currently running", containerName).
			WithHintf("Stop the container first with `atmos devcontainer stop %s`", name).
			WithHint("Or use `--force` flag to stop and remove in one step").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
			WithContext("devcontainer_name", name).
			WithContext("container_name", containerName).
			WithContext("container_status", containerInfo.Status).
			WithExitCode(1).
			Err()
	}

	// Stop if running.
	if isContainerRunning(containerInfo.Status) {
		if err := stopContainerIfRunning(ctx, runtime, containerInfo); err != nil {
			return errUtils.Build(err).
				WithContext("devcontainer_name", name).
				WithContext("container_name", containerName).
				Err()
		}
	}

	// Remove the container.
	return errUtils.Build(removeContainer(ctx, runtime, containerInfo, containerName)).
		WithContext("devcontainer_name", name).
		WithContext("container_name", containerName).
		Err()
}
