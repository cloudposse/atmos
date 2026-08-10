package step

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// homeDirProvider expands a leading `~` in step.WorkingDirectory, matching
// the same tilde-expansion --chdir already gets (see cmd/root.go's
// processChdirFlag). Stateless, so a single shared instance is safe.
var homeDirProvider = filesystem.NewOSHomeDirProvider()

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

// ResolveInWorkingDirectory resolves a Go-template field, then anchors a
// relative result to step.WorkingDirectory (itself template-resolved),
// returning an absolute path. An empty raw value short-circuits to "" (no
// resolve, no join) like the other Resolve* helpers. A value that resolves
// to "" or to an already-absolute path passes through unchanged — it is
// never re-anchored to WorkingDirectory.
func (h BaseHandler) ResolveInWorkingDirectory(step *schema.WorkflowStep, vars *Variables, value, field string) (string, error) {
	defer perf.Track(nil, "step.BaseHandler.ResolveInWorkingDirectory")()

	if value == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(value)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", field).
			Err()
	}
	if resolved == "" || filepath.IsAbs(resolved) {
		return resolved, nil
	}

	workDir, err := h.resolveWorkingDirectory(step, vars)
	if err != nil {
		return "", err
	}
	return filepath.Join(workDir, resolved), nil
}

// isDotPrefixedWorkingDirectory reports whether an already-resolved,
// non-empty, non-absolute working_directory value is "Dot" per
// docs/prd/base-path-resolution-semantics.md's classify() convention: an
// explicit "HERE" anchor. A false result means "Bare" — a value with no
// dot-anchor, which pkg/hooks-invoked steps may instead anchor to the
// step's component working directory rather than the process cwd (see
// resolveWorkingDirectory below).
//
// This mirrors the PRD's Dot-detection precisely, including the bare ".."
// case (not just "../"-prefixed) and the Windows ".\" / "..\" variants. It
// intentionally does NOT delegate to pkg/component.IsExplicitComponentPath,
// which is missing that bare ".." case and is scoped to component-path
// arguments, not working_directory values. Also, this package must not
// import pkg/hooks (which imports pkg/runner/step) to avoid a cycle, so this
// small, dedicated mirror is preferable to introducing a shared dependency
// for six string comparisons.
func isDotPrefixedWorkingDirectory(value string) bool {
	return value == "." || value == ".." ||
		strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") ||
		strings.HasPrefix(value, `.\`) || strings.HasPrefix(value, `..\`)
}

// resolveWorkingDirectory resolves step.WorkingDirectory (itself a possible
// template) to an absolute base directory.
//
// A leading `~` (e.g. `~/scratch`) is expanded to the user's home directory
// before the Dot/Bare/absolute checks below run, so it is never joined as a
// literal path segment. A resulting `~`-expanded value is always absolute
// and therefore short-circuits the classification just like any other
// absolute value.
//
// An explicit, non-empty, non-absolute value is classified per
// docs/prd/base-path-resolution-semantics.md's Dot/Bare convention (see
// isDotPrefixedWorkingDirectory):
//
//   - Dot-prefixed ("." , "..", "./x", "../x", ".\x", "..\x") always anchors
//     to the process cwd — unchanged historical behavior.
//   - Bare relative anchors to vars.componentWorkingDir when it is set to an
//     absolute path (populated only by pkg/hooks, via ComponentPath(ctx));
//     otherwise it falls back to the process cwd, exactly like Dot — so
//     workflows and custom commands, which never set componentWorkingDir,
//     are unaffected.
//
// An empty step.WorkingDirectory falls back to the process cwd, which is
// already absolute and therefore never reaches the Dot/Bare branch below —
// preserving the historical filepath.Abs-against-cwd behavior for steps
// that never set working_directory. (Hooks instead default an unset
// step.WorkingDirectory to ComponentPath(ctx) *before* this function ever
// runs, via setDefaultStepWorkingDirectory in pkg/hooks/step_engine.go, so
// this empty-value fallback is effectively workflow/custom-command-only in
// practice.)
func (h BaseHandler) resolveWorkingDirectory(step *schema.WorkflowStep, vars *Variables) (string, error) {
	defer perf.Track(nil, "step.BaseHandler.resolveWorkingDirectory")()

	workDir, err := resolveWorkingDirectoryValue(step, vars, step.WorkingDirectory)
	if err != nil {
		return "", err
	}
	if workDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", errUtils.Build(errUtils.ErrPathResolution).
				WithCause(err).
				WithContext("step", step.Name).
				Err()
		}
		workDir = cwd
	}
	if !filepath.IsAbs(workDir) {
		if anchor := vars.componentWorkingDir; anchor != "" && filepath.IsAbs(anchor) &&
			!isDotPrefixedWorkingDirectory(workDir) {
			return filepath.Join(anchor, workDir), nil
		}
		abs, err := filepath.Abs(workDir)
		if err != nil {
			return "", errUtils.Build(errUtils.ErrPathResolution).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", "working_directory").
				Err()
		}
		workDir = abs
	}
	return workDir, nil
}

// resolveWorkingDirectoryValue template-resolves and tilde-expands a
// step.WorkingDirectory value, returning "" unchanged when raw is empty.
// Split out of resolveWorkingDirectory to keep its own cyclomatic complexity
// under the repo's lint threshold.
func resolveWorkingDirectoryValue(step *schema.WorkflowStep, vars *Variables, raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(raw)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "working_directory").
			Err()
	}
	if resolved == "" {
		return "", nil
	}
	expanded, err := homeDirProvider.Expand(resolved)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrPathResolution).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "working_directory").
			Err()
	}
	return expanded, nil
}
