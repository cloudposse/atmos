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

// StartBackground launches a background step through the runner, registers its
// handle, and applies the implicit readiness gate: when the step declares a health
// check, WaitReady blocks until it is healthy before the workflow continues; with
// no health check WaitReady is a no-op (started == ready).
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
	return handle.WaitReady(ctx)
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
		}
		reg.Remove(name)
	}
	return stderrors.Join(errs...)
}
