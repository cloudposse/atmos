package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MatrixHandler registers the matrix workflow control step type.
type MatrixHandler struct {
	BaseHandler
}

func init() {
	Register(&MatrixHandler{
		BaseHandler: NewBaseHandler(schema.TaskTypeMatrix, CategoryCommand, false),
	})
}

// Validate checks that the matrix control step is structurally valid and that no
// child step is interactive (interactive steps cannot run concurrently).
func (h *MatrixHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.MatrixHandler.Validate")()

	if err := schema.ValidateWorkflowSteps([]schema.WorkflowStep{*step}); err != nil {
		return err
	}
	return validateControlChildrenNonInteractive(step)
}

// Execute expands the matrix and fans its children out via the registered
// ControlRunner (see control_seam.go), so matrix steps work through the registry
// from custom commands and lifecycle hooks, not just `atmos workflow`.
func (h *MatrixHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.MatrixHandler.Execute")()

	return runControlStep(ctx, step, vars)
}
