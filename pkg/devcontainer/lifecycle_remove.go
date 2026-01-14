package devcontainer

import (
	"context"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
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
		return buildConfigLoadError(err, name)
	}

	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return buildRuntimeDetectError(err, name, settings.Runtime)
	}

	ctx := context.Background()
	containerName, err := GenerateContainerName(name, instance)
	if err != nil {
		return buildContainerNameError(err, name, instance)
	}

	containerInfo, err := runtime.Inspect(ctx, containerName)
	if err != nil {
		if errors.Is(err, errUtils.ErrContainerNotFound) {
			return nil
		}
		return buildContainerInspectError(err, name, containerName, settings.Runtime)
	}

	if err := checkAndStopForRemoval(&removalParams{
		ctx:           ctx,
		runtime:       runtime,
		containerInfo: containerInfo,
		name:          name,
		containerName: containerName,
		force:         force,
	}); err != nil {
		return err
	}

	return errUtils.Build(removeContainer(ctx, runtime, containerInfo, containerName)).
		WithContext("devcontainer_name", name).
		WithContext("container_name", containerName).
		Err()
}

// removalParams holds parameters for container removal operations.
type removalParams struct {
	ctx           context.Context
	runtime       container.Runtime
	containerInfo *container.Info
	name          string
	containerName string
	force         bool
}

// checkAndStopForRemoval validates container state and stops if needed for removal.
func checkAndStopForRemoval(params *removalParams) error {
	if !isContainerRunning(params.containerInfo.Status) {
		return nil
	}

	if !params.force {
		return errUtils.Build(errUtils.ErrContainerRunning).
			WithExplanationf("Container `%s` is currently running", params.containerName).
			WithHintf("Stop the container first with `atmos devcontainer stop %s`", params.name).
			WithHint("Or use `--force` flag to stop and remove in one step").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
			WithContext("devcontainer_name", params.name).
			WithContext("container_name", params.containerName).
			WithContext("container_status", params.containerInfo.Status).
			WithExitCode(1).
			Err()
	}

	if err := stopContainerIfRunning(params.ctx, params.runtime, params.containerInfo); err != nil {
		return errUtils.Build(err).
			WithContext("devcontainer_name", params.name).
			WithContext("container_name", params.containerName).
			Err()
	}
	return nil
}
