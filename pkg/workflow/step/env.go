package step

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
)

// EnvHandler sets environment variables for subsequent steps.
type EnvHandler struct {
	BaseHandler
}

func init() {
	Register(&EnvHandler{
		BaseHandler: NewBaseHandler("env", CategoryUI, false),
	})
}

// Validate checks that the step has required fields.
func (h *EnvHandler) Validate(step *schema.WorkflowStep) error {
	if len(step.Vars) == 0 {
		return h.ValidateRequired(step, "vars", "")
	}
	return nil
}

// Execute sets environment variables in the workflow context.
func (h *EnvHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	for key, value := range step.Vars {
		// Resolve templates in the value.
		resolved, err := vars.Resolve(value)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve env var %s: %w", key, err)
		}
		vars.SetEnv(key, resolved)
	}

	return NewStepResult(""), nil
}
