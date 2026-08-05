package step

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ToastHandler displays a styled message with icon (success, info, warning, error).
type ToastHandler struct {
	BaseHandler
}

func init() {
	Register(&ToastHandler{
		BaseHandler: NewBaseHandler("toast", CategoryUI, false),
	})
}

// Validate checks that the step has required fields.
func (h *ToastHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ToastHandler.Validate")()

	return h.ValidateRequired(step, "content", step.Content)
}

// Execute displays the message with the appropriate style based on level.
func (h *ToastHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ToastHandler.Execute")()

	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Display based on level (default to info).
	switch strings.ToLower(step.Level) {
	case "success":
		ui.Success(content)
	case "warning", "warn":
		ui.Warning(content)
	case "error":
		ui.Error(content)
	default:
		// Default to info for "", "info", or any other value.
		ui.Info(content)
	}

	return NewStepResult(content), nil
}
