package devcontainer

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	errListContainers = "%w: failed to list containers: %w"
)

// Start starts a devcontainer with optional identity.
func (m *Manager) Start(atmosConfig *schema.AtmosConfiguration, name, instance, identityName string) error {
	defer perf.Track(atmosConfig, "devcontainer.Start")()

	ctx := context.Background()

	config, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
	if err != nil {
		return err
	}

	// Inject identity environment variables if identity is specified.
	if identityName != "" {
		if err := m.identityManager.InjectIdentityEnvironment(ctx, config, identityName); err != nil {
			return err
		}
	}

	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	containerName, err := GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	filters := map[string]string{"name": containerName}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return fmt.Errorf(errListContainers, errUtils.ErrContainerRuntimeOperation, err)
	}

	if len(containers) == 0 {
		params := &containerParams{
			ctx:           ctx,
			runtime:       runtime,
			config:        config,
			containerName: containerName,
			name:          name,
			instance:      instance,
		}
		return createAndStartNewContainer(params)
	}

	return startExistingContainer(ctx, runtime, &containers[0], containerName)
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
