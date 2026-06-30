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
type PodmanRuntime struct {
	// env is the complete environment for podman CLI subprocesses. When nil the
	// commands inherit os.Environ(); when set (via SetEnv) it carries credentials
	// materialized by auth integrations, e.g. DOCKER_CONFIG for ECR login.
	env []string
}

// NewPodmanRuntime creates a new Podman runtime.
func NewPodmanRuntime() *PodmanRuntime {
	defer perf.Track(nil, "container.NewPodmanRuntime")()

	return &PodmanRuntime{}
}

// SetEnv sets the environment for podman CLI subprocesses launched by this runtime.
// See EnvSetter for the rationale.
func (p *PodmanRuntime) SetEnv(env []string) {
	defer perf.Track(nil, "container.PodmanRuntime.SetEnv")()

	p.env = env
}

// command builds a podman CLI command with the runtime's configured environment applied.
func (p *PodmanRuntime) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, podmanCmd, args...)
	applyCommandEnv(cmd, p.env)
	return cmd
}

// Build builds a container image from a Dockerfile.
func (p *PodmanRuntime) Build(ctx context.Context, config *BuildConfig) error {
	defer perf.Track(nil, "container.PodmanRuntime.Build")()

	if config.Engine == "buildx" || config.Bake != nil {
		return fmt.Errorf("%w: Docker Buildx requires Docker in V1; Podman uses the native `podman build` path", errUtils.ErrContainerRuntimeOperation)
	}

	args := buildBuildArgs(config)

	cmd := p.command(ctx, args...)
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

	cmd := p.command(ctx, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: podman create failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	// When podman pulls an image, it outputs pull progress followed by container ID on last line.
	// Extract the last non-empty line as the container ID.
	containerID := extractContainerID(output)
	if containerID == "" {
		return "", fmt.Errorf("%w: podman create returned no container ID", errUtils.ErrContainerRuntimeOperation)
	}

	log.Debug("Created podman container", logKeyID, containerID, "name", config.Name)

	return containerID, nil
}

// Start starts a container.
func (p *PodmanRuntime) Start(ctx context.Context, containerID string) error {
	defer perf.Track(nil, "container.PodmanRuntime.Start")()

	cmd := p.command(ctx, "start", containerID)
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
	args := buildStopArgs(containerID, timeoutSecs)

	cmd := p.command(ctx, args...)
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

	args := buildRemoveArgs(containerID, force)

	cmd := p.command(ctx, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman rm failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	log.Debug("Removed podman container", logKeyID, containerID)
	return nil
}

// findContainerByIDOrName searches for a container in the given list by ID or name.
func findContainerByIDOrName(containers []Info, searchID string) (*Info, error) {
	for i := range containers {
		if containers[i].ID == searchID || containers[i].Name == searchID {
			return &containers[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", errUtils.ErrContainerNotFound, searchID)
}

// Inspect gets detailed information about a container.
func (p *PodmanRuntime) Inspect(ctx context.Context, containerID string) (*Info, error) {
	defer perf.Track(nil, "container.PodmanRuntime.Inspect")()

	// Use List to find the container by ID or name.
	// This provides basic information until full JSON-based podman inspect is implemented.
	containers, err := p.List(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list containers: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	return findContainerByIDOrName(containers, containerID)
}

// List lists containers matching the given filters.
func (p *PodmanRuntime) List(ctx context.Context, filters map[string]string) ([]Info, error) {
	defer perf.Track(nil, "container.PodmanRuntime.List")()

	output, err := p.executePodmanList(ctx, filters)
	if err != nil {
		return nil, err
	}

	var podmanContainers []map[string]interface{}
	if err := json.Unmarshal(output, &podmanContainers); err != nil {
		return nil, fmt.Errorf("%w: failed to parse podman output: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	return parsePodmanContainers(podmanContainers), nil
}

// buildPodmanListArgs constructs the arguments for podman ps command.
func buildPodmanListArgs(filters map[string]string) []string {
	args := []string{"ps", "-a", "--format", "json"}

	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}

	return args
}

func (p *PodmanRuntime) executePodmanList(ctx context.Context, filters map[string]string) ([]byte, error) {
	args := buildPodmanListArgs(filters)

	cmd := p.command(ctx, args...)
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
		Health: podmanHealth(containerJSON),
	}

	if labels, ok := containerJSON["Labels"].(map[string]interface{}); ok {
		info.Labels = parseLabelsMap(labels)
	}
	if raw, ok := containerJSON["Ports"]; ok {
		info.Ports = parsePodmanPorts(raw)
	}

	return info
}

// parsePodmanPorts extracts published port bindings from a podman `ps --format json`
// record. Podman represents ports as a structured array with snake_case keys
// (host_port/container_port/protocol), unlike docker's `ps` string column parsed by
// parseDockerPorts. Without this, Info.Ports is empty under podman and emulator
// endpoint resolution yields an empty URL (see pkg/emulator manager.endpoint), so
// Terraform falls back to real cloud endpoints.
func parsePodmanPorts(raw interface{}) []PortBinding {
	entries, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	var ports []PortBinding
	seen := make(map[PortBinding]struct{})
	for _, entry := range entries {
		binding, span, ok := parsePodmanPort(entry)
		if !ok {
			continue
		}
		if span <= 0 {
			span = 1
		}
		// Expand consecutive host/container ports within the range.
		for i := 0; i < span; i++ {
			expanded := PortBinding{
				ContainerPort: binding.ContainerPort + i,
				HostPort:      binding.HostPort + i,
				Protocol:      binding.Protocol,
			}
			if _, dup := seen[expanded]; dup {
				continue
			}
			seen[expanded] = struct{}{}
			ports = append(ports, expanded)
		}
	}

	return ports
}

// parsePodmanPort converts one podman port entry into a PortBinding and the
// port range span. Unpublished ports (host_port 0) are skipped; a missing
// protocol defaults to tcp. The span field reflects Podman's `range` key for
// consecutive port ranges; callers must expand [base, base+span) themselves.
func parsePodmanPort(entry interface{}) (PortBinding, int, bool) {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return PortBinding{}, 0, false
	}
	hostPort := jsonFieldInt(m["host_port"])
	containerPort := jsonFieldInt(m["container_port"])
	if hostPort == 0 || containerPort == 0 {
		return PortBinding{}, 0, false
	}
	protocol, _ := m["protocol"].(string)
	if protocol == "" {
		protocol = "tcp"
	}
	span := jsonFieldInt(m["range"])
	if span <= 0 {
		span = 1
	}
	return PortBinding{ContainerPort: containerPort, HostPort: hostPort, Protocol: protocol}, span, true
}

// JsonFieldInt coerces a JSON-decoded numeric field to an int. The json.Unmarshal
// call into interface{} yields float64 for numbers; int and json.Number are handled defensively.
func jsonFieldInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

// podmanHealth extracts the health state from a podman ps record. Podman embeds
// the health token in the human `.Status` string (like docker) and, on newer
// versions, exposes a machine-readable `.Health` field; both are checked.
func podmanHealth(containerJSON map[string]interface{}) string {
	if h := parseHealth(getString(containerJSON, "Status")); h != "" {
		return h
	}
	return normalizeHealth(getString(containerJSON, "Health"))
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

	return runExecCommand(p.command(ctx, buildExecArgs(containerID, cmd, opts)...), podmanCmd, opts)
}

// Shell opens an interactive shell in a running container (a new shell process via `exec`).
func (p *PodmanRuntime) Shell(ctx context.Context, containerID string, opts *ShellOptions) error {
	defer perf.Track(nil, "container.PodmanRuntime.Shell")()

	cmd, execOpts := buildShellCommand(opts)
	return p.Exec(ctx, containerID, cmd, execOpts)
}

// Attach connects to a running container's main process (PID 1) via `podman attach`.
func (p *PodmanRuntime) Attach(ctx context.Context, containerID string, opts *AttachOptions) error {
	defer perf.Track(nil, "container.PodmanRuntime.Attach")()

	args, execOpts := buildAttachArgs(containerID, opts)
	return runExecCommand(p.command(ctx, args...), podmanCmd, execOpts)
}

// Pull pulls a container image.
func (p *PodmanRuntime) Pull(ctx context.Context, image string) error {
	defer perf.Track(nil, "container.PodmanRuntime.Pull")()

	cmd := p.command(ctx, "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman pull failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	return nil
}

// Tag tags a container image.
func (p *PodmanRuntime) Tag(ctx context.Context, source, target string) error {
	defer perf.Track(nil, "container.PodmanRuntime.Tag")()

	cmd := p.command(ctx, buildTagArgs(source, target)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: podman tag failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	return nil
}

// Push pushes a container image.
func (p *PodmanRuntime) Push(ctx context.Context, image string) (*PushResult, error) {
	defer perf.Track(nil, "container.PodmanRuntime.Push")()

	cmd := p.command(ctx, buildPushArgs(image)...)
	output, err := cmd.CombinedOutput()
	cleanOutput := cleanPodmanOutput(output)
	result := &PushResult{
		Image:  image,
		Digest: parsePushDigest(cleanOutput),
		Output: cleanOutput,
	}
	if err != nil {
		return result, fmt.Errorf("%w: podman push failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanOutput)
	}

	return result, nil
}

// ImageInspect returns metadata for a local container image.
func (p *PodmanRuntime) ImageInspect(ctx context.Context, image string) (*ImageInfo, error) {
	defer perf.Track(nil, "container.PodmanRuntime.ImageInspect")()

	cmd := p.command(ctx, buildImageInspectArgs(image)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: podman image inspect failed: %w: %s", errUtils.ErrContainerRuntimeOperation, err, cleanPodmanOutput(output))
	}

	return parseImageInspectOutput(output)
}

// Logs shows logs from a container.
//
//nolint:revive // argument-limit: Logs keeps separate IO parameters for simplicity
func (p *PodmanRuntime) Logs(ctx context.Context, containerID string, follow bool, tail string, stdout, stderr io.Writer) error {
	defer perf.Track(nil, "container.PodmanRuntime.Logs")()

	// Default to iolib.Data/UI if nil.
	if stdout == nil {
		stdout = iolib.Data
	}
	if stderr == nil {
		stderr = iolib.UI
	}

	args := buildLogsArgs(containerID, follow, tail)

	cmd := p.command(ctx, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return cmd.Run()
}

// Info gets runtime information.
func (p *PodmanRuntime) Info(ctx context.Context) (*RuntimeInfo, error) {
	defer perf.Track(nil, "container.PodmanRuntime.Info")()

	cmd := p.command(ctx, "version", "--format", "{{.Version}}")
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
