package container

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// buildCreateArgs builds the common arguments for container creation.
// This function is shared between Docker and Podman runtimes to avoid duplication.
func buildCreateArgs(config *CreateConfig) []string {
	args := []string{"create", "--name", config.Name, "-it"}

	args = addRuntimeFlags(args, config)
	args = addMetadata(args, config)
	args = addResourceBindings(args, config)
	args = addImageAndCommand(args, config)

	return args
}

func addRuntimeFlags(args []string, config *CreateConfig) []string {
	if config.Init {
		args = append(args, "--init")
	}

	if config.Privileged {
		args = append(args, "--privileged")
	}

	for _, cap := range config.CapAdd {
		args = append(args, "--cap-add", cap)
	}

	for _, opt := range config.SecurityOpt {
		args = append(args, "--security-opt", opt)
	}

	return args
}

func addMetadata(args []string, config *CreateConfig) []string {
	for key, value := range config.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}

	for key, value := range config.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}

	return args
}

func addResourceBindings(args []string, config *CreateConfig) []string {
	for _, mount := range config.Mounts {
		mountStr := fmt.Sprintf("type=%s,source=%s,target=%s", mount.Type, mount.Source, mount.Target)
		if mount.ReadOnly {
			mountStr += ",readonly"
		}
		args = append(args, "--mount", mountStr)
	}

	for _, port := range config.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d/%s", port.HostPort, port.ContainerPort, port.Protocol))
	}

	if config.User != "" {
		args = append(args, "--user", config.User)
	}

	if config.WorkspaceFolder != "" {
		args = append(args, "-w", config.WorkspaceFolder)
	}

	return args
}

func addImageAndCommand(args []string, config *CreateConfig) []string {
	args = append(args, config.RunArgs...)

	if config.OverrideCommand {
		args = append(args, "--entrypoint", "/bin/sh")
	}

	args = append(args, config.Image)

	if config.OverrideCommand {
		args = append(args, "-c", "sleep infinity")
	}

	return args
}

// buildExecArgs builds the common arguments for container exec.
// This function is shared between Docker and Podman runtimes to avoid duplication.
func buildExecArgs(containerID string, cmd []string, opts *ExecOptions) []string {
	args := []string{"exec"}

	if opts != nil {
		args = addExecOptions(args, opts)
	}

	args = append(args, containerID)
	args = append(args, cmd...)

	return args
}

func addExecOptions(args []string, opts *ExecOptions) []string {
	if opts.User != "" {
		args = append(args, "--user", opts.User)
	}
	if opts.WorkingDir != "" {
		args = append(args, "-w", opts.WorkingDir)
	}
	if opts.Tty {
		args = append(args, "-t")
	}
	if opts.AttachStdin {
		args = append(args, "-i")
	}
	for _, env := range opts.Env {
		args = append(args, "-e", env)
	}
	return args
}

// execWithRuntime executes a command in a container using the specified runtime.
// This function is shared between Docker and Podman runtimes to avoid duplication.
func execWithRuntime(ctx context.Context, runtimeName string, containerID string, cmd []string, opts *ExecOptions) error {
	args := buildExecArgs(containerID, cmd, opts)

	execCmd := exec.CommandContext(ctx, runtimeName, args...)

	// For interactive mode (when Tty and AttachStdin are both true), connect to terminal.
	if opts != nil && opts.Tty && opts.AttachStdin {
		execCmd.Stdin = os.Stdin
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if err := execCmd.Run(); err != nil {
			// Don't treat exit code as error for interactive sessions.
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				log.Debug("Interactive session exited", "code", exitErr.ExitCode())
				return nil
			}
			return fmt.Errorf("%w: %s exec failed: %w", errUtils.ErrContainerRuntimeOperation, runtimeName, err)
		}
		return nil
	}

	// Non-interactive mode.
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s exec failed: %w: %s", errUtils.ErrContainerRuntimeOperation, runtimeName, err, string(output))
	}

	return nil
}
