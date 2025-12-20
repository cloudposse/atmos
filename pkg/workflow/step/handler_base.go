package step

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// BaseHandler provides common functionality for step handlers.
type BaseHandler struct {
	name        string
	category    StepCategory
	requiresTTY bool
}

// NewBaseHandler creates a new BaseHandler.
func NewBaseHandler(name string, category StepCategory, requiresTTY bool) BaseHandler {
	return BaseHandler{
		name:        name,
		category:    category,
		requiresTTY: requiresTTY,
	}
}

// GetName returns the step type name.
func (h BaseHandler) GetName() string {
	return h.name
}

// GetCategory returns the step category.
func (h BaseHandler) GetCategory() StepCategory {
	return h.category
}

// RequiresTTY returns whether this handler requires an interactive terminal.
func (h BaseHandler) RequiresTTY() bool {
	return h.requiresTTY
}

// CheckTTY verifies TTY availability for interactive steps.
// Returns an error if TTY is required but not available.
func (h BaseHandler) CheckTTY(step *schema.WorkflowStep) error {
	if !h.requiresTTY {
		return nil
	}

	term := terminal.New()
	if !term.IsTTY(terminal.Stdout) {
		return fmt.Errorf(
			"step '%s' requires interactive terminal\n\n"+
				"The step type '%s' requires a TTY for user input.\n"+
				"This workflow cannot run in non-interactive mode (CI, piped output).\n\n"+
				"Hints:\n"+
				"  - Use --dry-run to preview workflow without interactive steps\n"+
				"  - Set default values in workflow configuration\n"+
				"  - Use environment variables instead of interactive prompts in CI",
			step.Name, step.Type,
		)
	}
	return nil
}

// ValidateRequired checks that a required field is not empty.
func (h BaseHandler) ValidateRequired(step *schema.WorkflowStep, field, value string) error {
	if value == "" {
		return fmt.Errorf("step '%s' (%s): %s is required", step.Name, step.Type, field)
	}
	return nil
}

// ResolveContent resolves Go templates in the content field.
func (h BaseHandler) ResolveContent(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (string, error) {
	if step.Content == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(step.Content)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve content template: %w", step.Name, err)
	}
	return resolved, nil
}

// ResolvePrompt resolves Go templates in the prompt field.
func (h BaseHandler) ResolvePrompt(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (string, error) {
	if step.Prompt == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(step.Prompt)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve prompt template: %w", step.Name, err)
	}
	return resolved, nil
}

// ResolveCommand resolves Go templates in the command field.
func (h BaseHandler) ResolveCommand(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (string, error) {
	if step.Command == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(step.Command)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve command template: %w", step.Name, err)
	}
	return resolved, nil
}
