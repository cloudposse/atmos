package step

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

// SleepHandler pauses execution for a specified duration.
type SleepHandler struct {
	BaseHandler
}

func init() {
	Register(&SleepHandler{
		BaseHandler: NewBaseHandler("sleep", CategoryUI, false),
	})
}

// Validate checks that the step has valid fields.
func (h *SleepHandler) Validate(step *schema.WorkflowStep) error {
	// Timeout field is used for sleep duration.
	// If not specified, defaults to 1 second.
	return nil
}

// Execute pauses execution for the specified duration.
func (h *SleepHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	// Parse duration from timeout field.
	duration := time.Second // Default to 1 second.
	if step.Timeout != "" {
		resolved, err := vars.Resolve(step.Timeout)
		if err != nil {
			return nil, err
		}
		parsed, err := time.ParseDuration(resolved)
		if err != nil {
			return nil, err
		}
		duration = parsed
	}

	// Create a timer that respects context cancellation.
	select {
	case <-time.After(duration):
		return NewStepResult(duration.String()), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
