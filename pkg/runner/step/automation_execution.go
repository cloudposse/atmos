package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

// automationDeadline bounds a direct call with the step's `timeout:`. Sleep uses timeout as its
// duration, and HTTP owns its per-request timeout and retry policy inside the handler, so neither
// gets a deadline over the whole call. An overrun is reported through StepDeadline.Wrap as
// ErrStepTimeout naming the step, its type, and the configured duration.
func (l *AutomationLibrary) automationDeadline(ctx context.Context, step *schema.WorkflowStep) (*StepDeadline, error) {
	handler, _ := Get(step.Type)
	if handler.GetName() == "sleep" || handler.GetName() == "http" {
		unbounded := *step
		unbounded.Timeout = ""
		return StartStepDeadline(ctx, &unbounded, l.vars)
	}
	return StartStepDeadline(ctx, step, l.vars)
}
