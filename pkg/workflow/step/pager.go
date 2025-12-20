package step

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PagerHandler displays content in a scrollable pager.
type PagerHandler struct {
	BaseHandler
}

func init() {
	Register(&PagerHandler{
		BaseHandler: NewBaseHandler("pager", CategoryOutput, false),
	})
}

// Validate checks that the step has required fields.
func (h *PagerHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "content", step.Content)
}

// Execute displays content in a pager.
func (h *PagerHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	title := step.Title
	if title != "" {
		title, err = vars.Resolve(title)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve title: %w", step.Name, err)
		}
	}

	// Create pager with pager enabled.
	p := pager.NewWithAtmosConfig(true)
	if err := p.Run(title, content); err != nil {
		return nil, fmt.Errorf("step '%s': pager failed: %w", step.Name, err)
	}

	return NewStepResult(content), nil
}
