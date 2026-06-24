package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// PullMissing pulls only after initial container creation fails.
	PullMissing = "missing"
	// PullAlways pulls the image before creating the container.
	PullAlways = "always"
	// PullNever never pulls the image.
	PullNever = "never"

	// CleanupAlways removes the container after execution, even on failure.
	CleanupAlways = "always"
	// CleanupOnSuccess removes the container only after successful execution.
	CleanupOnSuccess = "on_success"
	// CleanupNever leaves the container behind.
	CleanupNever = "never"

	// KeyValueFormat formats a key/value pair as `key=value`.
	keyValueFormat = "%s=%s"
	// RuntimePlaceholder is shown in command previews when no runtime is selected.
	runtimePlaceholder = "docker|podman"
	// SpaceSeparator joins command arguments in human-readable previews.
	spaceSeparator = " "
)

// EphemeralConfig describes a one-shot container execution.
type EphemeralConfig struct {
	Name              string
	Image             string
	Command           []string
	WorkspaceHostPath string
	WorkspaceFolder   string
	WorkspaceReadOnly bool
	Mounts            []Mount
	Ports             []PortBinding
	Env               []string
	User              string
	Labels            map[string]string
	RunArgs           []string
	PullPolicy        string
	CleanupPolicy     string
	TTY               bool
	Interactive       bool
}

// EphemeralResult is the result of a one-shot container execution.
type EphemeralResult struct {
	ContainerID string
	Stdout      string
	Stderr      string
	ExitCode    int
}

// RunEphemeralContainer creates, starts, execs, and optionally removes a container.
func RunEphemeralContainer(ctx context.Context, runtime Runtime, config *EphemeralConfig) (*EphemeralResult, error) {
	defer perf.Track(nil, "container.RunEphemeralContainer")()

	if runtime == nil || config == nil {
		return nil, errUtils.ErrNilParam
	}

	normalizeEphemeralConfig(config)

	if err := pullImageIfAlways(ctx, runtime, config); err != nil {
		return nil, err
	}

	containerID, err := createEphemeralContainer(ctx, runtime, config)
	if err != nil {
		return nil, fmt.Errorf("%w: create ephemeral container: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	result := &EphemeralResult{ContainerID: containerID}
	shouldRemove := false
	defer func() {
		if shouldRemove {
			removeEphemeralContainer(ctx, runtime, containerID)
		}
	}()

	if err := runtime.Start(ctx, containerID); err != nil {
		if config.CleanupPolicy == CleanupAlways {
			shouldRemove = true
		}
		return result, fmt.Errorf("%w: start container %q: %w", errUtils.ErrContainerRuntimeOperation, containerID, err)
	}

	execErr := execEphemeralCommand(ctx, runtime, containerID, config, result)
	// Compute the exit code from the raw exec error before wrapping it.
	result.ExitCode = ExitCode(execErr)

	if shouldCleanup(config.CleanupPolicy, execErr) {
		shouldRemove = true
	}

	if execErr != nil {
		return result, fmt.Errorf("%w: exec in container %q: %w", errUtils.ErrContainerRuntimeOperation, containerID, execErr)
	}
	return result, nil
}

// pullImageIfAlways pulls the image up front when the pull policy is PullAlways.
func pullImageIfAlways(ctx context.Context, runtime Runtime, config *EphemeralConfig) error {
	if config.PullPolicy != PullAlways {
		return nil
	}
	if err := runtime.Pull(ctx, config.Image); err != nil {
		return fmt.Errorf("%w: pull image %q: %w", errUtils.ErrContainerRuntimeOperation, config.Image, err)
	}
	return nil
}

// removeEphemeralContainer removes the container, using a non-cancelled context
// so cleanup still runs when the original ctx was cancelled or deadlined.
func removeEphemeralContainer(ctx context.Context, runtime Runtime, containerID string) {
	cleanupCtx := ctx
	if cleanupCtx.Err() != nil {
		cleanupCtx = context.Background()
	}
	_ = runtime.Remove(cleanupCtx, containerID, true)
}

func normalizeEphemeralConfig(config *EphemeralConfig) {
	if config.WorkspaceFolder == "" {
		config.WorkspaceFolder = "/workspace"
	}
	if config.PullPolicy == "" {
		config.PullPolicy = PullMissing
	}
	if config.CleanupPolicy == "" {
		config.CleanupPolicy = CleanupAlways
	}
}

func createEphemeralContainer(ctx context.Context, runtime Runtime, config *EphemeralConfig) (string, error) {
	// Errors returned here are wrapped with the container sentinel by the caller
	// (RunEphemeralContainer), so they stay plain to avoid a doubled sentinel.
	createConfig := buildEphemeralCreateConfig(config)
	containerID, err := runtime.Create(ctx, createConfig)
	// Only the missing-image case is recoverable by pulling. Any other create
	// failure (bad mount, invalid arg, daemon error) must surface as-is — pulling
	// then would mask the real cause behind a misleading registry/pull error.
	if err == nil || config.PullPolicy != PullMissing || !IsImageMissingError(err) {
		return containerID, err
	}

	if pullErr := runtime.Pull(ctx, config.Image); pullErr != nil {
		return "", fmt.Errorf(
			"failed to create container and pull image: %w",
			errors.Join(
				fmt.Errorf("create: %w", err),
				fmt.Errorf("pull: %w", pullErr),
			),
		)
	}
	return runtime.Create(ctx, createConfig)
}

// IsImageMissingError reports whether an error indicates the image is not
// present locally (and is therefore recoverable by pulling or building), as
// opposed to an unrelated failure (transport, auth, daemon) that neither fixes.
func IsImageMissingError(err error) bool {
	defer perf.Track(nil, "container.IsImageMissingError")()

	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"no such image",
		"image not known",
		"manifest unknown",
		"unable to find image",
		"image not found",
		"pull access denied",
		"repository does not exist",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func buildEphemeralCreateConfig(config *EphemeralConfig) *CreateConfig {
	mounts := append([]Mount{}, config.Mounts...)
	if config.WorkspaceHostPath != "" {
		mounts = append(mounts, Mount{
			Type:     "bind",
			Source:   config.WorkspaceHostPath,
			Target:   config.WorkspaceFolder,
			ReadOnly: config.WorkspaceReadOnly,
		})
	}

	return &CreateConfig{
		Name:            config.Name,
		Image:           config.Image,
		WorkspaceFolder: config.WorkspaceFolder,
		Mounts:          mounts,
		Ports:           config.Ports,
		User:            config.User,
		Labels:          config.Labels,
		RunArgs:         config.RunArgs,
		OverrideCommand: true,
	}
}

func execEphemeralCommand(ctx context.Context, runtime Runtime, containerID string, config *EphemeralConfig, result *EphemeralResult) error {
	opts := &ExecOptions{
		User:         config.User,
		WorkingDir:   config.WorkspaceFolder,
		Env:          config.Env,
		AttachStdin:  config.Interactive,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          config.TTY,
	}

	if config.Interactive {
		opts.Stdin = os.Stdin
	}

	if config.TTY || config.Interactive {
		return runtime.Exec(ctx, containerID, config.Command, opts)
	}

	var stdout, stderr bytes.Buffer
	opts.Stdout = &stdout
	opts.Stderr = &stderr

	err := runtime.Exec(ctx, containerID, config.Command, opts)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	return err
}

func shouldCleanup(policy string, execErr error) bool {
	switch policy {
	case CleanupNever:
		return false
	case CleanupOnSuccess:
		return execErr == nil
	default:
		return true
	}
}

// ExitCode extracts a process exit code from an error.
func ExitCode(err error) int {
	defer perf.Track(nil, "container.ExitCode")()

	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

// BuildEphemeralPreview builds a human-readable equivalent runtime command.
func BuildEphemeralPreview(runtimeName string, config *EphemeralConfig) string {
	defer perf.Track(nil, "container.BuildEphemeralPreview")()

	normalizeEphemeralConfig(config)
	if runtimeName == "" {
		runtimeName = runtimePlaceholder
	}

	args := []string{runtimeName, "run"}
	args = appendEphemeralPreviewFlags(args, config)
	args = appendEphemeralPreviewMounts(args, config)
	for _, port := range config.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d/%s", port.HostPort, port.ContainerPort, port.Protocol))
	}
	args = append(args, config.RunArgs...)
	args = append(args, config.Image)
	args = append(args, config.Command...)
	return strings.Join(args, spaceSeparator)
}

// appendEphemeralPreviewFlags appends the scalar run flags for the preview command.
func appendEphemeralPreviewFlags(args []string, config *EphemeralConfig) []string {
	if config.CleanupPolicy == CleanupAlways {
		args = append(args, "--rm")
	}
	if config.TTY {
		args = append(args, "-t")
	}
	if config.Interactive {
		args = append(args, "-i")
	}
	if config.User != "" {
		args = append(args, "--user", config.User)
	}
	if config.WorkspaceFolder != "" {
		args = append(args, "-w", config.WorkspaceFolder)
	}
	for _, env := range config.Env {
		args = append(args, "-e", env)
	}
	return args
}

// appendEphemeralPreviewMounts appends the resolved mount flags for the preview command.
func appendEphemeralPreviewMounts(args []string, config *EphemeralConfig) []string {
	for _, mount := range buildEphemeralCreateConfig(config).Mounts {
		mountStr := fmt.Sprintf("type=%s,source=%s,target=%s", mount.Type, mount.Source, mount.Target)
		if mount.ReadOnly {
			mountStr += ",readonly"
		}
		args = append(args, "--mount", mountStr)
	}
	return args
}

// BuildImageBuildPreview builds a human-readable equivalent image build command.
func BuildImageBuildPreview(runtimeName string, config *BuildConfig) string {
	defer perf.Track(nil, "container.BuildImageBuildPreview")()

	if runtimeName == "" {
		runtimeName = runtimePlaceholder
		if config.Engine == "buildx" || config.Bake != nil {
			runtimeName = "docker"
		}
	}
	args := append([]string{runtimeName}, buildBuildArgs(config)...)
	return strings.Join(args, spaceSeparator)
}

// BuildImageTagPreview builds a human-readable equivalent image tag command.
func BuildImageTagPreview(runtimeName, source, target string) string {
	defer perf.Track(nil, "container.BuildImageTagPreview")()

	if runtimeName == "" {
		runtimeName = runtimePlaceholder
	}
	args := append([]string{runtimeName}, buildTagArgs(source, target)...)
	return strings.Join(args, spaceSeparator)
}

// BuildImagePushPreview builds a human-readable equivalent image push command.
func BuildImagePushPreview(runtimeName, image string) string {
	defer perf.Track(nil, "container.BuildImagePushPreview")()

	if runtimeName == "" {
		runtimeName = runtimePlaceholder
	}
	args := append([]string{runtimeName}, buildPushArgs(image)...)
	return strings.Join(args, spaceSeparator)
}
