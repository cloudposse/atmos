package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
)

// AlertHandler plays a terminal bell sound.
type AlertHandler struct {
	BaseHandler
}

func init() {
	Register(&AlertHandler{
		BaseHandler: NewBaseHandler("alert", CategoryUI, false),
	})
}

// Validate checks that the step has valid fields.
func (h *AlertHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.AlertHandler.Validate")()

	// No required fields - content is optional.
	return nil
}

// Execute plays the terminal bell and optionally displays a message.
func (h *AlertHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.AlertHandler.Execute")()

	// Play bell using terminal's Alert method (respects settings.terminal.alerts).
	term := terminal.New()
	term.Alert()

	// Optionally display a message.
	if step.Content != "" {
		content, err := h.ResolveContent(ctx, step, vars)
		if err != nil {
			return nil, err
		}
		if err := ui.Writeln(content); err != nil {
			return nil, err
		}
		return NewStepResult(content), nil
	}

	return NewStepResult(""), nil
}
