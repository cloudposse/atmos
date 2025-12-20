package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ErrorHandler displays an error message with red icon.
type ErrorHandler struct {
	BaseHandler
}

func init() {
	Register(&ErrorHandler{
		BaseHandler: NewBaseHandler("error", CategoryUI, false),
	})
}

// Validate checks that the step has required fields.
func (h *ErrorHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "content", step.Content)
}

// Execute displays the error message.
func (h *ErrorHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	if err := ui.Error(content); err != nil {
		return nil, err
	}

	return NewStepResult(content), nil
}
