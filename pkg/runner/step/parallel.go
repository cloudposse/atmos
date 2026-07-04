package step

import (
	"context"
	"fmt"

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

// Validate checks that the parallel control step is structurally valid.
func (h *ParallelHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ParallelHandler.Validate")()

	return schema.ValidateWorkflowSteps([]schema.WorkflowStep{*step})
}

// Execute is intentionally not implemented here. The workflow executor handles
// parallel control steps because it owns command execution, auth, retry, and output.
func (h *ParallelHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ParallelHandler.Execute")()

	return nil, fmt.Errorf("%w: parallel steps require workflow executor context", schema.ErrWorkflowControlStepInvalid)
}
