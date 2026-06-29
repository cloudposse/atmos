package step

import (
	"context"
	"fmt"

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

// Validate checks that the matrix control step is structurally valid.
func (h *MatrixHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.MatrixHandler.Validate")()

	return schema.ValidateWorkflowSteps([]schema.WorkflowStep{*step})
}

// Execute is intentionally not implemented here. The workflow executor handles
// matrix control steps because it owns command execution, auth, retry, and output.
func (h *MatrixHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.MatrixHandler.Execute")()

	return nil, fmt.Errorf("%w: matrix steps require workflow executor context", schema.ErrWorkflowControlStepInvalid)
}
