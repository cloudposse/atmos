package container

// This file holds the lifecycle operations that act on an existing container: down, logs, exec,
// attach, restart, start, stop, and rm. Creation and image operations live in executor.go.

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

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
