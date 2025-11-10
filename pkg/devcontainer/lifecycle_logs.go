package devcontainer

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Logs shows logs from a devcontainer.
func (m *Manager) Logs(atmosConfig *schema.AtmosConfiguration, name, instance string, follow bool, tail string) error {
	defer perf.Track(atmosConfig, "devcontainer.Logs")()

	_, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
	if err != nil {
		return err
	}

	// Get container runtime.
	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	// Generate container name.
	containerName, err := GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	// Get container info to verify it exists.
	ctx := context.Background()
	_, err = runtime.Inspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("%w: container %s not found", errUtils.ErrContainerNotFound, containerName)
	}

	// Show logs using default iolib.Data/UI channels.
	return runtime.Logs(ctx, containerName, follow, tail, nil, nil)
}
