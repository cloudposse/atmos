package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
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

// Push pushes the current branch, retrying on non-fast-forward rejection —
// the most common GitOps publishing failure (a concurrent publisher pushed
// first). Each retry rebases local commits onto the updated remote ref and
// pushes again after jittered exponential backoff, bounded by opts.Retries
// and a 30-second deadline. Atmos never adds force-push options.
func (p *Provider) Push(ctx context.Context, opts *atmosgit.PushOptions) error {
	defer perf.Track(nil, "cli.Provider.Push")()

	if opts.Retries < 0 || opts.Retries == math.MaxInt {
		return fmt.Errorf("%w: push retries must be nonnegative and less than MaxInt", errUtils.ErrInvalidConfig)
	}
	attempts := opts.Retries + 1
	initial, maximum, budget := pushRetryInitialDelay, pushRetryMaxDelay, pushRetryBudget
	jitter := pushRetryJitter
	config := schema.RetryConfig{
		MaxAttempts: &attempts, BackoffStrategy: schema.BackoffExponential,
		InitialDelay: &initial, MaxDelay: &maximum, RandomJitter: &jitter, MaxElapsedTime: &budget,
	}
	// The deadline also bounds a running Git command and the final retry wait.
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return p.pushWithRetry(ctx, opts, &config)
}

// pushWithRetry keeps Git recovery state local to this invocation. The generic
// executor owns scheduling; only a rejected push can authorize another rebase.
func (p *Provider) pushWithRetry(ctx context.Context, opts *atmosgit.PushOptions,
	config *schema.RetryConfig, executorOptions ...retry.Option,
) error {
	start := time.Now()
	attempts := 0
	rejected := false
	err := retry.New(*config, executorOptions...).ExecuteWithPredicate(ctx, func() error {
		if rejected {
			rejected = false
			if err := p.rebaseOntoRemote(ctx, opts.RepoContext); err != nil {
				return err
			}
		}
		attempts++
		result, err := p.pushOnce(ctx, opts)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Authentication failures must win even if the output also mentions rejection.
		if !matchesAny(result.StderrTail, authPatterns) && isRejectedPush(result) {
			rejected = true
			return errUtils.ErrGitPushRejected
		}
		return classify(err, result, "push")
	}, func(error) bool { return rejected })
	if err == nil {
		return nil
	}
	if rejected && !errors.Is(err, errUtils.ErrGitPushRejected) {
		err = errors.Join(errUtils.ErrGitPushRejected, err)
	}
	// Do not include remote URLs, ref names or subprocess output in diagnostics.
	return fmt.Errorf("git push after %d attempt(s), %d retr%s, elapsed %s: %w",
		attempts, max(0, attempts-1), pluralRetry(max(0, attempts-1)), time.Since(start).Round(time.Millisecond), err)
}

// pushOnce performs a single push attempt.
func (p *Provider) pushOnce(ctx context.Context, opts *atmosgit.PushOptions) (atmosgit.RunResult, error) {
	args := []string{"push", remoteOrDefault(opts.Remote)}
	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}
	args = append(args, opts.ExtraArgs...)
	return p.run(ctx, opts.Workdir, opts.Env, args...)
}

// rebaseOntoRemote replays local commits onto the updated remote ref so the
// next push attempt is fast-forward for the remote.
func (p *Provider) rebaseOntoRemote(ctx context.Context, rc atmosgit.RepoContext) error {
	args := []string{"pull", "--rebase", remoteOrDefault(rc.Remote)}
	if rc.Branch != "" {
		args = append(args, rc.Branch)
	}

	result, err := p.run(ctx, rc.Workdir, rc.Env, args...)
	if err != nil {
		return classify(err, result, "pull --rebase")
	}
	return nil
}

// pluralRetry pluralizes "retry" for error messages.
func pluralRetry(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
