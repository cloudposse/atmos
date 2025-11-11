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

// Start starts a devcontainer with optional identity.
func (m *Manager) Start(atmosConfig *schema.AtmosConfiguration, name, instance, identityName string) error {
	defer perf.Track(atmosConfig, "devcontainer.Start")()

	ctx := context.Background()

	config, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
	if err != nil {
		return errUtils.Build(err).
			WithExplanationf("Failed to load devcontainer configuration for `%s`", name).
			WithHintf("Verify that the devcontainer is defined in `atmos.yaml` under `components.devcontainer.%s`", name).
			WithHint("Run `atmos devcontainer list` to see all available devcontainers").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
			WithExample(`components:
  devcontainer:
    backend-api:
      spec:
        image: mcr.microsoft.com/devcontainers/go:1.24
        forwardPorts: [8080]
        workspaceFolder: /workspace`).
			WithContext("devcontainer_name", name).
			WithExitCode(2).
			Err()
	}

	// Inject identity environment variables if identity is specified.
	if identityName != "" {
		if err := m.identityManager.InjectIdentityEnvironment(ctx, config, identityName); err != nil {
			return errUtils.Build(err).
				WithExplanationf("Failed to inject identity `%s` into devcontainer environment", identityName).
				WithHintf("Verify that the identity `%s` is configured in `atmos.yaml`", identityName).
				WithHint("Run `atmos auth identity list` to see available identities").
				WithHint("See Atmos docs: https://atmos.tools/cli/commands/auth/auth-identity-configure/").
				WithContext("devcontainer_name", name).
				WithContext("identity_name", identityName).
				WithExitCode(2).
				Err()
		}
	}

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

	filters := map[string]string{"name": containerName}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
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
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithExplanationf("Failed to start existing container `%s` (ID: %s)", containerName, containerInfo.ID).
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
}
