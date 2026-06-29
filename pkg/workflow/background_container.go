package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/shlex"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/background"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// backgroundComponentType labels long-lived background container instances so they
// are discoverable by the container runtime and distinct from container components.
const backgroundComponentType = "workflow-background"

// readyTimeout bounds how long a background container service may take to report
// healthy before the wait/implicit-gate fails. Mirrors container.DefaultHealthyTimeout.
const readyTimeout = container.DefaultHealthyTimeout

// ContainerRunner starts background container services using Atmos's existing
// long-lived container lifecycle (Up/WaitHealthy/Down). It implements
// background.Runner. The container runtime supervises the process, so no goroutine
// is required.
type ContainerRunner struct {
	// Stack scopes the instance label namespace so background steps in different
	// stacks resolve to distinct instances.
	Stack string
	// DryRun skips the actual runtime calls (start/wait/stop become no-ops).
	DryRun bool
}

// Start launches the background container described by the step's `with:` block
// (step.Run) detached, and returns a handle that waits on its health check and
// tears it down.
func (cr *ContainerRunner) Start(ctx context.Context, step *schema.WorkflowStep, env []string) (background.Handle, error) {
	defer perf.Track(nil, "workflow.ContainerRunner.Start")()

	if step.Run == nil || strings.TrimSpace(step.Run.Image) == "" {
		return nil, fmt.Errorf("%w: background container step %q requires `with.image`", schema.ErrWorkflowControlStepInvalid, step.Name)
	}

	run := step.Run
	command, err := shlex.Split(run.Command)
	if err != nil {
		return nil, fmt.Errorf("%w: background container %q command %q: %w", schema.ErrWorkflowControlStepInvalid, step.Name, run.Command, err)
	}

	cfg := &container.NamedConfig{
		Stack:            cr.Stack,
		ComponentType:    backgroundComponentType,
		Component:        step.Name,
		Image:            run.Image,
		Command:          command,
		Ports:            toPortBindings(run.Ports),
		Mounts:           toMounts(run.Mounts),
		Env:              envSliceToMap(env),
		User:             run.User,
		Restart:          toRestartPolicy(run.Restart),
		HealthCheck:      toHealthCheck(run.HealthCheck),
		RuntimeName:      firstNonEmpty(run.Provider, step.Provider),
		RuntimeAutoStart: run.RuntimeAutoStart || step.RuntimeAutoStart,
		PullPolicy:       run.Pull,
		DryRun:           cr.DryRun,
	}

	handle := &containerHandle{
		name: step.Name,
		inst: container.Instance{
			Stack:         cr.Stack,
			ComponentType: backgroundComponentType,
			Component:     step.Name,
		},
		hasHealthCheck: cfg.HealthCheck != nil && !cfg.HealthCheck.Disable,
		dryRun:         cr.DryRun,
	}

	if cr.DryRun {
		return handle, nil
	}

	runtime, err := container.DetectRuntimeWithPreferenceAndRecovery(ctx, cfg.RuntimeName, cfg.RuntimeAutoStart)
	if err != nil {
		return nil, err
	}
	if _, err := container.UpWithRuntime(ctx, runtime, cfg); err != nil {
		return nil, fmt.Errorf("%w: start background container %q: %w", errUtils.ErrContainerRuntimeOperation, step.Name, err)
	}
	handle.runtime = runtime
	return handle, nil
}

// containerHandle supervises a started background container instance.
type containerHandle struct {
	name           string
	runtime        container.Runtime
	inst           container.Instance
	hasHealthCheck bool
	dryRun         bool
}

func (h *containerHandle) Name() string { return h.name }

// WaitReady blocks until the container reports healthy, reusing container.WaitHealthy.
// When the step declared no health check, "started" is treated as "ready".
func (h *containerHandle) WaitReady(ctx context.Context) error {
	defer perf.Track(nil, "workflow.containerHandle.WaitReady")()

	if h.dryRun || !h.hasHealthCheck {
		return nil
	}
	return container.WaitHealthy(ctx, h.runtime, h.inst, readyTimeout)
}

// Stop tears the container down (stop+remove) via container.Down.
func (h *containerHandle) Stop(ctx context.Context) error {
	defer perf.Track(nil, "workflow.containerHandle.Stop")()

	if h.dryRun || h.runtime == nil {
		return nil
	}
	if err := container.Down(ctx, h.runtime, h.inst.Stack, h.inst.ComponentType, h.inst.Component); err != nil {
		// Wrap at the handle boundary so implicit StopAll teardown preserves the
		// static sentinel and step-name context that Start adds on the startup path.
		return fmt.Errorf("%w: stop background container %q: %w", errUtils.ErrContainerRuntimeOperation, h.name, err)
	}
	return nil
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// defaultPortProtocol is assumed when a port mapping omits its protocol.
const defaultPortProtocol = "tcp"

// toPortBindings maps schema port specs onto runtime PortBindings.
func toPortBindings(ports []schema.ContainerPort) []container.PortBinding {
	if len(ports) == 0 {
		return nil
	}
	out := make([]container.PortBinding, 0, len(ports))
	for _, p := range ports {
		protocol := p.Protocol
		if protocol == "" {
			protocol = defaultPortProtocol
		}
		out = append(out, container.PortBinding{
			HostPort:      p.Host,
			ContainerPort: p.Container,
			Protocol:      protocol,
		})
	}
	return out
}

// toMounts maps schema mounts onto runtime Mounts.
func toMounts(mounts []schema.ContainerMount) []container.Mount {
	if len(mounts) == 0 {
		return nil
	}
	out := make([]container.Mount, 0, len(mounts))
	for _, m := range mounts {
		out = append(out, container.Mount{
			Type:     m.Type,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}
	return out
}

// toRestartPolicy maps the schema restart spec onto a runtime RestartPolicy, or nil
// when none is configured (the runtime then uses its default of `no`).
func toRestartPolicy(restart *schema.ContainerRestart) *container.RestartPolicy {
	if restart == nil || restart.Policy == "" {
		return nil
	}
	return &container.RestartPolicy{Policy: restart.Policy, MaxRetries: restart.MaxRetries}
}

// toHealthCheck maps the schema healthcheck onto a runtime HealthCheck, resolving
// the Compose `test` form (a bare string or a list whose first element is `NONE`,
// `CMD`, or `CMD-SHELL`) into a single shell command. Returns nil when no
// healthcheck is configured (the container inherits any image healthcheck).
func toHealthCheck(hc *schema.ContainerHealthCheck) *container.HealthCheck {
	if hc == nil {
		return nil
	}
	cmd, disable := resolveHealthTest(hc.Test)
	if hc.Disable {
		disable = true
	}
	if disable {
		return &container.HealthCheck{Disable: true}
	}
	return &container.HealthCheck{
		Cmd:           cmd,
		Interval:      hc.Interval,
		Timeout:       hc.Timeout,
		Retries:       hc.Retries,
		StartPeriod:   hc.StartPeriod,
		StartInterval: hc.StartInterval,
	}
}

// resolveHealthTest resolves a Compose `test` value into a single shell command for
// `--health-cmd`: a leading `NONE` disables the check, `CMD`/`CMD-SHELL` strip the
// prefix, and an unprefixed value is treated as `CMD-SHELL`.
func resolveHealthTest(test []string) (cmd string, disable bool) {
	if len(test) == 0 {
		return "", false
	}
	switch {
	case strings.EqualFold(test[0], "NONE"):
		return "", true
	case strings.EqualFold(test[0], "CMD"), strings.EqualFold(test[0], "CMD-SHELL"):
		return strings.Join(test[1:], " "), false
	default:
		return strings.Join(test, " "), false
	}
}
