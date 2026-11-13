package execution

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Runner executes one hook and reports its failure and captured output.
// The output feeds error hook matching and is never printed again. A nil output means
// nothing was captured.
type Runner interface {
	RunHook(ctx context.Context, kind HookKind, hook *schema.ComponentHook) (output Output, err error)
}

// Output is captured phase output. Error hook matching reads it as a stream, so output of
// any size is never loaded into memory as a whole.
type Output interface {
	// Open returns a new reader positioned at the start of the output. A reader that
	// implements io.Closer is closed after use. An error reports a failed capture.
	Open() (io.Reader, error)
}

// TextOutput is output that is already in memory.
type TextOutput string

// Open returns a reader over the text.
//
//nolint:lintroller // Trivial accessor; no I/O.
func (t TextOutput) Open() (io.Reader, error) { return strings.NewReader(string(t)), nil }

// failure is a failed phase with the output it produced.
type failure struct {
	err    error
	output Output
}

// Run executes the command lifecycle: before hooks, the action, after hooks, then error hooks.
//
// A failing before hook stops the action. After hooks run after a failure only when they set
// run_on_error. Error hooks run when any earlier phase failed and an on_errors expression matches
// that phase's error or output. Cancellation stops every remaining hook. The action's error is
// returned unchanged when nothing else failed.
func Run(ctx context.Context, config *schema.ComponentExecution, command string, runner Runner, action func() (Output, error)) error {
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
	var failures []failure
	for _, hook := range hooks {
		if ctx.Err() != nil {
			break
		}
		matched, err := matchesAny(hook.OnErrors, prior)
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

// matchesAny matches each pattern against every prior failure's error and output, in order,
// as one text. The text is streamed, so memory use does not grow with the output size.
func matchesAny(patterns []string, prior []failure) (bool, error) {
	for _, pattern := range patterns {
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		matched, err := matchFailures(expression, prior)
		if err != nil || matched {
			return matched, err
		}
	}
	return false, nil
}

func matchFailures(expression *regexp.Regexp, prior []failure) (bool, error) {
	readers := make([]io.Reader, 0, 3*len(prior))
	var closers []io.Closer
	defer func() {
		for _, closer := range closers {
			_ = closer.Close()
		}
	}()
	for _, item := range prior {
		readers = append(readers, strings.NewReader(item.err.Error()+"\n"))
		if item.output != nil {
			reader, err := item.output.Open()
			if err != nil {
				return false, err
			}
			if closer, ok := reader.(io.Closer); ok {
				closers = append(closers, closer)
			}
			readers = append(readers, reader)
		}
		readers = append(readers, strings.NewReader("\n"))
	}
	stream := &errorReader{reader: bufio.NewReader(io.MultiReader(readers...))}
	matched := expression.MatchReader(stream)
	if stream.err != nil {
		return false, stream.err
	}
	return matched, nil
}

// errorReader keeps the first read error, which regexp.MatchReader treats as end of input.
type errorReader struct {
	reader *bufio.Reader
	err    error
}

// ReadRune reads the next rune and remembers the first error other than end of input.
//
//nolint:lintroller // Called for every rune matched; tracking would dominate matching.
func (r *errorReader) ReadRune() (rune, int, error) {
	value, size, err := r.reader.ReadRune()
	if err != nil && !errors.Is(err, io.EOF) && r.err == nil {
		r.err = err
	}
	return value, size, err
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
