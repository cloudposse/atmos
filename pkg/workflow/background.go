package workflow

import (
	"context"
	stderrors "errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/background"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StartBackground launches a background step through the runner and registers its
// handle. Start is non-blocking: it does not wait on readiness, so consecutive
// background steps come up concurrently. The implicit readiness gate is applied by
// the workflow executor before the next foreground step (and by `wait`/`wait-all`),
// reusing the step's health check via Handle.WaitReady.
func StartBackground(
	ctx context.Context,
	reg *background.Registry,
	runner background.Runner,
	step *schema.WorkflowStep,
	env []string,
) error {
	defer perf.Track(nil, "workflow.StartBackground")()

	handle, err := runner.Start(ctx, step, env)
	if err != nil {
		return err
	}
	reg.Register(handle)
	return nil
}

// WaitBackground blocks until every named background step is ready (a service's
// health check passes). Names are validated upstream, but a missing name is
// reported rather than silently ignored.
func WaitBackground(ctx context.Context, reg *background.Registry, names []string) error {
	defer perf.Track(nil, "workflow.WaitBackground")()

	var errs []error
	for _, name := range names {
		handle, ok := reg.Get(name)
		if !ok {
			errs = append(errs, fmt.Errorf("%w: wait references unknown background step %q", schema.ErrWorkflowControlStepInvalid, name))
			continue
		}
		if err := handle.WaitReady(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return stderrors.Join(errs...)
}

// WaitAllBackground blocks until all currently-registered background steps are ready.
func WaitAllBackground(ctx context.Context, reg *background.Registry) error {
	defer perf.Track(nil, "workflow.WaitAllBackground")()

	return WaitBackground(ctx, reg, reg.Names())
}

// GatePendingBackground applies the implicit readiness gate run before each foreground
// step: it blocks until every registered background step that has not already passed its
// gate is ready, then records those names in gated so later foreground steps don't
// re-probe them. A nil/empty registry (or all-gated) is a no-op.
func GatePendingBackground(ctx context.Context, reg *background.Registry, gated map[string]bool) error {
	defer perf.Track(nil, "workflow.GatePendingBackground")()

	var pending []string
	for _, name := range reg.Names() {
		if !gated[name] {
			pending = append(pending, name)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	if err := WaitBackground(ctx, reg, pending); err != nil {
		return err
	}
	for _, name := range pending {
		gated[name] = true
	}
	return nil
}

// CancelBackground gracefully tears down the named background steps and removes
// them from the registry so the end-of-scope auto-teardown does not stop them again.
func CancelBackground(ctx context.Context, reg *background.Registry, names []string) error {
	defer perf.Track(nil, "workflow.CancelBackground")()

	var errs []error
	for _, name := range names {
		handle, ok := reg.Get(name)
		if !ok {
			errs = append(errs, fmt.Errorf("%w: cancel references unknown background step %q", schema.ErrWorkflowControlStepInvalid, name))
			continue
		}
		if err := handle.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%w: cancel background step %q: %w", errUtils.ErrContainerRuntimeOperation, name, err))
			// Keep the handle registered so the deferred StopAll at workflow exit retries teardown.
			continue
		}
		reg.Remove(name)
	}
	return stderrors.Join(errs...)
}
