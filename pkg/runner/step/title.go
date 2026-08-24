package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// TitleHandler sets the terminal window title.
type TitleHandler struct {
	BaseHandler
}

func init() {
	Register(&TitleHandler{
		BaseHandler: NewBaseHandler("title", CategoryUI, false),
	})
}

// Validate checks that the step has valid fields.
func (h *TitleHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.TitleHandler.Validate")()

	// Content is optional - if empty, restores original title.
	return nil
}

// Execute sets or restores the terminal window title.
func (h *TitleHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.TitleHandler.Execute")()

	term := terminal.New()

	if step.Content == "" {
		// Restore original title.
		term.RestoreTitle()
		return NewStepResult(""), nil
	}

	// Resolve and set title.
	title, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	term.SetTitle(title)
	return NewStepResult(title), nil
}
