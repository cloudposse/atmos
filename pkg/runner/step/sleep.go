package step

import (
	"context"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
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
	defer perf.Track(nil, "step.SleepHandler.Validate")()

	// Timeout field is used for sleep duration.
	// If not specified, defaults to 1 second.
	return nil
}

// Execute pauses execution for the specified duration.
func (h *SleepHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.SleepHandler.Execute")()

	// Parse duration from timeout field.
	duration := time.Second // Default to 1 second.
	if step.Timeout != "" {
		resolved, err := vars.Resolve(step.Timeout)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", "timeout").
				Err()
		}
		parsed, err := time.ParseDuration(resolved)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrInvalidDuration).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("value", resolved).
				Err()
		}
		duration = parsed
	}

	// Create a timer that respects context cancellation.
	select {
	case <-time.After(duration):
		return NewStepResult(duration.String()), nil
	case <-ctx.Done():
		return nil, errUtils.Build(errUtils.ErrUserAborted).
			WithCause(ctx.Err()).
			WithContext("step", step.Name).
			WithExplanation("Sleep interrupted by context cancellation").
			Err()
	}
}
