package devcontainer

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal/pty"
)

// Attach attaches to a running devcontainer.
// TODO: Add --identity flag support. When implemented, ENV file paths from identity
// must be resolved relative to container paths (e.g., /localhost or bind mount location),
// not host paths, since the container runs in its own filesystem namespace.
func (m *Manager) Attach(atmosConfig *schema.AtmosConfiguration, name, instance string, usePTY bool) error {
	defer perf.Track(atmosConfig, "devcontainer.Attach")()

	config, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
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

	ctx := context.Background()
	containerInfo, err := findAndStartContainer(ctx, runtime, containerName)
	if err != nil {
		return errUtils.Build(err).
			WithContext("devcontainer_name", name).
			WithContext("instance", instance).
			Err()
	}

	return attachToContainer(&attachParams{
		ctx:           ctx,
		runtime:       runtime,
		containerInfo: containerInfo,
		config:        config,
		containerName: containerName,
		usePTY:        usePTY,
	})
}

// findAndStartContainer finds a container and starts it if needed.
func findAndStartContainer(ctx context.Context, runtime container.Runtime, containerName string) (*container.Info, error) {
	filters := map[string]string{"name": containerName}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrContainerRuntimeOperation).
			WithCause(err).
			WithExplanationf("Failed to list containers with name `%s`", containerName).
			WithHint("Verify that the container runtime is accessible and running").
			WithHint("Run `docker ps -a` or `podman ps -a` to check container status").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
			WithContext("container_name", containerName).
			WithExitCode(3).
			Err()
	}

	if len(containers) == 0 {
		return nil, errUtils.Build(errUtils.ErrDevcontainerNotFound).
			WithExplanationf("Container `%s` does not exist", containerName).
			WithHintf("The container must be created before attaching to it").
			WithHint("Run `atmos devcontainer list` to see available containers").
			WithHint("Use `atmos devcontainer start` to create and start the container").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
			WithContext("container_name", containerName).
			WithExitCode(1).
			Err()
	}

	containerInfo := &containers[0]

	if !isContainerRunning(containerInfo.Status) {
		if err := startContainerForAttach(ctx, runtime, containerInfo, containerName); err != nil {
			return nil, err
		}
	}

	return containerInfo, nil
}

// startContainerForAttach starts a container before attaching.
func startContainerForAttach(ctx context.Context, runtime container.Runtime, containerInfo *container.Info, containerName string) error {
	return runWithSpinner(
		fmt.Sprintf("Starting container %s", containerName),
		fmt.Sprintf("Started container %s", containerName),
		func() error {
			if err := runtime.Start(ctx, containerInfo.ID); err != nil {
				return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
					WithCause(err).
					WithExplanationf("Failed to start container `%s` (ID: %s) before attaching", containerName, containerInfo.ID).
					WithHint("If the container has issues, use `atmos devcontainer start --replace` to recreate it").
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

// attachParams holds parameters for attaching to a container.
type attachParams struct {
	ctx           context.Context
	runtime       container.Runtime
	containerInfo *container.Info
	config        *Config
	containerName string
	usePTY        bool
}

// attachToContainer attaches to a container using PTY or regular mode.
func attachToContainer(params *attachParams) error {
	log.Debug("Attaching to container", "container", params.containerName)

	maskingEnabled := viper.GetBool("mask")

	// PTY mode: Use experimental PTY with masking.
	if params.usePTY {
		if !pty.IsSupported() {
			return errUtils.Build(errUtils.ErrPTYNotSupported).
				WithExplanation("PTY mode is only supported on macOS and Linux").
				WithHint("Remove the `--pty` flag to use standard attach mode").
				WithHint("Standard attach mode works on all platforms but has limited masking in TTY sessions").
				WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
				WithContext("devcontainer_name", params.containerName).
				WithExitCode(1).
				Err()
		}

		log.Debug("Using experimental PTY mode with masking support")
		shellArgs := getShellArgs(params.config.UserEnvProbe)
		return attachToContainerWithPTY(params.ctx, params.runtime, params.containerInfo.ID, shellArgs, maskingEnabled)
	}

	// Regular mode: Warn about masking limitations in interactive TTY sessions.
	if maskingEnabled {
		log.Debug("Interactive TTY session - output masking is not available due to TTY limitations")
	}

	shellArgs := getShellArgs(params.config.UserEnvProbe)
	attachOpts := &container.AttachOptions{ShellArgs: shellArgs}

	// IO streams are nil in opts, will default to iolib.Data/UI in runtime.
	return params.runtime.Attach(params.ctx, params.containerInfo.ID, attachOpts)
}

// attachToContainerWithPTY attaches to a container using PTY mode with masking support.
// This is an experimental feature that provides TTY functionality while preserving
// output masking capabilities.
func attachToContainerWithPTY(ctx context.Context, runtime container.Runtime, containerID string, shellArgs []string, maskingEnabled bool) error {
	// Get the IO context for masking.
	ioCtx := iolib.GetContext()

	// Determine the runtime binary (docker or podman).
	runtimeInfo, err := runtime.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get runtime info: %w", err)
	}

	runtimeBinary := runtimeInfo.Type

	// Build the runtime attach command with shell.
	// Example: docker exec -it <containerID> /bin/bash -l
	args := []string{"exec", "-it", containerID, "/bin/bash"}
	args = append(args, shellArgs...)

	cmd := exec.Command(runtimeBinary, args...)

	// Configure PTY options with masking.
	ptyOpts := &pty.Options{
		Masker:        ioCtx.Masker(),
		EnableMasking: maskingEnabled,
	}

	// Execute with PTY.
	return pty.ExecWithPTY(ctx, cmd, ptyOpts)
}

// getShellArgs returns shell arguments based on userEnvProbe setting.
func getShellArgs(userEnvProbe string) []string {
	if userEnvProbe == "loginShell" || userEnvProbe == "loginInteractiveShell" {
		return []string{"-l"}
	}
	return nil
}
