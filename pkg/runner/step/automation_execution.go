package step

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// automationContext bounds a direct call. Sleep uses timeout as its duration,
// and HTTP owns its per-request timeout and retry policy inside the handler.
func (l *AutomationLibrary) automationContext(ctx context.Context, step *schema.WorkflowStep) (context.Context, context.CancelFunc, error) {
	handler, _ := Get(step.Type)
	if step.Timeout == "" || handler.GetName() == "sleep" || handler.GetName() == "http" {
		return ctx, func() {}, nil
	}
	value, err := l.vars.Resolve(step.Timeout)
	if err != nil {
		return nil, nil, err
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return nil, nil, fmt.Errorf("%w: timeout must be a positive duration, got %q", errUtils.ErrAutomation, value)
	}
	child, cancel := context.WithTimeout(ctx, duration)
	return child, cancel, nil
}
