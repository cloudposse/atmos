package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPushRateLimitCooldown(t *testing.T) {
	for _, mode := range []string{"429", "secondary403", "fallback", "reset", "budget", "cancel", "auth"} {
		t.Run(mode, func(t *testing.T) {
			runner := newFakeRunner()
			limited := atmosgit.RunResult{HTTP: &atmosgit.HTTPResponse{Status: 429, RetryAfter: new(5 * time.Second)}}
			config := &schema.RetryConfig{MaxAttempts: new(3), InitialDelay: new(time.Second), MaxDelay: new(time.Second), MaxElapsedTime: new(5 * time.Minute)}
			wantDelay := 5 * time.Second
			switch mode {
			case "secondary403":
				limited.HTTP.Status = 403
			case "fallback":
				limited = atmosgit.RunResult{StderrTail: "remote: rate limit exceeded"}
				wantDelay = time.Minute
			case "reset":
				limited.HTTP = &atmosgit.HTTPResponse{Status: 403, Remaining: new(0), ResetAt: time.Now().Add(10 * time.Second)}
				wantDelay = 10 * time.Second
			case "budget":
				config.MaxElapsedTime = new(3 * time.Second)
			case "auth":
				limited = atmosgit.RunResult{HTTP: &atmosgit.HTTPResponse{Status: 403}, StderrTail: "fatal: The requested URL returned error: 403"}
			}
			runner.on("push", limited, exitErr(1))
			runner.on("push", atmosgit.RunResult{}, nil)
			clock := &advancingPushClock{}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancel" {
				clock.onWait = cancel
			}
			err := New(WithRunner(runner)).pushWithRetry(ctx, &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Branch: "main"}}, config, retry.WithClock(clock))
			switch mode {
			case "budget":
				require.ErrorIs(t, err, errUtils.ErrGitRateLimited)
				var budget retry.MaxElapsedTimeError
				require.ErrorAs(t, err, &budget)
				require.Empty(t, clock.waits)
				require.Len(t, runner.calls, 1)
			case "cancel":
				require.ErrorIs(t, err, context.Canceled)
				require.Len(t, runner.calls, 1)
			case "auth":
				require.ErrorIs(t, err, errUtils.ErrGitAuthFailed)
				require.Empty(t, clock.waits)
				require.Len(t, runner.calls, 1)
			default:
				require.NoError(t, err)
				require.Len(t, clock.waits, 1)
				require.InDelta(t, wantDelay.Seconds(), clock.waits[0].Seconds(), 1)
				require.Equal(t, []string{"push origin main", "push origin main"}, runner.joinedCalls())
			}
		})
	}
}

func TestPushThrottledRebaseIsRetriedBeforePush(t *testing.T) {
	runner := newFakeRunner()
	runner.on("push", atmosgit.RunResult{StderrTail: "non-fast-forward"}, exitErr(1))
	runner.on("push", atmosgit.RunResult{}, nil)
	runner.on("pull", atmosgit.RunResult{HTTP: &atmosgit.HTTPResponse{Status: 429, RetryAfter: new(2 * time.Second)}}, exitErr(1))
	runner.on("pull", atmosgit.RunResult{}, nil)
	clock := &advancingPushClock{}
	err := New(WithRunner(runner)).pushWithRetry(t.Context(), &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Branch: "main"}}, pushTestPolicy(4), retry.WithClock(clock))
	require.NoError(t, err)
	require.Equal(t, []string{"push origin main", "pull --rebase origin main", "pull --rebase origin main", "push origin main"}, runner.joinedCalls())
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, clock.waits)
}

func TestRateLimitBackoffAndPositiveJitter(t *testing.T) {
	result := &atmosgit.RunResult{HTTP: &atmosgit.HTTPResponse{Status: 429, RetryAfter: new(time.Minute)}}
	config := &schema.RetryConfig{RandomJitter: new(0.5), MaxDelay: new(time.Second)}
	for range 100 {
		delay := rateLimitDelay(result, 0, config)
		require.GreaterOrEqual(t, delay, time.Minute)
		require.LessOrEqual(t, delay, 90*time.Second)
	}
	require.Equal(t, 2*time.Minute, rateLimitDelay(&atmosgit.RunResult{}, 1, &schema.RetryConfig{}))
	require.True(t, errors.Is(&rateLimitError{delay: time.Minute}, errUtils.ErrGitRateLimited))
}
