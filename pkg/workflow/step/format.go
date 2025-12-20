package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/schema"
)

// FormatHandler formats and displays text using Go templates.
type FormatHandler struct {
	BaseHandler
}

func init() {
	Register(&FormatHandler{
		BaseHandler: NewBaseHandler("format", CategoryOutput, false),
	})
}

// Validate checks that the step has required fields.
func (h *FormatHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "content", step.Content)
}

// Execute formats and displays the content.
func (h *FormatHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Write to stdout (data channel).
	if err := data.Writeln(content); err != nil {
		return nil, err
	}

	return NewStepResult(content), nil
}
