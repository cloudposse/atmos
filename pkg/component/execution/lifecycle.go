package execution

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Runner executes one hook and reports its failure and captured output.
// The output feeds error hook matching and is never printed again.
type Runner interface {
	RunHook(ctx context.Context, kind HookKind, hook *schema.ComponentHook) (output string, err error)
}

// failure is a failed phase with the output it produced.
type failure struct {
	err    error
	output string
}

// Run executes the command lifecycle: before hooks, the action, after hooks, then error hooks.
//
// A failing before hook stops the action. After hooks run after a failure only when they set
// run_on_error. Error hooks run when any earlier phase failed and an on_errors expression matches
// that phase's error or output. Cancellation stops every remaining hook. The action's error is
// returned unchanged when nothing else failed.
func Run(ctx context.Context, config *schema.ComponentExecution, command string, runner Runner, action func() (string, error)) error {
	defer perf.Track(nil, "execution.Run")()

	if config == nil || (len(config.BeforeHooks)+len(config.AfterHooks)+len(config.ErrorHooks) == 0) {
		_, err := action()
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	failures := runHooks(ctx, runner, KindBefore, hooksFor(config.BeforeHooks, command), nil)
	if len(failures) == 0 {
		if output, err := action(); err != nil {
			failures = append(failures, failure{err: err, output: output})
		}
	}
	if err := ctx.Err(); err != nil {
		return joinFailures(failures, err)
	}

	failures = append(failures, runHooks(ctx, runner, KindAfter, hooksFor(config.AfterHooks, command), failures)...)
	if err := ctx.Err(); err != nil {
		return joinFailures(failures, err)
	}
	failures = append(failures, runErrorHooks(ctx, runner, hooksFor(config.ErrorHooks, command), failures)...)
	return joinFailures(failures, ctx.Err())
}

// runHooks mirrors Terragrunt: every hook sees the failures before it, including earlier hooks
// of the same kind, and runs only when there are none or it sets run_on_error.
func runHooks(ctx context.Context, runner Runner, kind HookKind, hooks []*schema.ComponentHook, prior []failure) []failure {
	var failures []failure
	for _, hook := range hooks {
		if len(prior)+len(failures) > 0 && !hook.RunOnError {
			continue
		}
		if err := ctx.Err(); err != nil {
			// Record the cancellation so the action does not start after an interrupted hook.
			failures = append(failures, failure{err: err})
			break
		}
		if output, err := runner.RunHook(ctx, kind, hook); err != nil {
			failures = append(failures, failure{err: hookError(kind, hook, err), output: output})
		}
	}
	return failures
}

func runErrorHooks(ctx context.Context, runner Runner, hooks []*schema.ComponentHook, prior []failure) []failure {
	if len(hooks) == 0 || len(prior) == 0 {
		return nil
	}
	var text strings.Builder
	for _, item := range prior {
		text.WriteString(item.err.Error() + "\n" + item.output + "\n")
	}
	var failures []failure
	for _, hook := range hooks {
		if ctx.Err() != nil {
			break
		}
		matched, err := matchesAny(hook.OnErrors, text.String())
		if err != nil {
			failures = append(failures, failure{err: fmt.Errorf("matching on_errors of error hook %q: %w", hook.Name, err)})
			continue
		}
		if !matched {
			continue
		}
		if output, err := runner.RunHook(ctx, KindError, hook); err != nil {
			failures = append(failures, failure{err: hookError(KindError, hook, err), output: output})
		}
	}
	return failures
}

func matchesAny(patterns []string, text string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := regexp.MatchString(pattern, text)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func hookError(kind HookKind, hook *schema.ComponentHook, err error) error {
	return fmt.Errorf("%w: %s %q: %w", errUtils.ErrComponentHookFailed, kind, hook.Name, err)
}

// joinFailures keeps a lone error intact so callers can still inspect its concrete type.
func joinFailures(failures []failure, extra error) error {
	errs := make([]error, 0, len(failures)+1)
	for _, item := range failures {
		errs = append(errs, item.err)
	}
	if extra != nil && !containsError(errs, extra) {
		errs = append(errs, extra)
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
}

func containsError(errs []error, target error) bool {
	for _, err := range errs {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
