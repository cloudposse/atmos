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
	podmanCmd = "podman"
)

// cleanPodmanOutput cleans up podman output by unescaping literal \n, \t, etc.
func cleanPodmanOutput(output []byte) string {
	s := string(output)
	// Unescape common escape sequences that podman outputs as literal strings.
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\r", "\r")
	return strings.TrimSpace(s)
}

// PodmanRuntime implements the Runtime interface for Podman.
type PodmanRuntime struct{}

// NewPodmanRuntime creates a new Podman runtime.
func NewPodmanRuntime() *PodmanRuntime {
	defer perf.Track(nil, "container.NewPodmanRuntime")()

	return &PodmanRuntime{}
}

// Build builds a container image from a Dockerfile.
func (p *PodmanRuntime) Build(ctx context.Context, config *BuildConfig) error {
	defer perf.Track(nil, "container.PodmanRuntime.Build")()

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

	cmd := exec.CommandContext(ctx, podmanCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman build failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	log.Debug("Built podman image", "tags", config.Tags)
	return nil
}

// Create creates a new container.
func (p *PodmanRuntime) Create(ctx context.Context, config *CreateConfig) (string, error) {
	defer perf.Track(nil, "container.PodmanRuntime.Create")()

	args := buildCreateArgs(config)

	cmd := exec.CommandContext(ctx, podmanCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: podman create failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	// When podman pulls an image, it outputs pull progress followed by container ID on last line.
	// Extract the last non-empty line as the container ID.
	lines := strings.Split(string(output), "\n")
	var containerID string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			containerID = line
			break
		}
	}

	if containerID == "" {
		return "", fmt.Errorf("%w: podman create returned no container ID", errUtils.ErrContainerRuntimeOperation)
	}

	log.Debug("Created podman container", logKeyID, containerID, "name", config.Name)

	return containerID, nil
}

// Start starts a container.
func (p *PodmanRuntime) Start(ctx context.Context, containerID string) error {
	defer perf.Track(nil, "container.PodmanRuntime.Start")()

	cmd := exec.CommandContext(ctx, podmanCmd, "start", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman start failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	log.Debug("Started podman container", logKeyID, containerID)
	return nil
}

// Stop stops a running container.
func (p *PodmanRuntime) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	defer perf.Track(nil, "container.PodmanRuntime.Stop")()

	timeoutSecs := int(timeout.Seconds())
	cmd := exec.CommandContext(ctx, podmanCmd, "stop", "-t", fmt.Sprintf("%d", timeoutSecs), containerID) //nolint:gosec // podman command is intentional
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman stop failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	log.Debug("Stopped podman container", logKeyID, containerID)
	return nil
}

// Remove removes a container.
func (p *PodmanRuntime) Remove(ctx context.Context, containerID string, force bool) error {
	defer perf.Track(nil, "container.PodmanRuntime.Remove")()

	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, containerID)

	cmd := exec.CommandContext(ctx, podmanCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman rm failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	log.Debug("Removed podman container", logKeyID, containerID)
	return nil
}

// Inspect gets detailed information about a container.
func (p *PodmanRuntime) Inspect(ctx context.Context, containerID string) (*Info, error) {
	defer perf.Track(nil, "container.PodmanRuntime.Inspect")()

	// TODO: Implement actual inspection using podman inspect with JSON output.
	return nil, errUtils.ErrNotImplemented
}

// List lists containers matching the given filters.
func (p *PodmanRuntime) List(ctx context.Context, filters map[string]string) ([]Info, error) {
	defer perf.Track(nil, "container.PodmanRuntime.List")()

	output, err := executePodmanList(ctx, filters)
	if err != nil {
		return nil, err
	}

	var podmanContainers []map[string]interface{}
	if err := json.Unmarshal(output, &podmanContainers); err != nil {
		return nil, fmt.Errorf("%w: failed to parse podman output: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	return parsePodmanContainers(podmanContainers), nil
}

func executePodmanList(ctx context.Context, filters map[string]string) ([]byte, error) {
	args := []string{"ps", "-a", "--format", "json"}

	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.CommandContext(ctx, podmanCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: podman ps failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	return output, nil
}

func parsePodmanContainers(podmanContainers []map[string]interface{}) []Info {
	containers := make([]Info, 0, len(podmanContainers))

	for _, containerJSON := range podmanContainers {
		info := parsePodmanContainer(containerJSON)
		containers = append(containers, info)
	}

	return containers
}

func parsePodmanContainer(containerJSON map[string]interface{}) Info {
	name := extractPodmanName(containerJSON)

	info := Info{
		ID:     getString(containerJSON, "Id"),
		Name:   name,
		Image:  getString(containerJSON, "Image"),
		Status: getString(containerJSON, "State"),
	}

	if labels, ok := containerJSON["Labels"].(map[string]interface{}); ok {
		info.Labels = parseLabelsMap(labels)
	}

	return info
}

func extractPodmanName(containerJSON map[string]interface{}) string {
	names, ok := containerJSON["Names"].([]interface{})
	if !ok || len(names) == 0 {
		return ""
	}

	if n, ok := names[0].(string); ok {
		return n
	}

	return ""
}

func parseLabelsMap(labels map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range labels {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

// Exec executes a command in a running container.
func (p *PodmanRuntime) Exec(ctx context.Context, containerID string, cmd []string, opts *ExecOptions) error {
	defer perf.Track(nil, "container.PodmanRuntime.Exec")()

	return execWithRuntime(ctx, podmanCmd, containerID, cmd, opts)
}

// Attach attaches to a running container with an interactive shell.
func (p *PodmanRuntime) Attach(ctx context.Context, containerID string, opts *AttachOptions) error {
	defer perf.Track(nil, "container.PodmanRuntime.Attach")()

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

	return p.Exec(ctx, containerID, cmd, execOpts)
}

// Pull pulls a container image.
func (p *PodmanRuntime) Pull(ctx context.Context, image string) error {
	defer perf.Track(nil, "container.PodmanRuntime.Pull")()

	cmd := exec.CommandContext(ctx, podmanCmd, "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman pull failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	return nil
}

// Logs shows logs from a container.
func (p *PodmanRuntime) Logs(ctx context.Context, containerID string, follow bool, tail string) error {
	defer perf.Track(nil, "container.PodmanRuntime.Logs")()

	args := []string{"logs"}

	if follow {
		args = append(args, "--follow")
	}

	if tail != "" && tail != "all" {
		args = append(args, "--tail", tail)
	}

	args = append(args, containerID)

	cmd := exec.CommandContext(ctx, podmanCmd, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// Info gets runtime information.
func (p *PodmanRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	defer perf.Track(nil, "container.PodmanRuntime.Info")()

	cmd := exec.CommandContext(ctx, podmanCmd, "version", "--format", "{{.Version}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: podman version failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	return &RuntimeInfo{
		Type:    string(TypePodman),
		Version: strings.TrimSpace(string(output)),
		Running: true,
	}, nil
}
