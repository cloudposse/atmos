package step

import (
	"context"
	"errors"
	"regexp"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StepOutputError carries the output a failed step attempt produced alongside its error, so a
// `retry.conditions` pattern can match the text a command printed and not only the error that
// reports its exit status. Error and Unwrap leave the wrapped error untouched, so errors.Is and
// errors.As behave exactly as they did before the output was attached.
type StepOutputError struct {
	Err    error
	Stdout string
	Stderr string
}

// Error returns the wrapped error's message unchanged.
func (e *StepOutputError) Error() string {
	defer perf.Track(nil, "step.StepOutputError.Error")()

	return e.Err.Error()
}

// Unwrap returns the wrapped error.
func (e *StepOutputError) Unwrap() error {
	defer perf.Track(nil, "step.StepOutputError.Unwrap")()

	return e.Err
}

// WithStepOutput attaches the output captured in result to err. It returns err unchanged when
// there is no error, no result, no captured output, or the error already carries its output.
func WithStepOutput(err error, result *StepResult) error {
	defer perf.Track(nil, "step.WithStepOutput")()

	if err == nil || result == nil {
		return err
	}
	var existing *StepOutputError
	if errors.As(err, &existing) {
		return err
	}
	stdout, stderr := stepResultOutput(result)
	if stdout == "" && stderr == "" {
		return err
	}
	return &StepOutputError{Err: err, Stdout: stdout, Stderr: stderr}
}

// stepResultOutput returns the stdout and stderr a handler recorded for its result. Command steps
// store them as metadata; other steps expose their value and error text.
func stepResultOutput(result *StepResult) (string, string) {
	stdout, stderr := result.Value, result.Error
	if text, ok := result.Metadata["stdout"].(string); ok {
		stdout = text
	}
	if text, ok := result.Metadata["stderr"].(string); ok {
		stderr = text
	}
	return stdout, stderr
}

// RetryPredicate builds the predicate that decides whether a failed attempt of a step is retried.
// Cancellation, a step exit, and a user abort are never retried. Without `retry.conditions` every
// other error is retried. With conditions, an error is retried only when a condition (a regular
// expression, optionally written /like this/) matches the attempt's stdout, its stderr, or the
// error text; any other failure ends the retry loop at once. An invalid pattern is an error.
func RetryPredicate(cfg *schema.RetryConfig) (func(error) bool, error) {
	defer perf.Track(nil, "step.RetryPredicate")()

	var patterns []*regexp.Regexp
	if cfg != nil && len(cfg.Conditions) > 0 {
		compiled, err := retry.CompileConditions(cfg.Conditions)
		if err != nil {
			return nil, errors.Join(errUtils.ErrInvalidConfig, err)
		}
		patterns = compiled
	}
	return func(err error) bool {
		if !retryableStepError(err) {
			return false
		}
		if len(patterns) == 0 {
			return true
		}
		return matchesRetryPatterns(patterns, err)
	}, nil
}

// retryableStepError reports whether err may be retried at all.
func retryableStepError(err error) bool {
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
		!errors.Is(err, errUtils.ErrWorkflowExit) && !errors.Is(err, errUtils.ErrUserAborted)
}

// matchesRetryPatterns reports whether any pattern matches the error text or the output attached
// to the error.
func matchesRetryPatterns(patterns []*regexp.Regexp, err error) bool {
	if retry.MatchesAny(patterns, err.Error()) {
		return true
	}
	var withOutput *StepOutputError
	if errors.As(err, &withOutput) {
		return retry.MatchesAny(patterns, withOutput.Stdout) || retry.MatchesAny(patterns, withOutput.Stderr)
	}
	return false
}

// RetryWithConditions runs fn under cfg, retrying only the failures that `retry.conditions`
// selects. A nil cfg runs fn once. Callers that apply `retry:` outside RunWithStepRetry (workflow
// and custom-command shell and atmos steps) use it so conditions mean the same thing everywhere.
func RetryWithConditions(ctx context.Context, cfg *schema.RetryConfig, fn func() error) error {
	defer perf.Track(nil, "step.RetryWithConditions")()

	if cfg == nil {
		return retry.Do(ctx, nil, fn)
	}
	shouldRetry, err := RetryPredicate(cfg)
	if err != nil {
		return err
	}
	return retry.WithPredicate(ctx, cfg, fn, shouldRetry)
}
