package container

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_runtime_test.go -package=container github.com/cloudposse/atmos/pkg/container Runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/composition"
	cfg "github.com/cloudposse/atmos/pkg/config"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// Seams for testability — overridden in tests.
var (
	setupComponentAuthForCLI = e.SetupComponentAuthForCLI
	processStacks            = e.ProcessStacks
	detectRuntime            = ctr.DetectRuntimeWithPreferenceAndRecovery
	initCliConfig            = cfg.InitCliConfig
	describeStacks           = e.ExecuteDescribeStacks
)

// defaultStopTimeout is the grace period for stop/restart/down operations.
const defaultStopTimeout = 10 * time.Second

// resolved holds the merged, runtime-ready configuration for a container
// component instance.
type resolved struct {
	atmosConfig schema.AtmosConfiguration
	spec        ContainerSpec
	env         map[string]string // container application env (incl. resolved secrets)
	envList     []string          // same env as a slice, forwarded to the runtime CLI
	runtimePref string            // docker | podman | "" (auto-detect)
	autoStart   bool
	stack       string
	component   string
	dryRun      bool
}

// prepare resolves the component section (templates, YAML functions, secrets,
// optional auth) and projects it onto a runtime-ready ContainerSpec.
func prepare(info *schema.ConfigAndStacksInfo) (*resolved, error) {
	defer perf.Track(nil, "container.prepare")()

	info.ComponentType = cfg.ContainerComponentType
	atmosConfig, err := initCliConfig(*info, true)
	if err != nil {
		return nil, err
	}

	var authManager auth.AuthManager
	if info.Identity != "" {
		authManager, err = setupComponentAuthForCLI(&atmosConfig, info)
		if err != nil {
			return nil, err
		}
	}

	processedInfo, err := processStacks(&atmosConfig, *info, true, true, true, nil, authManager)
	if err != nil {
		return nil, err
	}
	*info = processedInfo

	// Abstract components are blueprints, not deployable instances.
	if isAbstractSection(info.ComponentSection) {
		return nil, fmt.Errorf("%w: component %q is abstract and cannot be operated directly", errUtils.ErrComponentExecutionFailed, info.ComponentFromArg)
	}

	spec, err := FromComponentSection(info.ComponentSection)
	if err != nil {
		return nil, err
	}

	// Hard-error on invalid composition membership (closed contract).
	if err := composition.ValidateMembership(info.ComponentFromArg, spec.Composition, atmosConfig.Compositions); err != nil {
		return nil, err
	}

	// Surface invalid restart/healthcheck settings up front as friendly errors.
	if err := spec.ValidateRun(); err != nil {
		return nil, err
	}

	env := envListToMap(info.ComponentEnvList)
	return &resolved{
		atmosConfig: atmosConfig,
		spec:        spec,
		env:         env,
		envList:     mapToEnvList(env),
		runtimePref: strings.TrimSpace(atmosConfig.Container.Runtime.Provider),
		autoStart:   atmosConfig.Container.Runtime.AutoStart,
		stack:       info.Stack,
		component:   info.ComponentFromArg,
		dryRun:      info.DryRun,
	}, nil
}

// runtime detects the container runtime and forwards the resolved environment
// (so registry auth and app credentials reach the docker/podman subprocess).
func (r *resolved) runtime(ctx context.Context) (ctr.Runtime, error) {
	runtime, err := detectRuntime(ctx, r.runtimePref, r.autoStart)
	if err != nil {
		return nil, err
	}
	if setter, ok := runtime.(ctr.EnvSetter); ok {
		setter.SetEnv(r.envList)
	}
	return runtime, nil
}

// ExecuteBuild builds the component image from the build configuration.
func ExecuteBuild(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteBuild")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	buildConfig := r.spec.ToBuildConfig()
	if buildConfig == nil {
		return fmt.Errorf("%w: component %q has no build configuration", errUtils.ErrComponentConfigInvalid, r.component)
	}
	if r.dryRun {
		ui.Infof("[dry-run] build %s (context: %s)", strings.Join(buildConfig.Tags, ", "), buildConfig.Context)
		return nil
	}
	runtime, err := r.runtime(ctx)
	if err != nil {
		return err
	}
	if err := spinner.ExecWithSpinner(
		fmt.Sprintf("Building %s", r.component),
		fmt.Sprintf("%s built", r.component),
		func() error { return runtime.Build(ctx, buildConfig) },
	); err != nil {
		return fmt.Errorf("%w: build %q: %w", errUtils.ErrComponentExecutionFailed, r.component, err)
	}
	return nil
}

// ExecutePush pushes the component image to its registry. It sends every
// configured build tag (so registry-qualified tags in `build.tags` push to
// multiple registries in one operation), falling back to the single top-level
// image when no build tags are set. Pushes happen in order and fail fast.
func ExecutePush(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecutePush")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	refs := r.spec.PushRefs()
	if len(refs) == 0 {
		return fmt.Errorf("%w: component %q has no image or build.tags to push", errUtils.ErrComponentConfigInvalid, r.component)
	}
	if r.dryRun {
		for _, ref := range refs {
			ui.Infof("[dry-run] push %s", ref)
		}
		return nil
	}
	runtime, err := r.runtime(ctx)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := spinner.ExecWithSpinner(
			fmt.Sprintf("Pushing %s", ref),
			fmt.Sprintf("%s pushed", ref),
			func() error { _, pushErr := runtime.Push(ctx, ref); return pushErr },
		); err != nil {
			return fmt.Errorf("%w: push %q: %w", errUtils.ErrComponentExecutionFailed, ref, err)
		}
	}
	return nil
}

// ExecutePull pulls the component image.
func ExecutePull(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecutePull")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	image, err := r.requireImage()
	if err != nil {
		return err
	}
	if r.dryRun {
		ui.Infof("[dry-run] pull %s", image)
		return nil
	}
	runtime, err := r.runtime(ctx)
	if err != nil {
		return err
	}
	if err := spinner.ExecWithSpinner(
		fmt.Sprintf("Pulling %s", image),
		fmt.Sprintf("%s pulled", image),
		func() error { return runtime.Pull(ctx, image) },
	); err != nil {
		return fmt.Errorf("%w: pull %q: %w", errUtils.ErrComponentExecutionFailed, image, err)
	}
	return nil
}

// ExecuteRun runs the component as a one-shot foreground container (run).
func ExecuteRun(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteRun")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	image, err := r.requireImage()
	if err != nil {
		return err
	}
	if r.spec.Run == nil || strings.TrimSpace(r.spec.Run.Command) == "" {
		return fmt.Errorf("%w: component %q has no run.command for run", errUtils.ErrComponentConfigInvalid, r.component)
	}
	ports := r.spec.Ports()
	if r.dryRun {
		ui.Infof("[dry-run] run %s: %s", image, r.spec.Run.Command)
		return nil
	}
	runtime, err := r.runtime(ctx)
	if err != nil {
		return err
	}
	if err := r.ensureImage(ctx, runtime, image); err != nil {
		return err
	}
	_, err = ctr.RunEphemeralContainer(ctx, runtime, &ctr.EphemeralConfig{
		Name:    ctr.RuntimeName(r.stack, cfg.ContainerComponentType, r.component),
		Image:   image,
		Command: []string{"/bin/sh", "-lc", r.spec.Run.Command},
		Mounts:  r.spec.Mounts(),
		Ports:   ports,
		Env:     r.envList,
		User:    r.runUser(),
		Labels:  ctr.InstanceLabels(r.stack, cfg.ContainerComponentType, r.component),
		Host:    r.spec.HostRuntime(),
	})
	if err != nil {
		return fmt.Errorf("%w: run %q: %w", errUtils.ErrComponentExecutionFailed, r.component, err)
	}
	return nil
}

// ExecuteUp creates or starts the long-lived named container (run).
func ExecuteUp(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteUp")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	image, err := r.requireImage()
	if err != nil {
		return err
	}
	ports := r.spec.Ports()
	command, err := r.spec.CommandArgs()
	if err != nil {
		return err
	}
	namedConfig := &ctr.NamedConfig{
		Stack:         r.stack,
		ComponentType: cfg.ContainerComponentType,
		Component:     r.component,
		Image:         image,
		Command:       command,
		Ports:         ports,
		Mounts:        r.spec.Mounts(),
		Env:           r.env,
		User:          r.runUser(),
		Host:          r.spec.HostRuntime(),
		Restart:       r.spec.RestartPolicy(),
		HealthCheck:   r.spec.HealthCheck(),
	}
	if r.dryRun {
		ui.Infof("[dry-run] up %s as %s", image, ctr.RuntimeName(r.stack, cfg.ContainerComponentType, r.component))
		return nil
	}
	runtime, err := r.runtime(ctx)
	if err != nil {
		return err
	}
	if err := r.ensureImage(ctx, runtime, image); err != nil {
		return err
	}
	if err := spinner.ExecWithSpinnerDynamic(
		fmt.Sprintf("Starting %s", r.component),
		func() (string, error) {
			named, upErr := ctr.UpWithRuntime(ctx, runtime, namedConfig)
			if upErr != nil {
				return "", upErr
			}
			if named.AlreadyRunning {
				return fmt.Sprintf("%s is already running (%s)", r.component, named.Name()), nil
			}
			return fmt.Sprintf("%s is up (%s)", r.component, named.Name()), nil
		},
	); err != nil {
		return fmt.Errorf("%w: up %q: %w", errUtils.ErrComponentExecutionFailed, r.component, err)
	}
	return nil
}

// ExecuteDown stops and removes the long-lived container.
func ExecuteDown(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteDown")()

	r, runtime, err := r2(ctx, info)
	if err != nil {
		return err
	}
	return spinner.ExecWithSpinner(
		fmt.Sprintf("Stopping %s", r.component),
		fmt.Sprintf("%s is down", r.component),
		func() error { return ctr.Down(ctx, runtime, r.stack, cfg.ContainerComponentType, r.component) },
	)
}

// ExecuteLogs streams logs from a single component's container (no follow, all
// lines). The richer multi-component / follow behavior lives in
// ExecuteLogsWithOptions (logs.go); this thin entry point is kept for callers and
// tests that stream one component with defaults.
func ExecuteLogs(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteLogs")()

	return ExecuteLogsWithOptions(ctx, info, logsOptions{tail: defaultLogsTail})
}

// ExecuteExec runs a command in the component's container. Args after `--` form
// the command; defaults to a shell.
func ExecuteExec(ctx context.Context, info *schema.ConfigAndStacksInfo, command []string) error {
	defer perf.Track(nil, "container.ExecuteExec")()

	d, err := discover(ctx, info)
	if err != nil {
		return err
	}
	if len(command) == 0 {
		command = []string{"/bin/sh"}
	}
	if err := d.runtime.Exec(ctx, containerRef(d.in), command, &ctr.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	}); err != nil {
		return fmt.Errorf("%w: exec %q: %w", errUtils.ErrComponentExecutionFailed, d.r.component, err)
	}
	return nil
}

// ExecuteAttach attaches local stdin/stdout/stderr to the component container's
// main process (PID 1), mirroring `docker/podman attach`. Unlike `exec`, it does
// not start a new shell — it connects to the existing process. Detach with the
// runtime's detach keys (Ctrl-P Ctrl-Q), which leaves the container running.
func ExecuteAttach(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteAttach")()

	d, err := discover(ctx, info)
	if err != nil {
		return err
	}
	if !ctr.IsContainerRunning(d.in.Status) {
		return fmt.Errorf("%w: %q is not running (try `atmos container up`)", errUtils.ErrComponentExecutionFailed, d.r.component)
	}
	if err := d.runtime.Attach(ctx, containerRef(d.in), &ctr.AttachOptions{}); err != nil {
		return fmt.Errorf("%w: attach %q: %w", errUtils.ErrComponentExecutionFailed, d.r.component, err)
	}
	return nil
}

// ExecuteRestart stops then starts the component's container.
func ExecuteRestart(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteRestart")()

	d, err := discover(ctx, info)
	if err != nil {
		return err
	}
	id := containerRef(d.in)
	return spinner.ExecWithSpinner(
		fmt.Sprintf("Restarting %s", d.r.component),
		fmt.Sprintf("%s restarted", d.r.component),
		func() error {
			if ctr.IsContainerRunning(d.in.Status) {
				if err := d.runtime.Stop(ctx, id, defaultStopTimeout); err != nil {
					return fmt.Errorf("%w: stop %q: %w", errUtils.ErrComponentExecutionFailed, d.r.component, err)
				}
			}
			if err := d.runtime.Start(ctx, id); err != nil {
				return fmt.Errorf("%w: start %q: %w", errUtils.ErrComponentExecutionFailed, d.r.component, err)
			}
			return nil
		},
	)
}

// ExecuteStart starts the component's existing (stopped) container in place,
// discovered by label. It is the inverse of stop: unlike `up`, it never creates
// or recreates the container — if none exists, `up` is the way to create it.
func ExecuteStart(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteStart")()

	d, err := discover(ctx, info)
	if err != nil {
		return err
	}
	if ctr.IsContainerRunning(d.in.Status) {
		ui.Infof("%s is already running", d.r.component)
		return nil
	}
	if err := spinner.ExecWithSpinner(
		fmt.Sprintf("Starting %s", d.r.component),
		fmt.Sprintf("%s started", d.r.component),
		func() error { return d.runtime.Start(ctx, containerRef(d.in)) },
	); err != nil {
		return fmt.Errorf("%w: start %q: %w", errUtils.ErrComponentExecutionFailed, d.r.component, err)
	}
	return nil
}

// ExecuteStop stops the component's container without removing it.
func ExecuteStop(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteStop")()

	d, err := discover(ctx, info)
	if err != nil {
		return err
	}
	if err := spinner.ExecWithSpinner(
		fmt.Sprintf("Stopping %s", d.r.component),
		fmt.Sprintf("%s stopped", d.r.component),
		func() error { return d.runtime.Stop(ctx, containerRef(d.in), defaultStopTimeout) },
	); err != nil {
		return fmt.Errorf("%w: stop %q: %w", errUtils.ErrComponentExecutionFailed, d.r.component, err)
	}
	return nil
}

// ExecuteRm removes the component's container.
func ExecuteRm(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "container.ExecuteRm")()

	d, err := discover(ctx, info)
	if err != nil {
		return err
	}
	if err := spinner.ExecWithSpinner(
		fmt.Sprintf("Removing %s", d.r.component),
		fmt.Sprintf("%s removed", d.r.component),
		func() error { return d.runtime.Remove(ctx, containerRef(d.in), true) },
	); err != nil {
		return fmt.Errorf("%w: remove %q: %w", errUtils.ErrComponentExecutionFailed, d.r.component, err)
	}
	return nil
}

// r2 prepares the instance and detects a runtime (no instance lookup).
func r2(ctx context.Context, info *schema.ConfigAndStacksInfo) (*resolved, ctr.Runtime, error) {
	r, err := prepare(info)
	if err != nil {
		return nil, nil, err
	}
	runtime, err := r.runtime(ctx)
	if err != nil {
		return nil, nil, err
	}
	return r, runtime, nil
}

// discovered bundles a prepared instance, its runtime, and the container found
// by label discovery.
type discovered struct {
	r       *resolved
	runtime ctr.Runtime
	in      *ctr.Info
}

// discover prepares the instance, detects a runtime, and finds the container by
// label, erroring if it is not found.
func discover(ctx context.Context, info *schema.ConfigAndStacksInfo) (*discovered, error) {
	r, runtime, err := r2(ctx, info)
	if err != nil {
		return nil, err
	}
	in, found, err := ctr.FindInstance(ctx, runtime, r.stack, cfg.ContainerComponentType, r.component)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w for %q (try `atmos container up`)", errUtils.ErrNoRunningContainer, r.component)
	}
	return &discovered{r: r, runtime: runtime, in: in}, nil
}

// ensureImage builds the component image before start when the component has a
// `build` and the image is not already present locally. This lets `up` and
// `run` work without a separate `build` step (build-before-start). Components
// without a build (image-only) are pulled on demand by the runtime instead.
func (r *resolved) ensureImage(ctx context.Context, runtime ctr.Runtime, image string) error {
	if r.spec.Build == nil {
		return nil
	}
	// Already built/available locally — nothing to do. Only a missing image is
	// recoverable by building; transport/auth/daemon failures must surface as-is
	// so the real cause is not masked behind a misleading build.
	if _, err := runtime.ImageInspect(ctx, image); err == nil {
		return nil
	} else if !ctr.IsImageMissingError(err) {
		return fmt.Errorf("%w: inspect image %q for %q: %w", errUtils.ErrComponentExecutionFailed, image, r.component, err)
	}
	buildConfig := r.spec.ToBuildConfig()
	if buildConfig == nil {
		return nil
	}
	if err := spinner.ExecWithSpinner(
		fmt.Sprintf("Building %s for %s", image, r.component),
		fmt.Sprintf("Built %s for %s", image, r.component),
		func() error { return runtime.Build(ctx, buildConfig) },
	); err != nil {
		return fmt.Errorf("%w: build %q: %w", errUtils.ErrComponentExecutionFailed, r.component, err)
	}
	return nil
}

// requireImage returns the configured image or an error.
func (r *resolved) requireImage() (string, error) {
	if strings.TrimSpace(r.spec.Image) == "" {
		return "", fmt.Errorf("%w: component %q has no image configured", errUtils.ErrComponentConfigInvalid, r.component)
	}
	return r.spec.Image, nil
}

// runUser returns the configured run user, if any.
func (r *resolved) runUser() string {
	if r.spec.Run == nil {
		return ""
	}
	return r.spec.Run.User
}

// containerRef returns the most specific identifier for a discovered container.
func containerRef(in *ctr.Info) string {
	if in == nil {
		return ""
	}
	if in.ID != "" {
		return in.ID
	}
	return in.Name
}

// isAbstractSection reports whether a resolved component section is marked
// `metadata.type: abstract`.
func isAbstractSection(section map[string]any) bool {
	metadata, ok := section["metadata"].(map[string]any)
	if !ok {
		return false
	}
	t, _ := metadata["type"].(string)
	return t == "abstract"
}

// envListToMap converts the resolved component env list (the component `env:`
// section, with secrets) into a map. Container application env comes from this
// first-class section, not from `run`.
func envListToMap(componentEnvList []string) map[string]string {
	env := map[string]string{}
	for _, kv := range componentEnvList {
		if key, value, ok := strings.Cut(kv, "="); ok {
			env[key] = value
		}
	}
	return env
}

// mapToEnvList converts an env map to a sorted-free slice of "K=V" entries.
func mapToEnvList(env map[string]string) []string {
	list := make([]string, 0, len(env))
	for k, v := range env {
		list = append(list, fmt.Sprintf("%s=%s", k, v))
	}
	return list
}
