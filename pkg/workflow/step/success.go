package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// SuccessHandler displays a success message with green checkmark.
type SuccessHandler struct {
	BaseHandler
}

func init() {
	Register(&SuccessHandler{
		BaseHandler: NewBaseHandler("success", CategoryUI, false),
	})
}

// Validate checks that the step has required fields.
func (h *SuccessHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "content", step.Content)
}

// Execute displays the success message.
func (h *SuccessHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	if err := ui.Success(content); err != nil {
		return nil, err
	}

	return NewStepResult(content), nil
}
