package step

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The wait, wait-all, and cancel action steps coordinate background steps. Like
// parallel/matrix, their real logic lives in the workflow executor (it owns the
// run-scoped background registry), so the handlers here only register the type
// names and validate; Execute returns a control-context error if reached directly.

// WaitHandler registers the `wait` action step (block until named background steps ready).
type WaitHandler struct{ BaseHandler }

// WaitAllHandler registers the `wait-all` action step (block until all background steps ready).
type WaitAllHandler struct{ BaseHandler }

// CancelHandler registers the `cancel` action step (tear down named background steps).
type CancelHandler struct{ BaseHandler }

func init() {
	Register(&WaitHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWait, CategoryCommand, false)})
	Register(&WaitAllHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWaitAll, CategoryCommand, false)})
	Register(&CancelHandler{BaseHandler: NewBaseHandler(schema.TaskTypeCancel, CategoryCommand, false)})
}

// Validate requires `for:` to name at least one target.
func (h *WaitHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.WaitHandler.Validate")()

	return validateForTargets(step, schema.TaskTypeWait)
}

// Validate requires `for:` to name at least one target.
func (h *CancelHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.CancelHandler.Validate")()

	return validateForTargets(step, schema.TaskTypeCancel)
}

// Validate is a no-op: wait-all takes no targets.
func (h *WaitAllHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.WaitAllHandler.Validate")()

	return nil
}

// validateForTargets ensures a wait/cancel action step names its target(s).
func validateForTargets(step *schema.WorkflowStep, stepType string) error {
	if len(step.For) == 0 {
		return fmt.Errorf("%w: %s step %q requires `for:` naming the background step(s) to %s",
			schema.ErrWorkflowControlStepInvalid, stepType, step.Name, stepType)
	}
	return nil
}

func (h *WaitHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.WaitHandler.Execute")()

	return nil, errBackgroundActionContext(schema.TaskTypeWait)
}

func (h *WaitAllHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.WaitAllHandler.Execute")()

	return nil, errBackgroundActionContext(schema.TaskTypeWaitAll)
}

func (h *CancelHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.CancelHandler.Execute")()

	return nil, errBackgroundActionContext(schema.TaskTypeCancel)
}

func errBackgroundActionContext(stepType string) error {
	return fmt.Errorf("%w: %s steps require workflow executor context", schema.ErrWorkflowControlStepInvalid, stepType)
}
