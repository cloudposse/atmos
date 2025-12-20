package step

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
)

// JoinHandler joins multiple strings together.
type JoinHandler struct {
	BaseHandler
}

func init() {
	Register(&JoinHandler{
		BaseHandler: NewBaseHandler("join", CategoryOutput, false),
	})
}

// Validate checks that the step has required fields.
func (h *JoinHandler) Validate(step *schema.WorkflowStep) error {
	// Join can use either content (single template) or options (array of strings).
	if step.Content == "" && len(step.Options) == 0 {
		return fmt.Errorf("step '%s' (join): either content or options is required", step.Name)
	}
	return nil
}

// Execute joins strings and returns the result.
func (h *JoinHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	var parts []string

	if len(step.Options) > 0 {
		// Join options array.
		for i, opt := range step.Options {
			resolved, err := vars.Resolve(opt)
			if err != nil {
				return nil, fmt.Errorf("step '%s': failed to resolve option %d: %w", step.Name, i, err)
			}
			parts = append(parts, resolved)
		}
	} else if step.Content != "" {
		// Content is a single template - resolve and return as-is.
		content, err := h.ResolveContent(ctx, step, vars)
		if err != nil {
			return nil, err
		}
		return NewStepResult(content), nil
	}

	// Join with newlines by default.
	result := strings.Join(parts, "\n")
	return NewStepResult(result), nil
}
