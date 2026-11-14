package cli

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"regexp"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

var httpTooManyRequests = regexp.MustCompile(`(?i)(?:returned error:\s*|http(?:/[0-9.]+)?\s+)(?:429)\b`)

// rateLimitError carries only a safe cooldown, never raw subprocess diagnostics.
type rateLimitError struct{ delay time.Duration }

func (e *rateLimitError) Error() string {
	return fmt.Sprintf("%s; retry after at least %s (increase retry.max_elapsed_time if needed)", errUtils.ErrGitRateLimited, e.delay.Round(time.Millisecond))
}
func (e *rateLimitError) Unwrap() error             { return errUtils.ErrGitRateLimited }
func (e *rateLimitError) RetryAfter() time.Duration { return e.delay }

func isRateLimited(result *atmosgit.RunResult) bool {
	if result.HTTP != nil {
		h := result.HTTP
		if h.Status == http.StatusTooManyRequests {
			return true
		}
		if h.Status == http.StatusUnauthorized {
			return false
		}
		if h.Status == http.StatusForbidden && (h.RetryAfter != nil || h.Remaining != nil && *h.Remaining == 0) {
			return true
		}
	}
	if matchesAny(result.StderrTail, authPatterns) {
		return false
	}
	return httpTooManyRequests.MatchString(result.StderrTail) || matchesAny(result.StderrTail, []string{"rate limit exceeded", "secondary rate limit", "too many requests", "rate limit is exceeded"})
}

// rateLimitDelay treats transport hints as a minimum. Positive-only jitter cannot
// shorten a server cooldown. Without a hint, wait at least a minute, doubling on
// repeated throttling. The caller's attempt/deadline limits still apply.
func rateLimitDelay(result *atmosgit.RunResult, failures int, config *schema.RetryConfig) time.Duration {
	const maxBackoffExponent = 10
	fallback := time.Minute * time.Duration(1<<min(failures, maxBackoffExponent))
	delay := fallback
	if hinted := rateLimitHint(result.HTTP); hinted >= 0 {
		delay = hinted
		if failures > 0 {
			delay = max(delay, fallback)
		}
	}
	if config.RandomJitter != nil && *config.RandomJitter > 0 {
		// Bound arithmetic even if an untrusted Retry-After saturated the duration.
		extra := float64(delay) * rand.Float64() * *config.RandomJitter //nolint:gosec // Scheduling jitter requires no cryptographic randomness.
		if extra < float64(time.Duration(1<<63-1)-delay) {
			delay += time.Duration(extra)
		}
	}
	return delay
}

func rateLimitHint(h *atmosgit.HTTPResponse) time.Duration {
	if h == nil {
		return -1
	}
	hinted := time.Duration(-1)
	if h.RetryAfter != nil {
		hinted = *h.RetryAfter
	}
	if h.Remaining != nil && *h.Remaining == 0 && !h.ResetAt.IsZero() {
		hinted = max(hinted, max(time.Duration(0), time.Until(h.ResetAt)))
	}
	return hinted
}

// runPushTransport captures only sanitized HTTP response metadata. The Runner
// filters curl tracing in memory; it is never written to a trace file or log.
func (p *Provider) runPushTransport(ctx context.Context, dir string, env []string, args ...string) (atmosgit.RunResult, error) {
	return p.runner.Run(ctx, gitCommand, args, atmosgit.RunOptions{Dir: dir, Env: env, Stderr: p.stderr, CaptureHTTPMetadata: true})
}

// pushRetryState distinguishes a rejected ref from a throttled transport. A
// throttled pull must finish its rebase before any subsequent push is attempted.
type pushRetryState struct {
	provider  *Provider
	opts      *atmosgit.PushOptions
	config    *schema.RetryConfig
	patterns  []*regexp.Regexp
	attempts  int
	rejected  bool
	throttles int
}

func (s *pushRetryState) attempt(ctx context.Context) error {
	if s.rejected {
		if err := s.rebase(ctx); err != nil {
			return err
		}
	}
	s.attempts++
	result, err := s.provider.pushOnce(ctx, s.opts)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if isRateLimited(&result) {
		return s.throttle(&result)
	}
	if retryablePushResult(result, s.patterns) {
		s.rejected = true
		return errUtils.ErrGitPushRejected
	}
	return classify(err, result, "push")
}

// rebase preserves rejected state only while a throttled fetch needs recovery.
func (s *pushRetryState) rebase(ctx context.Context) error {
	args := []string{"pull", "--rebase", remoteOrDefault(s.opts.Remote)}
	if s.opts.Branch != "" {
		args = append(args, s.opts.Branch)
	}
	result, err := s.provider.runPushTransport(ctx, s.opts.Workdir, s.opts.Env, args...)
	if err == nil {
		s.rejected = false
		return nil
	}
	if ctx.Err() != nil {
		s.rejected = false
		return ctx.Err()
	}
	if isRateLimited(&result) {
		return s.throttle(&result)
	}
	s.rejected = false
	return classify(err, result, "pull --rebase")
}

func (s *pushRetryState) throttle(result *atmosgit.RunResult) error {
	delay := rateLimitDelay(result, s.throttles, s.config)
	s.throttles++
	return &rateLimitError{delay: delay}
}
