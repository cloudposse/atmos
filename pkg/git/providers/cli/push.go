package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	pushRetryInitialDelay = 250 * time.Millisecond
	pushRetryMaxDelay     = 5 * time.Second
	pushRetryBudget       = 30 * time.Second
	pushRetryJitter       = 0.5
)

// Push retries rejected refs and throttled transports with the shared retry policy.
// Only rejected refs trigger a rebase. Server cooldowns take precedence over the
// configured delay cap, while attempts and the elapsed-time budget remain bounded.
// Atmos never adds force-push options.
func (p *Provider) Push(ctx context.Context, opts *atmosgit.PushOptions) error {
	defer perf.Track(nil, "cli.Provider.Push")()

	config, err := pushRetryConfig(opts)
	if err != nil {
		return err
	}
	// Also bound running Git commands when the shared policy specifies a budget.
	if opts.Retry != nil && config.MaxElapsedTime != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *config.MaxElapsedTime)
		defer cancel()
	}
	return p.pushWithRetry(ctx, opts, config)
}

// pushRetryConfig preserves legacy defaults unless an explicit shared policy is supplied.
func pushRetryConfig(opts *atmosgit.PushOptions) (*schema.RetryConfig, error) {
	if opts.Retry != nil {
		if err := retry.Validate(opts.Retry); err != nil {
			return nil, fmt.Errorf("%w: push retry: %w", errUtils.ErrInvalidConfig, err)
		}
		return opts.Retry, nil
	}
	if opts.Retries < 0 || opts.Retries == math.MaxInt {
		return nil, fmt.Errorf("%w: push retries must be nonnegative and less than MaxInt", errUtils.ErrInvalidConfig)
	}
	attempts := opts.Retries + 1
	initial, maximum, budget := pushRetryInitialDelay, pushRetryMaxDelay, pushRetryBudget
	jitter := pushRetryJitter
	return &schema.RetryConfig{
		MaxAttempts: &attempts, BackoffStrategy: schema.BackoffExponential,
		InitialDelay: &initial, MaxDelay: &maximum, RandomJitter: &jitter, MaxElapsedTime: &budget,
	}, nil
}

// pushWithRetry keeps Git recovery state local to this invocation. The generic
// executor owns scheduling; only a rejected push can authorize another rebase.
func (p *Provider) pushWithRetry(ctx context.Context, opts *atmosgit.PushOptions,
	config *schema.RetryConfig, executorOptions ...retry.Option,
) error {
	patterns, err := retry.CompileConditions(config.Conditions)
	if err != nil {
		return err
	}
	start := time.Now()
	state := &pushRetryState{provider: p, opts: opts, config: config, patterns: patterns}
	err = retry.New(*config, executorOptions...).ExecuteWithPredicate(ctx, func() error {
		return state.attempt(ctx)
	}, func(err error) bool { return state.rejected || errors.Is(err, errUtils.ErrGitRateLimited) })
	if err == nil {
		return nil
	}
	if state.rejected && !errors.Is(err, errUtils.ErrGitPushRejected) {
		err = errors.Join(errUtils.ErrGitPushRejected, err)
	}
	// Do not include remote URLs, ref names or subprocess output in diagnostics.
	return fmt.Errorf("git push after %d attempt(s), %d retr%s, elapsed %s: %w",
		state.attempts, max(0, state.attempts-1), pluralRetry(max(0, state.attempts-1)), time.Since(start).Round(time.Millisecond), err)
}

// pushOnce performs a single push attempt.
func (p *Provider) pushOnce(ctx context.Context, opts *atmosgit.PushOptions) (atmosgit.RunResult, error) {
	args := []string{"push", remoteOrDefault(opts.Remote)}
	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}
	args = append(args, opts.ExtraArgs...)
	return p.runPushTransport(ctx, opts.Workdir, opts.Env, args...)
}

// pluralRetry pluralizes "retry" for error messages.
func pluralRetry(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// retryablePushResult keeps configured conditions within safe rejected-push recovery.
func retryablePushResult(result atmosgit.RunResult, patterns []*regexp.Regexp) bool {
	if isAuthenticationFailure(result.StderrTail) || !isRejectedPush(result) {
		return false
	}
	return len(patterns) == 0 || retry.MatchesAny(patterns, result.StderrTail)
}
