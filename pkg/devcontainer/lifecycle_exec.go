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

// ExecParams encapsulates parameters for ExecuteExec.
type ExecParams struct {
	Name        string
	Instance    string
	Interactive bool
	UsePTY      bool
	Command     []string
}

// Exec executes a command in a running devcontainer.
// TODO: Add --identity flag support. When implemented, ENV file paths from identity
// must be resolved relative to container paths (e.g., /localhost or bind mount location),
// not host paths, since the container runs in its own filesystem namespace.
func (m *Manager) Exec(atmosConfig *schema.AtmosConfiguration, params ExecParams) error {
	defer perf.Track(atmosConfig, "devcontainer.Exec")()

	config, settings, err := m.configLoader.LoadConfig(atmosConfig, params.Name)
	if err != nil {
		return err
	}

	runtime, err := m.runtimeDetector.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	containerName, err := GenerateContainerName(params.Name, params.Instance)
	if err != nil {
		return err
	}

	ctx := context.Background()
	containerInfo, err := findAndStartContainer(ctx, runtime, containerName, config)
	if err != nil {
		return err
	}

	return execInContainer(&execParams{
		ctx:         ctx,
		runtime:     runtime,
		containerID: containerInfo.ID,
		interactive: params.Interactive,
		usePTY:      params.UsePTY,
		command:     params.Command,
	})
}

// execParams holds parameters for executing commands in a container.
type execParams struct {
	ctx         context.Context
	runtime     container.Runtime
	containerID string
	interactive bool
	usePTY      bool
	command     []string
}

// execInContainer executes a command in a container using PTY or regular mode.
func execInContainer(params *execParams) error {
	maskingEnabled := viper.GetBool("mask")

	// PTY mode: Use experimental PTY with masking.
	if params.usePTY {
		if !pty.IsSupported() {
			return fmt.Errorf("%w: only macOS and Linux are supported", errUtils.ErrPTYNotSupported)
		}

		log.Debug("Using experimental PTY mode with masking support")
		return execInContainerWithPTY(params.ctx, params.runtime, params.containerID, params.command, maskingEnabled)
	}

	// Regular mode (existing behavior).
	if params.interactive && maskingEnabled {
		log.Debug("Interactive TTY mode enabled - output masking is not available due to TTY limitations")
	}

	execOpts := &container.ExecOptions{
		Tty:          params.interactive, // TTY mode for interactive sessions.
		AttachStdin:  params.interactive, // Attach stdin only in interactive mode.
		AttachStdout: true,
		AttachStderr: true,
		// IO streams are nil, will default to iolib.Data/UI in runtime.
	}

	return params.runtime.Exec(params.ctx, params.containerID, params.command, execOpts)
}

// execInContainerWithPTY executes a command using PTY mode with masking support.
// This is an experimental feature that provides TTY functionality while preserving
// output masking capabilities.
func execInContainerWithPTY(ctx context.Context, runtime container.Runtime, containerID string, command []string, maskingEnabled bool) error {
	// Get the IO context for masking.
	ioCtx := iolib.GetContext()

	// Determine the runtime binary (docker or podman).
	runtimeInfo, err := runtime.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get runtime info: %w", err)
	}

	runtimeBinary := runtimeInfo.Type

	// Build the runtime exec command.
	// Example: docker exec -it <containerID> <command...>
	args := []string{"exec", "-it", containerID}
	args = append(args, command...)

	cmd := exec.Command(runtimeBinary, args...)

	// Configure PTY options with masking.
	ptyOpts := &pty.Options{
		Masker:        ioCtx.Masker(),
		EnableMasking: maskingEnabled,
	}

	// Execute with PTY.
	return pty.ExecWithPTY(ctx, cmd, ptyOpts)
}
