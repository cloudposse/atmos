package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// WarnHandler displays a warning message with yellow icon.
type WarnHandler struct {
	BaseHandler
}

func init() {
	Register(&WarnHandler{
		BaseHandler: NewBaseHandler("warn", CategoryUI, false),
	})
}

// Validate checks that the step has required fields.
func (h *WarnHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "content", step.Content)
}

// Execute displays the warning message.
func (h *WarnHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	if err := ui.Warning(content); err != nil {
		return nil, err
	}

	return NewStepResult(content), nil
}
