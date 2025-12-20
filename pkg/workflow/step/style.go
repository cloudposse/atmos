package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// StyleHandler applies terminal styling to text.
type StyleHandler struct {
	BaseHandler
}

func init() {
	Register(&StyleHandler{
		BaseHandler: NewBaseHandler("style", CategoryOutput, false),
	})
}

// Validate checks that the step has required fields.
func (h *StyleHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "content", step.Content)
}

// Execute applies styling and displays the content.
func (h *StyleHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Get current styles and apply heading styling.
	styles := theme.GetCurrentStyles()
	if styles != nil {
		content = styles.Heading.Render(content)
	}

	// Write to stdout (data channel).
	if err := data.Writeln(content); err != nil {
		return nil, err
	}

	return NewStepResult(content), nil
}
