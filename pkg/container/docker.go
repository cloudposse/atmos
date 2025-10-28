package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	dockerCmd    = "docker"
	logKeyID     = "id"
	logKeyImage  = "image"
	logKeyStatus = "status"
)

// DockerRuntime implements the Runtime interface for Docker.
type DockerRuntime struct{}

// NewDockerRuntime creates a new Docker runtime.
func NewDockerRuntime() *DockerRuntime {
	defer perf.Track(nil, "container.NewDockerRuntime")()

	return &DockerRuntime{}
}

// Build builds a container image from a Dockerfile.
func (d *DockerRuntime) Build(ctx context.Context, config *BuildConfig) error {
	defer perf.Track(nil, "container.DockerRuntime.Build")()

	args := []string{"build"}

	// Add build args
	for key, value := range config.Args {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
	}

	// Add tags
	for _, tag := range config.Tags {
		args = append(args, "-t", tag)
	}

	// Add context and dockerfile
	args = append(args, "-f", config.Dockerfile, config.Context)

	cmd := exec.CommandContext(ctx, dockerCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: docker build failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	log.Debug("Built docker image", "tags", config.Tags)
	return nil
}

// Create creates a new container.
func (d *DockerRuntime) Create(ctx context.Context, config *CreateConfig) (string, error) {
	defer perf.Track(nil, "container.DockerRuntime.Create")()

	args := buildCreateArgs(config)

	cmd := exec.CommandContext(ctx, dockerCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: docker create failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	containerID := strings.TrimSpace(string(output))
	log.Debug("Created docker container", logKeyID, containerID, "name", config.Name)

	return containerID, nil
}

// Start starts a container.
func (d *DockerRuntime) Start(ctx context.Context, containerID string) error {
	defer perf.Track(nil, "container.DockerRuntime.Start")()

	cmd := exec.CommandContext(ctx, dockerCmd, "start", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: docker start failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	log.Debug("Started docker container", logKeyID, containerID)
	return nil
}

// Stop stops a running container.
func (d *DockerRuntime) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	defer perf.Track(nil, "container.DockerRuntime.Stop")()

	timeoutSecs := int(timeout.Seconds())
	cmd := exec.CommandContext(ctx, dockerCmd, "stop", "-t", fmt.Sprintf("%d", timeoutSecs), containerID) //nolint:gosec // docker command is intentional
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: docker stop failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	log.Debug("Stopped docker container", logKeyID, containerID)
	return nil
}

// Remove removes a container.
func (d *DockerRuntime) Remove(ctx context.Context, containerID string, force bool) error {
	defer perf.Track(nil, "container.DockerRuntime.Remove")()

	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, containerID)

	cmd := exec.CommandContext(ctx, dockerCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: docker rm failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	log.Debug("Removed docker container", logKeyID, containerID)
	return nil
}

// Inspect gets detailed information about a container.
func (d *DockerRuntime) Inspect(ctx context.Context, containerID string) (*Info, error) {
	defer perf.Track(nil, "container.DockerRuntime.Inspect")()

	// TODO: Implement actual inspection using docker inspect with JSON output.
	return nil, errUtils.ErrNotImplemented
}

// List lists containers matching the given filters.
func (d *DockerRuntime) List(ctx context.Context, filters map[string]string) ([]Info, error) {
	defer perf.Track(nil, "container.DockerRuntime.List")()

	args := []string{"ps", "-a", "--format", "{{json .}}"}

	// Add filters.
	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.CommandContext(ctx, dockerCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: docker ps failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	// Parse JSON output line by line.
	var containers []Info
	scanner := strings.Split(string(output), "\n")
	for _, line := range scanner {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse JSON line.
		var containerJSON map[string]interface{}
		if err := json.Unmarshal([]byte(line), &containerJSON); err != nil {
			log.Debug("Failed to parse container JSON", "error", err, "line", line)
			continue
		}

		info := Info{
			ID:     getString(containerJSON, "ID"),
			Name:   strings.TrimPrefix(getString(containerJSON, "Names"), "/"),
			Image:  getString(containerJSON, "Image"),
			Status: getString(containerJSON, "Status"),
		}

		// Parse labels if present.
		if labelsStr := getString(containerJSON, "Labels"); labelsStr != "" {
			info.Labels = parseLabels(labelsStr)
		}

		containers = append(containers, info)
	}

	return containers, nil
}

// getString safely gets a string value from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// parseLabels parses Docker labels from comma-separated format.
// Format: "label1=value1,label2=value2".
func parseLabels(labelsStr string) map[string]string {
	labels := make(map[string]string)
	parts := strings.Split(labelsStr, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			labels[kv[0]] = kv[1]
		}
	}
	return labels
}

// Exec executes a command in a running container.
func (d *DockerRuntime) Exec(ctx context.Context, containerID string, cmd []string, opts *ExecOptions) error {
	defer perf.Track(nil, "container.DockerRuntime.Exec")()

	return execWithRuntime(ctx, dockerCmd, containerID, cmd, opts)
}

// Attach attaches to a running container with an interactive shell.
func (d *DockerRuntime) Attach(ctx context.Context, containerID string, opts *AttachOptions) error {
	defer perf.Track(nil, "container.DockerRuntime.Attach")()

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

	return d.Exec(ctx, containerID, cmd, execOpts)
}

// Pull pulls a container image.
func (d *DockerRuntime) Pull(ctx context.Context, image string) error {
	defer perf.Track(nil, "container.DockerRuntime.Pull")()

	cmd := exec.CommandContext(ctx, dockerCmd, "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: docker pull failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	return nil
}

// Logs shows logs from a container.
func (d *DockerRuntime) Logs(ctx context.Context, containerID string, follow bool, tail string) error {
	defer perf.Track(nil, "container.DockerRuntime.Logs")()

	args := []string{"logs"}

	if follow {
		args = append(args, "--follow")
	}

	if tail != "" && tail != "all" {
		args = append(args, "--tail", tail)
	}

	args = append(args, containerID)

	cmd := exec.CommandContext(ctx, dockerCmd, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// Info gets runtime information.
func (d *DockerRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	defer perf.Track(nil, "container.DockerRuntime.Info")()

	cmd := exec.CommandContext(ctx, dockerCmd, "version", "--format", "{{.Server.Version}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: docker version failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	return &RuntimeInfo{
		Type:    string(TypeDocker),
		Version: strings.TrimSpace(string(output)),
		Running: true,
	}, nil
}
