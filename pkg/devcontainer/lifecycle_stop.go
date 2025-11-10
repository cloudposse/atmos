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
		return err
	}

	// Detect runtime.
	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	// Generate container name.
	containerName, err := GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	// Check if container exists.
	filters := map[string]string{
		"name": containerName,
	}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return fmt.Errorf(errListContainers, errUtils.ErrContainerRuntimeOperation, err)
	}

	if len(containers) == 0 {
		return fmt.Errorf("%w: container %s not found", errUtils.ErrDevcontainerNotFound, containerName)
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
				return fmt.Errorf("%w: failed to stop container: %w", errUtils.ErrContainerRuntimeOperation, err)
			}
			return nil
		})
}
