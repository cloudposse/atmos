package container

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// defaultStopTimeout is the grace period given to a long-lived container to
// shut down before it is killed.
const defaultStopTimeout = 10 * time.Second

// NamedConfig describes a long-lived, label-discovered container component
// instance. The container is named and labeled from the canonical component
// instance address (<stack>/<component_type>/<component>), so all subsequent
// lifecycle operations discover it by label rather than by local state.
type NamedConfig struct {
	// Canonical identity — drives the runtime name and labels.
	Stack         string
	ComponentType string
	Component     string

	// Container spec.
	Image       string
	Command     []string
	Ports       []PortBinding
	Networks    []NetworkAttachment
	Mounts      []Mount
	Env         map[string]string
	User        string
	Labels      map[string]string // extra labels, merged over the canonical instance labels
	RunArgs     []string
	Privileged  bool           // run the container in privileged mode
	Host        bool           // grant access to the host container runtime (Docker-out-of-Docker)
	Restart     *RestartPolicy // restart policy (nil = runtime default)
	HealthCheck *HealthCheck   // health check (nil = inherit image healthcheck)

	// Runtime selection and behavior.
	RuntimeName      string   // preferred runtime: "docker" | "podman" | "" (auto-detect)
	RuntimeAutoStart bool     // auto-init/start the Podman machine when needed
	RuntimeEnv       []string // environment forwarded to EnvSetter runtimes (e.g. registry auth)
	PullPolicy       string   // missing | always | never
	DryRun           bool
}

// Named is a started (or dry-run) long-lived container component instance.
type Named struct {
	config      NamedConfig
	runtime     Runtime
	containerID string
	name        string
	// AlreadyRunning is true when Up found the instance already running and made
	// no change.
	AlreadyRunning bool
}

// Up reconciles the desired long-lived container for a component instance:
// if it already exists and is running it is left untouched; if it exists but is
// stopped it is started; otherwise it is created (with canonical labels) and
// started. Discovery is by the canonical instance label, never local state.
func Up(ctx context.Context, config *NamedConfig) (*Named, error) {
	defer perf.Track(nil, "container.Up")()

	if config == nil {
		return nil, errUtils.ErrNilParam
	}
	if config.Image == "" {
		return nil, fmt.Errorf("%w: container image is required", errUtils.ErrContainerRuntimeOperation)
	}

	name := RuntimeName(config.Stack, config.ComponentType, config.Component)
	if config.DryRun {
		return &Named{config: *config, name: name}, nil
	}

	runtime, err := DetectRuntimeWithPreferenceAndRecovery(ctx, config.RuntimeName, config.RuntimeAutoStart)
	if err != nil {
		return nil, err
	}
	if setter, ok := runtime.(EnvSetter); ok {
		setter.SetEnv(config.RuntimeEnv)
	}

	return upWithRuntime(ctx, runtime, config, name)
}

// UpWithRuntime reconciles the desired long-lived container against an
// already-detected runtime. Callers that own runtime detection (e.g. to forward
// a resolved registry-auth environment) use this directly; Up is the
// convenience wrapper that detects the runtime itself.
func UpWithRuntime(ctx context.Context, runtime Runtime, config *NamedConfig) (*Named, error) {
	defer perf.Track(nil, "container.UpWithRuntime")()

	if runtime == nil || config == nil {
		return nil, errUtils.ErrNilParam
	}
	if config.Image == "" {
		return nil, fmt.Errorf("%w: container image is required", errUtils.ErrContainerRuntimeOperation)
	}
	name := RuntimeName(config.Stack, config.ComponentType, config.Component)
	return upWithRuntime(ctx, runtime, config, name)
}

// upWithRuntime reconciles the desired named container against the given runtime:
// it reuses a running instance, starts a stopped one, or creates and starts a new
// one. A created-but-unstartable container is cleaned up best-effort.
func upWithRuntime(ctx context.Context, runtime Runtime, config *NamedConfig, name string) (*Named, error) {
	existing, found, err := FindInstance(ctx, runtime, config.Stack, config.ComponentType, config.Component)
	if err != nil {
		return nil, err
	}
	if found {
		id := containerRef(existing)
		if IsContainerRunning(existing.Status) {
			return &Named{config: *config, runtime: runtime, containerID: id, name: name, AlreadyRunning: true}, nil
		}
		if err := runtime.Start(ctx, id); err != nil {
			return nil, fmt.Errorf("%w: start container %q: %w", errUtils.ErrContainerRuntimeOperation, id, err)
		}
		return &Named{config: *config, runtime: runtime, containerID: id, name: name}, nil
	}

	containerID, err := createNamedContainer(ctx, runtime, config, name)
	if err != nil {
		return nil, err
	}
	if err := runtime.Start(ctx, containerID); err != nil {
		// Best-effort cleanup of the created-but-unstarted container. If removal
		// also fails, surface both so operators can see the orphan.
		if rmErr := runtime.Remove(context.Background(), containerID, true); rmErr != nil {
			return nil, fmt.Errorf(
				"%w: start container %q and cleanup failed: %w",
				errUtils.ErrContainerRuntimeOperation,
				containerID,
				errors.Join(err, rmErr),
			)
		}
		return nil, fmt.Errorf("%w: start container %q: %w", errUtils.ErrContainerRuntimeOperation, containerID, err)
	}
	return &Named{config: *config, runtime: runtime, containerID: containerID, name: name}, nil
}

// createNamedContainer creates the named container, honoring the pull policy and
// recovering from a missing image by pulling once and retrying the create.
func createNamedContainer(ctx context.Context, runtime Runtime, config *NamedConfig, name string) (string, error) {
	createConfig := buildNamedCreateConfig(config, name)
	if config.PullPolicy == PullAlways {
		if err := pullWithRetry(ctx, runtime, config.Image); err != nil {
			return "", fmt.Errorf("%w: pull image %q: %w", errUtils.ErrContainerRuntimeOperation, config.Image, err)
		}
	}

	containerID, err := runtime.Create(ctx, createConfig)
	if err == nil {
		return containerID, nil
	}
	// Only a missing image is recoverable by pulling. Any other create failure
	// (bad mount, invalid arg, daemon error) must surface as-is so the real
	// cause is not masked behind a misleading registry error.
	if config.PullPolicy == PullNever || !IsImageMissingError(err) {
		return "", fmt.Errorf("%w: create container: %w", errUtils.ErrContainerRuntimeOperation, err)
	}
	createErr := err
	if pullErr := pullWithRetry(ctx, runtime, config.Image); pullErr != nil {
		return "", fmt.Errorf(
			"%w: create container and pull image: %w",
			errUtils.ErrContainerRuntimeOperation,
			errors.Join(createErr, pullErr),
		)
	}
	containerID, err = runtime.Create(ctx, createConfig)
	if err != nil {
		return "", fmt.Errorf("%w: create container after pull: %w", errUtils.ErrContainerRuntimeOperation, err)
	}
	return containerID, nil
}

// buildNamedCreateConfig assembles the runtime CreateConfig for a named container,
// merging canonical instance labels with any caller-supplied labels.
func buildNamedCreateConfig(config *NamedConfig, name string) *CreateConfig {
	labels := map[string]string{}
	for k, v := range config.Labels {
		labels[k] = v
	}
	for k, v := range InstanceLabels(config.Stack, config.ComponentType, config.Component) {
		labels[k] = v // reserved identity labels are authoritative.
	}

	return &CreateConfig{
		Name:        name,
		Image:       config.Image,
		Command:     config.Command,
		Mounts:      config.Mounts,
		Ports:       config.Ports,
		Networks:    config.Networks,
		Env:         config.Env,
		User:        config.User,
		Labels:      labels,
		RunArgs:     config.RunArgs,
		Privileged:  config.Privileged,
		Host:        config.Host,
		Restart:     config.Restart,
		HealthCheck: config.HealthCheck,
	}
}

// FindInstance discovers a long-lived container for a component instance by its
// canonical instance label. It returns the container info and whether a match
// was found.
func FindInstance(ctx context.Context, runtime Runtime, stack, componentType, component string) (*Info, bool, error) {
	defer perf.Track(nil, "container.FindInstance")()

	if runtime == nil {
		return nil, false, errUtils.ErrNilParam
	}

	containers, err := runtime.List(ctx, DiscoveryFilter(stack, componentType, component))
	if err != nil {
		return nil, false, fmt.Errorf("%w: list containers: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	instance := InstanceAddress(stack, componentType, component)
	for i := range containers {
		// Defend against runtimes whose label filter is a loose match: confirm
		// the canonical instance label exactly.
		if containers[i].Labels[LabelInstance] == instance {
			return &containers[i], true, nil
		}
	}
	return nil, false, nil
}

// Down stops and removes the long-lived container for a component instance,
// discovered by label. It is a no-op (nil error) when no instance is found.
func Down(ctx context.Context, runtime Runtime, stack, componentType, component string) error {
	defer perf.Track(nil, "container.Down")()

	info, found, err := FindInstance(ctx, runtime, stack, componentType, component)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	id := containerRef(info)
	if IsContainerRunning(info.Status) {
		if stopErr := runtime.Stop(ctx, id, defaultStopTimeout); stopErr != nil {
			return fmt.Errorf("%w: stop container %q: %w", errUtils.ErrContainerRuntimeOperation, id, stopErr)
		}
	}
	if rmErr := runtime.Remove(ctx, id, true); rmErr != nil {
		return fmt.Errorf("%w: remove container %q: %w", errUtils.ErrContainerRuntimeOperation, id, rmErr)
	}
	return nil
}

// ID returns the runtime container ID. In dry run it returns the generated name.
func (n *Named) ID() string {
	defer perf.Track(nil, "container.Named.ID")()

	if n == nil {
		return ""
	}
	if n.containerID != "" {
		return n.containerID
	}
	return n.name
}

// Name returns the generated runtime container name.
func (n *Named) Name() string {
	defer perf.Track(nil, "container.Named.Name")()

	if n == nil {
		return ""
	}
	return n.name
}

// containerRef returns the most specific identifier (ID, else Name) for a
// discovered container.
func containerRef(info *Info) string {
	if info == nil {
		return ""
	}
	if info.ID != "" {
		return info.ID
	}
	return info.Name
}
