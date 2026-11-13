package cli

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPushExplicitRetryPolicy(t *testing.T) {
	for _, attempts := range []int{1, 2, 4} {
		runner := newFakeRunner()
		runner.on("push origin main", atmosgit.RunResult{StderrTail: "non-fast-forward"}, exitErr(1))
		err := New(WithRunner(runner)).Push(t.Context(), &atmosgit.PushOptions{
			RepoContext: atmosgit.RepoContext{Branch: "main"}, Retries: 9,
			Retry: &schema.RetryConfig{MaxAttempts: &attempts},
		})
		require.Error(t, err)
		require.Len(t, runner.calls, 2*attempts-1)
	}
}

func TestPushEmptyAndInvalidRetryPolicy(t *testing.T) {
	for _, policy := range []*schema.RetryConfig{{}, {RandomJitter: new(2.0)}, {MaxAttempts: new(0)}, {Conditions: []string{"["}}} {
		runner := newFakeRunner()
		runner.on("push", atmosgit.RunResult{StderrTail: "non-fast-forward"}, exitErr(1))
		err := New(WithRunner(runner)).Push(t.Context(), &atmosgit.PushOptions{Retries: 3, Retry: policy})
		require.Error(t, err)
		if policy.RandomJitter == nil && policy.MaxAttempts == nil && policy.Conditions == nil {
			require.Len(t, runner.calls, 1)
		} else {
			require.Empty(t, runner.calls)
		}
	}
}

func TestPushConfiguredBackoff(t *testing.T) {
	runner := newFakeRunner()
	runner.on("push", atmosgit.RunResult{StderrTail: "non-fast-forward"}, exitErr(1))
	config, err := pushRetryConfig(&atmosgit.PushOptions{Retry: &schema.RetryConfig{
		MaxAttempts: new(4), InitialDelay: new(time.Second), MaxDelay: new(4 * time.Second),
		Multiplier: new(3.0), RandomJitter: new(0.0), BackoffStrategy: schema.BackoffExponential,
	}})
	require.NoError(t, err)
	clock := &advancingPushClock{}
	err = New(WithRunner(runner)).pushWithRetry(context.Background(), &atmosgit.PushOptions{}, config, retry.WithClock(clock))
	require.Error(t, err)
	require.Equal(t, []time.Duration{time.Second, 3 * time.Second, 4 * time.Second}, clock.waits)
}

func TestPushConditionsCannotRetryAuthentication(t *testing.T) {
	for _, output := range []string{"authentication failed non-fast-forward", "non-fast-forward"} {
		runner := newFakeRunner()
		runner.on("push", atmosgit.RunResult{StderrTail: output}, exitErr(1))
		err := New(WithRunner(runner)).Push(t.Context(), &atmosgit.PushOptions{Retry: &schema.RetryConfig{
			MaxAttempts: new(3), Conditions: []string{"authentication"},
		}})
		require.Error(t, err)
		require.Len(t, runner.calls, 1)
	}
}
