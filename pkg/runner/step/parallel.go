package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ParallelHandler registers the parallel workflow control step type.
type ParallelHandler struct {
	BaseHandler
}

func init() {
	Register(&ParallelHandler{
		BaseHandler: NewBaseHandler(schema.TaskTypeParallel, CategoryCommand, false),
	})
}

// Validate checks that the parallel control step is structurally valid and that
// no child step is interactive (interactive steps cannot run concurrently).
func (h *ParallelHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ParallelHandler.Validate")()

	if err := schema.ValidateWorkflowSteps([]schema.WorkflowStep{*step}); err != nil {
		return err
	}
	return validateControlChildrenNonInteractive(step)
}

// Execute fans the parallel step out to its children via the registered
// ControlRunner (see control_seam.go). The heavy scheduler/graph and (for
// `atmos workflow`) auth/output machinery lives in pkg/workflow and is reached
// through the seam, so parallel steps work through the registry — from custom
// commands and lifecycle hooks, not just `atmos workflow`.
func (h *ParallelHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ParallelHandler.Execute")()

	return runControlStep(ctx, step, vars)
}
