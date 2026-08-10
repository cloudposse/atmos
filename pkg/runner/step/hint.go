package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// HintHandler displays a muted hint message with the Atmos hint UI style.
type HintHandler struct {
	BaseHandler
}

func init() {
	Register(&HintHandler{
		BaseHandler: NewBaseHandler("hint", CategoryUI, false),
	})
}

// Validate checks that the step has required fields.
func (h *HintHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.HintHandler.Validate")()

	return h.ValidateRequired(step, "content", step.Content)
}

// Execute displays the hint message.
func (h *HintHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.HintHandler.Execute")()

	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	ui.Hint(content)

	return NewStepResult(content), nil
}
