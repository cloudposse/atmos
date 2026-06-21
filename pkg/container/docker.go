package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
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
type DockerRuntime struct {
	// env is the complete environment for docker CLI subprocesses. When nil the
	// commands inherit os.Environ(); when set (via SetEnv) it carries credentials
	// materialized by auth integrations, e.g. DOCKER_CONFIG for ECR login.
	env []string
}

// NewDockerRuntime creates a new Docker runtime.
func NewDockerRuntime() *DockerRuntime {
	defer perf.Track(nil, "container.NewDockerRuntime")()

	return &DockerRuntime{}
}

// SetEnv sets the environment for docker CLI subprocesses launched by this runtime.
// See EnvSetter for the rationale.
func (d *DockerRuntime) SetEnv(env []string) {
	defer perf.Track(nil, "container.DockerRuntime.SetEnv")()

	d.env = env
}

// command builds a docker CLI command with the runtime's configured environment applied.
func (d *DockerRuntime) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, dockerCmd, args...)
	applyCommandEnv(cmd, d.env)
	return cmd
}

// Build builds a container image from a Dockerfile.
func (d *DockerRuntime) Build(ctx context.Context, config *BuildConfig) error {
	defer perf.Track(nil, "container.DockerRuntime.Build")()

	args := buildBuildArgs(config)

	cmd := d.command(ctx, args...)
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

	cmd := d.command(ctx, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: docker create failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	// `docker create` pulls the image inline when it is missing locally, printing pull
	// progress before the container ID, so the ID is the final non-empty line of output.
	containerID := extractContainerID(output)
	if containerID == "" {
		return "", fmt.Errorf("%w: docker create returned no container ID", errUtils.ErrContainerRuntimeOperation)
	}

	log.Debug("Created docker container", logKeyID, containerID, "name", config.Name)

	return containerID, nil
}

// Start starts a container.
func (d *DockerRuntime) Start(ctx context.Context, containerID string) error {
	defer perf.Track(nil, "container.DockerRuntime.Start")()

	cmd := d.command(ctx, "start", containerID)
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
	args := buildStopArgs(containerID, timeoutSecs)

	cmd := d.command(ctx, args...)
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

	args := buildRemoveArgs(containerID, force)

	cmd := d.command(ctx, args...)
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

	data, err := d.runDockerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	info := d.parseInspectData(data)
	return info, nil
}

// runDockerInspect executes docker inspect and returns parsed JSON.
func (d *DockerRuntime) runDockerInspect(ctx context.Context, containerID string) (map[string]interface{}, error) {
	cmd := d.command(ctx, "inspect", "--format", "{{json .}}", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: docker inspect failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	var data map[string]interface{}
	if err := json.Unmarshal(output, &data); err != nil {
		return nil, fmt.Errorf("%w: failed to parse docker inspect output: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	return data, nil
}

// parseInspectData converts docker inspect JSON into Info struct.
func (d *DockerRuntime) parseInspectData(data map[string]interface{}) *Info {
	info := &Info{
		ID:    getString(data, "Id"),
		Name:  strings.TrimPrefix(getString(data, "Name"), "/"),
		Image: getString(data, "Image"),
	}

	// Use .State.Status when available (machine-readable), fall back to .Status (human-readable).
	info.Status = getStatusFromInspect(data)

	// Parse created timestamp.
	if created := getString(data, "Created"); created != "" {
		if ts, err := time.Parse(time.RFC3339Nano, created); err == nil {
			info.Created = ts
		}
	}

	// Parse labels.
	info.Labels = getLabelsFromInspect(data)

	return info
}

// getStatusFromInspect extracts status from inspect data, preferring .State.Status.
func getStatusFromInspect(data map[string]interface{}) string {
	if state, ok := data["State"].(map[string]interface{}); ok {
		if status := getString(state, "Status"); status != "" {
			return status
		}
	}
	return getString(data, "Status")
}

// getLabelsFromInspect extracts labels from inspect data.
func getLabelsFromInspect(data map[string]interface{}) map[string]string {
	config, ok := data["Config"].(map[string]interface{})
	if !ok {
		return nil
	}

	labels, ok := config["Labels"].(map[string]interface{})
	if !ok || len(labels) == 0 {
		return nil
	}

	result := make(map[string]string, len(labels))
	for k, v := range labels {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

// List lists containers matching the given filters.
func (d *DockerRuntime) List(ctx context.Context, filters map[string]string) ([]Info, error) {
	defer perf.Track(nil, "container.DockerRuntime.List")()

	args := []string{"ps", "-a", "--format", "{{json .}}"}

	// Add filters.
	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}

	cmd := d.command(ctx, args...)
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

		// Use .State when available (machine-readable), fall back to .Status (human-readable).
		status := getString(containerJSON, "State")
		if status == "" {
			status = getString(containerJSON, "Status")
		}

		info := Info{
			ID:     getString(containerJSON, "ID"),
			Name:   strings.TrimPrefix(getString(containerJSON, "Names"), "/"),
			Image:  getString(containerJSON, "Image"),
			Status: status,
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

	return runExecCommand(d.command(ctx, buildExecArgs(containerID, cmd, opts)...), dockerCmd, opts)
}

// Shell opens an interactive shell in a running container (a new shell process via `exec`).
func (d *DockerRuntime) Shell(ctx context.Context, containerID string, opts *ShellOptions) error {
	defer perf.Track(nil, "container.DockerRuntime.Shell")()

	cmd, execOpts := buildShellCommand(opts)
	return d.Exec(ctx, containerID, cmd, execOpts)
}

// Attach connects to a running container's main process (PID 1) via `docker attach`.
func (d *DockerRuntime) Attach(ctx context.Context, containerID string, opts *AttachOptions) error {
	defer perf.Track(nil, "container.DockerRuntime.Attach")()

	args, execOpts := buildAttachArgs(containerID, opts)
	return runExecCommand(d.command(ctx, args...), dockerCmd, execOpts)
}

// Pull pulls a container image.
func (d *DockerRuntime) Pull(ctx context.Context, image string) error {
	defer perf.Track(nil, "container.DockerRuntime.Pull")()

	cmd := d.command(ctx, "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: docker pull failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	return nil
}

// Tag tags a container image.
func (d *DockerRuntime) Tag(ctx context.Context, source, target string) error {
	defer perf.Track(nil, "container.DockerRuntime.Tag")()

	cmd := d.command(ctx, buildTagArgs(source, target)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: docker tag failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	return nil
}

// Push pushes a container image.
func (d *DockerRuntime) Push(ctx context.Context, image string) (*PushResult, error) {
	defer perf.Track(nil, "container.DockerRuntime.Push")()

	cmd := d.command(ctx, buildPushArgs(image)...)
	output, err := cmd.CombinedOutput()
	result := &PushResult{
		Image:  image,
		Digest: parsePushDigest(string(output)),
		Output: string(output),
	}
	if err != nil {
		return result, fmt.Errorf("%w: docker push failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	return result, nil
}

// ImageInspect returns metadata for a local container image.
func (d *DockerRuntime) ImageInspect(ctx context.Context, image string) (*ImageInfo, error) {
	defer perf.Track(nil, "container.DockerRuntime.ImageInspect")()

	cmd := d.command(ctx, buildImageInspectArgs(image)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: docker image inspect failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	return parseImageInspectOutput(output)
}

// Logs shows logs from a container.
//
//nolint:revive // argument-limit: Logs keeps separate IO parameters for simplicity
func (d *DockerRuntime) Logs(ctx context.Context, containerID string, follow bool, tail string, stdout, stderr io.Writer) error {
	defer perf.Track(nil, "container.DockerRuntime.Logs")()

	// Default to iolib.Data/UI if nil.
	if stdout == nil {
		stdout = iolib.Data
	}
	if stderr == nil {
		stderr = iolib.UI
	}

	args := buildLogsArgs(containerID, follow, tail)

	cmd := d.command(ctx, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return cmd.Run()
}

// Info gets runtime information.
func (d *DockerRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	defer perf.Track(nil, "container.DockerRuntime.Info")()

	cmd := d.command(ctx, "version", "--format", "{{.Server.Version}}")
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
