package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

func sshRetryPolicy() *schema.RetryConfig {
	return &schema.RetryConfig{MaxAttempts: new(16), MaxElapsedTime: new(10 * time.Minute), InitialDelay: new(50 * time.Millisecond), MaxDelay: new(time.Second), BackoffStrategy: schema.BackoffExponential, RandomJitter: new(0.5)}
}

// Real Git and OpenSSH clients contend on one branch. Only cooldown scheduling is
// virtual here; TestSSHRateLimitWallClockCooldown separately validates elapsed time.
func TestSSHConcurrentPublishersRecoverFromThrottling(t *testing.T) {
	env := gitTestEnv(t)
	bare, seed := sshPushFixture(t, env)
	server := newSSHGitServer(t, env, bare, func(call sshTestCall, ordinal int) sshTestAction {
		if call.command == "receive-pack" && ordinal == 0 {
			return sshTestAction{message: "remote: rate limit exceeded"}
		}
		return sshTestAction{}
	})
	const publishers = 12
	dirs := make([]string, publishers)
	for i := range dirs {
		dirs[i] = sshPublisherFixture(t, env, seed, i)
	}
	type result struct {
		err   error
		waits []time.Duration
	}
	results := make(chan result, publishers)
	start := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	for i, dir := range dirs {
		go func() {
			<-start
			clock := &advancingPushClock{}
			opts := &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Workdir: dir, Env: server.env, Remote: server.remote(fmt.Sprintf("client-%d", i)), Branch: "main"}}
			err := New().pushWithRetry(ctx, opts, sshRetryPolicy(), retry.WithClock(clock))
			results <- result{err: err, waits: clock.waits}
		}()
	}
	close(start)
	for range publishers {
		result := <-results
		assert.NoError(t, result.err)
		if assert.NotEmpty(t, result.waits) {
			assert.GreaterOrEqual(t, result.waits[0], time.Minute)
			assert.LessOrEqual(t, result.waits[0], 90*time.Second)
		}
	}
	for i := range publishers {
		require.Equal(t, "image: v2\n", gitRun(t, env, bare, "show", fmt.Sprintf("main:app-%d.yaml", i)))
	}
	require.Equal(t, fmt.Sprint(publishers+1), strings.TrimSpace(gitRun(t, env, bare, "rev-list", "--count", "main")))
	reads, pushes := 0, 0
	for _, call := range server.snapshot() {
		if call.command == "upload-pack" {
			reads++
		} else {
			pushes++
		}
	}
	require.GreaterOrEqual(t, pushes, publishers*2)
	require.Positive(t, reads, "same-branch contention must exercise recovery fetches")
	t.Logf("%d/%d publishers completed; %d SSH pushes, %d SSH recovery fetches; cooldowns advanced by test clock", publishers, publishers, pushes, reads)
}

func TestSSHRateLimitFailureBoundaries(t *testing.T) {
	for _, mode := range []string{"budget", "attempts", "cancel", "authentication", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			env := gitTestEnv(t)
			bare, seed := sshPushFixture(t, env)
			server := newSSHGitServer(t, env, bare, func(_ sshTestCall, _ int) sshTestAction {
				if mode == "disconnect" {
					return sshTestAction{disconnect: true}
				}
				return sshTestAction{message: "remote: too many requests"}
			})
			user := "client"
			if mode == "authentication" {
				user = "denied"
			}
			config := sshRetryPolicy()
			if mode == "budget" {
				config.MaxElapsedTime = new(30 * time.Second)
			}
			if mode == "attempts" {
				config.MaxAttempts = new(2)
				config.RandomJitter = nil
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			clock := &advancingPushClock{}
			if mode == "cancel" {
				clock.onWait = cancel
			}
			err := New().pushWithRetry(ctx, &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Workdir: seed, Env: server.env, Remote: server.remote(user), Branch: "main"}}, config, retry.WithClock(clock))
			require.Error(t, err)
			switch mode {
			case "budget":
				require.ErrorIs(t, err, errUtils.ErrGitRateLimited)
				var budget retry.MaxElapsedTimeError
				require.ErrorAs(t, err, &budget)
				require.Empty(t, clock.waits)
				require.Len(t, server.snapshot(), 1)
			case "attempts":
				require.ErrorIs(t, err, errUtils.ErrGitRateLimited)
				require.Equal(t, []time.Duration{time.Minute}, clock.waits)
				require.Len(t, server.snapshot(), 2)
			case "cancel":
				require.ErrorIs(t, err, context.Canceled)
				require.Len(t, server.snapshot(), 1)
			case "authentication":
				require.ErrorIs(t, err, errUtils.ErrGitAuthFailed)
				require.Empty(t, server.snapshot())
				require.Empty(t, clock.waits)
			case "disconnect":
				require.NotErrorIs(t, err, errUtils.ErrGitRateLimited)
				require.Empty(t, clock.waits)
				require.Len(t, server.snapshot(), 1)
			}
		})
	}
}

func TestSSHThrottledRebaseRecoversBeforePush(t *testing.T) {
	env := gitTestEnv(t)
	bare, seed := sshPushFixture(t, env)
	local := sshPublisherFixture(t, env, seed, 0)
	competitor := sshPublisherFixture(t, env, seed, 1)
	gitRun(t, env, competitor, "push", bare, "main")
	server := newSSHGitServer(t, env, bare, func(call sshTestCall, ordinal int) sshTestAction {
		if call.command == "upload-pack" && ordinal == 0 {
			return sshTestAction{message: "remote: secondary rate limit exceeded"}
		}
		return sshTestAction{}
	})
	clock := &advancingPushClock{}
	config := sshRetryPolicy()
	config.RandomJitter = nil
	err := New().pushWithRetry(t.Context(), &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Workdir: local, Env: server.env, Remote: server.remote("client"), Branch: "main"}}, config, retry.WithClock(clock))
	require.NoError(t, err)
	var commands []string
	for _, call := range server.snapshot() {
		commands = append(commands, call.command)
	}
	require.Equal(t, []string{"receive-pack", "upload-pack", "upload-pack", "receive-pack"}, commands)
	require.Equal(t, []time.Duration{50 * time.Millisecond, time.Minute}, clock.waits)
	for i := range 2 {
		require.Equal(t, "image: v2\n", gitRun(t, env, bare, "show", fmt.Sprintf("main:app-%d.yaml", i)))
	}
}

func TestSSHRateLimitWallClockCooldown(t *testing.T) {
	if testing.Short() {
		t.Skip("validates the real 60-second SSH fallback cooldown")
	}
	env := gitTestEnv(t)
	bare, seed := sshPushFixture(t, env)
	local := sshPublisherFixture(t, env, seed, 0)
	server := newSSHGitServer(t, env, bare, func(_ sshTestCall, ordinal int) sshTestAction {
		if ordinal == 0 {
			return sshTestAction{message: "remote: rate limit exceeded"}
		}
		return sshTestAction{}
	})
	err := New().Push(t.Context(), &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Workdir: local, Env: server.env, Remote: server.remote("client"), Branch: "main"}, Retry: &schema.RetryConfig{MaxAttempts: new(2), MaxElapsedTime: new(90 * time.Second)}})
	require.NoError(t, err)
	calls := server.snapshot()
	require.Len(t, calls, 2)
	elapsed := calls[1].at.Sub(calls[0].at)
	require.GreaterOrEqual(t, elapsed, time.Minute)
	require.Equal(t, "image: v2\n", gitRun(t, env, bare, "show", "main:app-0.yaml"))
	t.Logf("SSH throttle-to-retry interval: %s; push succeeded", elapsed)
}
