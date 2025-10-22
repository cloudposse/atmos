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

// PodmanRuntime implements the Runtime interface for Podman.
type PodmanRuntime struct{}

// NewPodmanRuntime creates a new Podman runtime.
func NewPodmanRuntime() *PodmanRuntime {
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

	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman build failed: %v: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	log.Debug("Built podman image", "tags", config.Tags)
	return nil
}

// Create creates a new container.
func (p *PodmanRuntime) Create(ctx context.Context, config *CreateConfig) (string, error) {
	defer perf.Track(nil, "container.PodmanRuntime.Create")()

	args := buildCreateArgs(config)

	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: podman create failed: %v: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	containerID := strings.TrimSpace(string(output))
	log.Debug("Created podman container", "id", containerID, "name", config.Name)

	return containerID, nil
}

// Start starts a container.
func (p *PodmanRuntime) Start(ctx context.Context, containerID string) error {
	defer perf.Track(nil, "container.PodmanRuntime.Start")()

	cmd := exec.CommandContext(ctx, "podman", "start", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman start failed: %v: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	log.Debug("Started podman container", "id", containerID)
	return nil
}

// Stop stops a running container.
func (p *PodmanRuntime) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	defer perf.Track(nil, "container.PodmanRuntime.Stop")()

	timeoutSecs := int(timeout.Seconds())
	cmd := exec.CommandContext(ctx, "podman", "stop", "-t", fmt.Sprintf("%d", timeoutSecs), containerID) //nolint:gosec // podman command is intentional
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman stop failed: %v: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	log.Debug("Stopped podman container", "id", containerID)
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

	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman rm failed: %v: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	log.Debug("Removed podman container", "id", containerID)
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

	args := []string{"ps", "-a", "--format", "json"}

	// Add filters.
	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: podman ps failed: %v: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	// Podman returns a JSON array.
	var podmanContainers []map[string]interface{}
	if err := json.Unmarshal(output, &podmanContainers); err != nil {
		return nil, fmt.Errorf("%w: failed to parse podman output: %v", errUtils.ErrContainerRuntimeOperation, err)
	}

	var containers []Info
	for _, containerJSON := range podmanContainers {
		// Extract names (Podman returns an array of names).
		var name string
		if names, ok := containerJSON["Names"].([]interface{}); ok && len(names) > 0 {
			if n, ok := names[0].(string); ok {
				name = n
			}
		}

		info := Info{
			ID:     getString(containerJSON, "Id"),
			Name:   name,
			Image:  getString(containerJSON, "Image"),
			Status: getString(containerJSON, "State"),
		}

		// Parse labels if present.
		if labels, ok := containerJSON["Labels"].(map[string]interface{}); ok {
			info.Labels = make(map[string]string)
			for k, v := range labels {
				if s, ok := v.(string); ok {
					info.Labels[k] = s
				}
			}
		}

		containers = append(containers, info)
	}

	return containers, nil
}

// Exec executes a command in a running container.
func (p *PodmanRuntime) Exec(ctx context.Context, containerID string, cmd []string, opts *ExecOptions) error {
	defer perf.Track(nil, "container.PodmanRuntime.Exec")()

	return execWithRuntime(ctx, "podman", containerID, cmd, opts)
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

	cmd := exec.CommandContext(ctx, "podman", "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman pull failed: %v: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
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

	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// Info gets runtime information.
func (p *PodmanRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	defer perf.Track(nil, "container.PodmanRuntime.Info")()

	cmd := exec.CommandContext(ctx, "podman", "version", "--format", "{{.Version}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: podman version failed: %v: %s", errUtils.ErrContainerRuntimeOperation, err, string(output))
	}

	return &RuntimeInfo{
		Type:    string(TypePodman),
		Version: strings.TrimSpace(string(output)),
		Running: true,
	}, nil
}
