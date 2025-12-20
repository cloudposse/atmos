package step

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
// Either content or path must be provided.
func (h *PagerHandler) Validate(step *schema.WorkflowStep) error {
	if step.Content == "" && step.Path == "" {
		return h.ValidateRequired(step, "content or path", "")
	}
	return nil
}

// Execute displays content in a pager.
func (h *PagerHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	var content string
	var err error

	// If path is provided, read from file.
	if step.Path != "" {
		content, err = h.readFile(step, vars)
		if err != nil {
			return nil, err
		}
	} else {
		// Otherwise use content field.
		content, err = h.ResolveContent(ctx, step, vars)
		if err != nil {
			return nil, err
		}
	}

	title := step.Title
	if title != "" {
		title, err = vars.Resolve(title)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve title: %w", step.Name, err)
		}
	} else if step.Path != "" {
		// Default title to filename if reading from file.
		title = filepath.Base(step.Path)
	}

	// Create pager with pager enabled.
	p := pager.NewWithAtmosConfig(true)
	if err := p.Run(title, content); err != nil {
		return nil, fmt.Errorf("step '%s': pager failed: %w", step.Name, err)
	}

	return NewStepResult(content), nil
}

// readFile reads content from a file path.
func (h *PagerHandler) readFile(step *schema.WorkflowStep, vars *Variables) (string, error) {
	path, err := vars.Resolve(step.Path)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve path: %w", step.Name, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to read file '%s': %w", step.Name, path, err)
	}

	return string(data), nil
}
