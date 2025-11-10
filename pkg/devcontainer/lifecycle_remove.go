package devcontainer

import (
	"context"
	"fmt"

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
		return err
	}

	// Initialize container runtime.
	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return fmt.Errorf("%w: failed to initialize container runtime: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	ctx := context.Background()

	// Generate container name.
	containerName, err := GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	// Check if container exists.
	containerInfo, err := runtime.Inspect(ctx, containerName)
	if err != nil {
		// Container doesn't exist - nothing to remove, consider this success.
		return nil
	}

	// Stop container if running and force=false.
	if isContainerRunning(containerInfo.Status) && !force {
		return fmt.Errorf("%w: %s, use --force to remove", errUtils.ErrContainerRunning, containerName)
	}

	// Stop if running.
	if isContainerRunning(containerInfo.Status) {
		if err := stopContainerIfRunning(ctx, runtime, containerInfo); err != nil {
			return err
		}
	}

	// Remove the container.
	return removeContainer(ctx, runtime, containerInfo, containerName)
}
