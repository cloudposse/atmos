package step

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// BaseHandler provides common functionality for step handlers.
type BaseHandler struct {
	name        string
	category    StepCategory
	requiresTTY bool
	aliases     []string
}

// NewBaseHandler creates a new BaseHandler. Optional aliases are alternate type
// names that resolve to the same handler (e.g. "webhook" as an alias for "http").
func NewBaseHandler(name string, category StepCategory, requiresTTY bool, aliases ...string) BaseHandler {
	defer perf.Track(nil, "step.NewBaseHandler")()

	return BaseHandler{
		name:        name,
		category:    category,
		requiresTTY: requiresTTY,
		aliases:     aliases,
	}
}

// GetName returns the step type name.
func (h BaseHandler) GetName() string {
	defer perf.Track(nil, "step.BaseHandler.GetName")()

	return h.name
}

// GetAliases returns the alternate type names that resolve to this handler.
func (h BaseHandler) GetAliases() []string {
	defer perf.Track(nil, "step.BaseHandler.GetAliases")()

	return h.aliases
}

// GetCategory returns the step category.
func (h BaseHandler) GetCategory() StepCategory {
	defer perf.Track(nil, "step.BaseHandler.GetCategory")()

	return h.category
}

// RequiresTTY returns whether this handler requires an interactive terminal.
func (h BaseHandler) RequiresTTY() bool {
	defer perf.Track(nil, "step.BaseHandler.RequiresTTY")()

	return h.requiresTTY
}

// hasInteractiveTTY reports whether an interactive terminal is available.
// Interactive steps require both stdin and stdout to be TTYs. It honors
// --force-tty / ATMOS_FORCE_TTY via terminal.New().
func (h BaseHandler) hasInteractiveTTY() bool {
	defer perf.Track(nil, "step.BaseHandler.hasInteractiveTTY")()

	term := terminal.New()
	// Interactive steps require both stdin and stdout to be TTYs.
	return term.IsTTY(terminal.Stdin) && term.IsTTY(terminal.Stdout)
}

// ttyRequiredError builds the error returned when an interactive step cannot run
// because there is no TTY and no default value is configured.
func (h BaseHandler) ttyRequiredError(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.BaseHandler.ttyRequiredError")()

	return errUtils.Build(errUtils.ErrStepTTYRequired).
		WithContext("step", step.Name).
		WithContext("type", step.Type).
		WithExplanation(fmt.Sprintf("The step type '%s' requires a TTY for user input", step.Type)).
		WithHint("Use --dry-run to preview workflow without interactive steps").
		WithHint("Set default values in workflow configuration").
		WithHint("Use environment variables instead of interactive prompts in CI").
		Err()
}

// resolveInteractive decides how an interactive step should obtain its value
// based on TTY availability and whether a `default` is configured:
//
//   - (true, nil): a TTY is available; the caller renders the interactive prompt.
//   - (false, nil): there is no TTY but the step has a `default`; the caller
//     uses the default value non-interactively (e.g. in CI).
//   - (false, err): there is no TTY and no `default` is set; err is
//     ErrStepTTYRequired (the historical behavior).
//
// Non-interactive handlers (requiresTTY == false) always return (true, nil).
func (h BaseHandler) resolveInteractive(step *schema.WorkflowStep) (bool, error) {
	defer perf.Track(nil, "step.BaseHandler.resolveInteractive")()

	if !h.requiresTTY || h.hasInteractiveTTY() {
		return true, nil
	}
	if step.Default != "" {
		return false, nil
	}
	return false, h.ttyRequiredError(step)
}

// ResolveDefault resolves Go templates in the step's default value, returning an
// empty string when no default is configured. Shared by all interactive
// handlers so default resolution is defined in exactly one place.
func (h BaseHandler) ResolveDefault(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (string, error) {
	defer perf.Track(nil, "step.BaseHandler.ResolveDefault")()

	if step.Default == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(step.Default)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "default").
			Err()
	}
	return resolved, nil
}

// ValidateRequired checks that a required field is not empty.
func (h BaseHandler) ValidateRequired(step *schema.WorkflowStep, field, value string) error {
	defer perf.Track(nil, "step.BaseHandler.ValidateRequired")()

	if value == "" {
		return errUtils.Build(errUtils.ErrStepFieldRequired).
			WithContext("step", step.Name).
			WithContext("type", step.Type).
			WithContext("field", field).
			Err()
	}
	return nil
}

// ResolveContent resolves Go templates in the content field.
func (h BaseHandler) ResolveContent(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (string, error) {
	defer perf.Track(nil, "step.BaseHandler.ResolveContent")()

	if step.Content == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(step.Content)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "content").
			Err()
	}
	return resolved, nil
}

// ResolvePrompt resolves Go templates in the prompt field.
func (h BaseHandler) ResolvePrompt(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (string, error) {
	defer perf.Track(nil, "step.BaseHandler.ResolvePrompt")()

	if step.Prompt == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(step.Prompt)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "prompt").
			Err()
	}
	return resolved, nil
}

// ResolveCommand resolves Go templates in the command field.
func (h BaseHandler) ResolveCommand(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (string, error) {
	defer perf.Track(nil, "step.BaseHandler.ResolveCommand")()

	if step.Command == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(step.Command)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "command").
			Err()
	}
	return resolved, nil
}
