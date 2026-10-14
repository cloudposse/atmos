package hooks

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// knownGitHookNames lists the hook names Git itself invokes. A configured name outside this
// list is still installable (Git ignores the shim) but is almost always a typo.
var knownGitHookNames = map[string]struct{}{
	"applypatch-msg": {}, "pre-applypatch": {}, "post-applypatch": {}, "pre-commit": {},
	"pre-merge-commit": {}, "prepare-commit-msg": {}, "commit-msg": {}, "post-commit": {},
	"pre-rebase": {}, "post-checkout": {}, "post-merge": {}, "pre-push": {},
	"pre-receive": {}, "update": {}, "proc-receive": {}, "post-receive": {},
	"post-update": {}, "reference-transaction": {}, "push-to-checkout": {}, "pre-auto-gc": {},
	"post-rewrite": {}, "sendemail-validate": {}, "fsmonitor-watchman": {}, "p4-changelist": {},
	"p4-prepare-changelist": {}, "p4-post-changelist": {}, "p4-pre-submit": {}, "post-index-change": {},
}

// IsKnownGitHookName reports whether Git itself invokes a hook with this name.
func IsKnownGitHookName(name string) bool {
	defer perf.Track(nil, "hooks.IsKnownGitHookName")()

	_, ok := knownGitHookNames[name]
	return ok
}

// validateHookConfig checks that a hook sets exactly one of command or a non-empty steps list.
func validateHookConfig(name string, entry schema.GitHookEntry) error {
	if (strings.TrimSpace(entry.Command) != "") == (len(entry.Steps) > 0) {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHint("Configure exactly one of command or a non-empty steps list for a Git hook.").
			WithContext("hook", name).
			Err()
	}
	return nil
}

// ValidateHook checks a hook's configuration without running it: the command-vs-steps
// exclusivity rule and, for step hooks, the same validation RunSteps performs before executing.
// Every error carries the hook name.
func ValidateHook(name string, entry schema.GitHookEntry) error {
	defer perf.Track(nil, "hooks.ValidateHook")()

	if err := validateHookConfig(name, entry); err != nil {
		return wrapHookError(name, err)
	}
	if len(entry.Steps) == 0 {
		return nil
	}
	if err := step.NewAutomationLibrary(step.NewVariables(), nil).ValidateSteps(entry.Steps); err != nil {
		return wrapHookError(name, err)
	}
	return nil
}

// invalidHooksError reports that one or more configured hooks failed validation. It unwraps to
// every per-hook error so errors.Is and errors.As match each of them.
type invalidHooksError struct {
	errs []error
}

func (e *invalidHooksError) Error() string {
	return fmt.Sprintf("%d configured Git hook(s) are invalid", len(e.errs))
}

func (e *invalidHooksError) Unwrap() []error {
	return e.errs
}

// validateHooks validates every named hook and joins all failures so one install attempt
// reports every broken hook rather than only the first.
func validateHooks(names []string, cfg *schema.GitConfig) error {
	if cfg == nil {
		return nil
	}
	var errs []error
	var lines []string
	for _, name := range names {
		entry, ok := cfg.Hooks[name]
		if !ok {
			continue
		}
		if err := ValidateHook(name, entry); err != nil {
			errs = append(errs, err)
			lines = append(lines, "- "+err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	// Keep every cause reachable through errors.Is, and list them as a Markdown bullet list so
	// each broken hook renders on its own line.
	return errUtils.Build(&invalidHooksError{errs: errs}).
		WithExplanationf("These configured Git hooks are invalid:\n\n%s", strings.Join(lines, "\n")).
		WithHint("Fix the listed hooks in git.hooks, or name only valid hooks: atmos git hooks install <hook>.").
		WithExitCode(2).
		Err()
}

// wrapHookError prefixes err with the hook name so every failure identifies its hook.
func wrapHookError(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("git hook %q: %w", name, err)
}
