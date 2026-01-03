package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// MarkdownHandler renders and displays markdown content.
type MarkdownHandler struct {
	BaseHandler
}

func init() {
	Register(&MarkdownHandler{
		BaseHandler: NewBaseHandler("markdown", CategoryUI, false),
	})
}

// Validate checks that the step has required fields.
func (h *MarkdownHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.MarkdownHandler.Validate")()

	return h.ValidateRequired(step, "content", step.Content)
}

// Execute renders and displays the markdown content.
func (h *MarkdownHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.MarkdownHandler.Execute")()

	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Use MarkdownMessage to render to stderr (UI channel).
	if err := ui.MarkdownMessage(content); err != nil {
		return nil, err
	}

	return NewStepResult(content), nil
}
