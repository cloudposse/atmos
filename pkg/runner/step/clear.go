package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ClearHandler clears the current terminal line.
type ClearHandler struct {
	BaseHandler
}

func init() {
	Register(&ClearHandler{
		BaseHandler: NewBaseHandler("clear", CategoryUI, false),
	})
}

// Validate checks that the step has valid fields.
func (h *ClearHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ClearHandler.Validate")()

	// No required fields.
	return nil
}

// Execute clears the current terminal line.
func (h *ClearHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ClearHandler.Execute")()

	ui.ClearLine()
	return NewStepResult(""), nil
}
