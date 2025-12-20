package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// InfoHandler displays an informational message with cyan icon.
type InfoHandler struct {
	BaseHandler
}

func init() {
	Register(&InfoHandler{
		BaseHandler: NewBaseHandler("info", CategoryUI, false),
	})
}

// Validate checks that the step has required fields.
func (h *InfoHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "content", step.Content)
}

// Execute displays the info message.
func (h *InfoHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	if err := ui.Info(content); err != nil {
		return nil, err
	}

	return NewStepResult(content), nil
}
