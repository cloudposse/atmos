package devcontainer

import (
	"context"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Stop stops a devcontainer.
func (m *Manager) Stop(atmosConfig *schema.AtmosConfiguration, name, instance string, timeout int) error {
	defer perf.Track(atmosConfig, "devcontainer.Stop")()

	ctx := context.Background()

	_, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
	if err != nil {
		return buildConfigLoadError(err, name)
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
		return buildContainerNotFoundError(name, containerName, instance)
	}

	container := containers[0]
	if !strings.Contains(strings.ToLower(container.Status), "running") {
		_ = ui.Infof("Container %s is already stopped", containerName)
		return nil
	}

	return stopContainerWithTimeout(&stopParams{
		ctx:           ctx,
		runtime:       runtime,
		containerInfo: &container,
		containerName: containerName,
		name:          name,
		timeout:       timeout,
	})
}

// buildContainerNotFoundError builds a standardized error when container is not found.
func buildContainerNotFoundError(name, containerName, instance string) error {
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

// stopParams holds parameters for stopping a container.
type stopParams struct {
	ctx           context.Context
	runtime       container.Runtime
	containerInfo *container.Info
	containerName string
	name          string
	timeout       int
}

// stopContainerWithTimeout stops a container with the specified timeout.
func stopContainerWithTimeout(params *stopParams) error {
	return runWithSpinner(
		fmt.Sprintf("Stopping container %s", params.containerName),
		fmt.Sprintf("Stopped container %s", params.containerName),
		func() error {
			stopTimeout := time.Duration(params.timeout) * time.Second
			if err := params.runtime.Stop(params.ctx, params.containerInfo.ID, stopTimeout); err != nil {
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to stop container `%s` (ID: %s)", params.containerName, params.containerInfo.ID).
					WithHintf("The container may be stuck or the timeout (%d seconds) may be too short", params.timeout).
					WithHint("Check that the container runtime daemon is running").
					WithHintf("Run `atmos devcontainer logs %s` to check container logs", params.name).
					WithHint("Try increasing the timeout with `--timeout` flag").
					WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
					WithContext("container_name", params.containerName).
					WithContext("container_id", params.containerInfo.ID).
					WithContext("timeout_seconds", fmt.Sprintf("%d", params.timeout)).
					WithExitCode(3).
					Err()
			}
			return nil
		})
}
