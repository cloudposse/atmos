package devcontainer

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Start starts a devcontainer with optional identity.
func (m *Manager) Start(atmosConfig *schema.AtmosConfiguration, name, instance, identityName string) error {
	defer perf.Track(atmosConfig, "devcontainer.Start")()

	ctx := context.Background()

	config, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
	if err != nil {
		return buildConfigLoadError(err, name)
	}

	if identityName != "" {
		if err := m.identityManager.InjectIdentityEnvironment(ctx, config, identityName); err != nil {
			return buildIdentityInjectionError(err, name, identityName)
		}
	}

	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return buildRuntimeDetectError(err, name, settings.Runtime)
	}

	containerName, err := GenerateContainerName(name, instance)
	if err != nil {
		return buildContainerNameError(err, name, instance)
	}

	filters := map[string]string{"name": containerName}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return buildContainerListError(err, name, containerName, settings.Runtime)
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

	return startExistingContainer(ctx, runtime, &containers[0], containerName, config)
}

// startExistingContainer starts an existing container if it's not running.
func startExistingContainer(ctx context.Context, runtime container.Runtime, containerInfo *container.Info, containerName string, config *Config) error {
	if isContainerRunning(containerInfo.Status) {
		_ = ui.Infof("Container %s is already running", containerName)
		return nil
	}

	err := runWithSpinner(
		fmt.Sprintf("Starting container %s", containerName),
		fmt.Sprintf("Started container %s", containerName),
		func() error {
			if err := runtime.Start(ctx, containerInfo.ID); err != nil {
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to start existing container `%s` (ID: %s)", containerName, containerInfo.ID).
					WithHint("If the container has issues, use `--replace` flag to remove and recreate it").
					WithHint("Check that the container runtime daemon is running").
					WithHintf("Run `docker inspect %s` or `podman inspect %s` to see container details", containerName, containerName).
					WithHint("Try removing and recreating the container with `atmos devcontainer remove` and `atmos devcontainer start`").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
					WithContext("container_name", containerName).
					WithContext("container_id", containerInfo.ID).
					WithExitCode(3).
					Err()
			}
			return nil
		})
	if err != nil {
		return err
	}

	// Inspect container to get actual port information after starting.
	inspectedInfo, err := runtime.Inspect(ctx, containerInfo.ID)
	if err != nil {
		log.Warn("Failed to inspect container for port info", "error", err)
		displayContainerInfo(config, nil)
	} else {
		displayContainerInfo(config, inspectedInfo)
	}

	return nil
}
