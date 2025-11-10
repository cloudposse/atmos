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
		return err
	}

	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	containerName, err := GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	ctx := context.Background()
	containerInfo, err := findAndStartContainer(ctx, runtime, containerName)
	if err != nil {
		return err
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
		return nil, fmt.Errorf(errListContainers, errUtils.ErrContainerRuntimeOperation, err)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("%w: container %s not found", errUtils.ErrDevcontainerNotFound, containerName)
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
				return fmt.Errorf("%w: failed to start container: %w", errUtils.ErrContainerRuntimeOperation, err)
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
			return fmt.Errorf("%w: only macOS and Linux are supported", errUtils.ErrPTYNotSupported)
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
