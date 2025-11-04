package container

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// buildBuildArgs builds the arguments for a container image build operation.
// This function is shared between Docker and Podman runtimes to avoid duplication.
// Extracted to allow testing the argument building logic without executing commands.
func buildBuildArgs(config *BuildConfig) []string {
	args := []string{"build"}

	// Add build args.
	for key, value := range config.Args {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
	}

	// Add tags.
	for _, tag := range config.Tags {
		args = append(args, "-t", tag)
	}

	// Add context and dockerfile.
	args = append(args, "-f", config.Dockerfile, config.Context)

	return args
}

// buildRemoveArgs builds the arguments for a container remove operation.
// This function is shared between Docker and Podman runtimes to avoid duplication.
// Extracted to allow testing the argument building logic without executing commands.
func buildRemoveArgs(containerID string, force bool) []string {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, containerID)
	return args
}

// buildStopArgs builds the arguments for a container stop operation.
// This function is shared between Docker and Podman runtimes to avoid duplication.
// Extracted to allow testing the argument building logic without executing commands.
func buildStopArgs(containerID string, timeoutSecs int) []string {
	return []string{"stop", "-t", fmt.Sprintf("%d", timeoutSecs), containerID}
}

// buildLogsArgs builds the arguments for a container logs operation.
// This function is shared between Docker and Podman runtimes to avoid duplication.
// Extracted to allow testing the argument building logic without executing commands.
func buildLogsArgs(containerID string, follow bool, tail string) []string {
	args := []string{"logs"}

	if follow {
		args = append(args, "--follow")
	}

	if tail != "" && tail != "all" {
		args = append(args, "--tail", tail)
	}

	args = append(args, containerID)
	return args
}

// buildAttachCommand builds the shell command and exec options for an attach operation.
// This function is shared between Docker and Podman runtimes to avoid duplication.
// Extracted to allow testing the command building logic without executing commands.
func buildAttachCommand(opts *AttachOptions) ([]string, *ExecOptions) {
	shell := "/bin/bash"
	var shellArgs []string

	if opts != nil {
		if opts.Shell != "" {
			shell = opts.Shell
		}
		if len(opts.ShellArgs) > 0 {
			shellArgs = opts.ShellArgs
		}
	}

	// Build command: shell + args.
	cmd := []string{shell}
	cmd = append(cmd, shellArgs...)

	execOpts := &ExecOptions{
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}

	if opts != nil && opts.User != "" {
		execOpts.User = opts.User
	}

	return cmd, execOpts
}

// execWithRuntime executes a command in a container using the specified runtime.
// This function is shared between Docker and Podman runtimes to avoid duplication.
//
//nolint:revive // argument-limit: io.Writer parameters required for IO/UI framework integration
func execWithRuntime(ctx context.Context, runtimeName string, containerID string, cmd []string, opts *ExecOptions, stdout, stderr io.Writer) error {
	args := buildExecArgs(containerID, cmd, opts)

	execCmd := exec.CommandContext(ctx, runtimeName, args...)

	// For interactive mode (when Tty and AttachStdin are both true), connect to terminal.
	if opts != nil && opts.Tty && opts.AttachStdin {
		execCmd.Stdin = os.Stdin
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if err := execCmd.Run(); err != nil {
			// Propagate exit code to caller so CLI can report failure.
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				log.Debug("Interactive session exited", "code", exitErr.ExitCode())
				return fmt.Errorf("%w: %s exec exited with code %d: %w", errUtils.ErrContainerRuntimeOperation, runtimeName, exitErr.ExitCode(), err)
			}
			return fmt.Errorf("%w: %s exec failed: %w", errUtils.ErrContainerRuntimeOperation, runtimeName, err)
		}
		return nil
	}

	// Non-interactive mode - use provided io.Writers.
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("%w: %s exec failed: %w", errUtils.ErrContainerRuntimeOperation, runtimeName, err)
	}

	return nil
}
