package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

func TestPushSucceedsFirstAttempt(t *testing.T) {
	runner := newFakeRunner()
	provider := New(WithRunner(runner))

	err := provider.Push(context.Background(), &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Remote: "origin", Branch: "main"},
		Retries:     3,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"push origin main"}, runner.joinedCalls())
}

func TestPushRetriesOnRejectionThenSucceeds(t *testing.T) {
	runner := newFakeRunner()
	rejected := atmosgit.RunResult{ExitCode: 1, StderrTail: "! [rejected] main -> main (fetch first)\nerror: failed to push some refs"}
	runner.on("push origin main", rejected, exitErr(1))
	runner.on("push origin main", atmosgit.RunResult{}, nil)
	provider := New(WithRunner(runner))

	err := provider.Push(context.Background(), &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Branch: "main"},
		Retries:     3,
	})
	require.NoError(t, err)

	calls := runner.joinedCalls()
	assert.Equal(t, []string{
		"push origin main",
		"pull --rebase origin main",
		"push origin main",
	}, calls)
}

func TestPushExhaustsRetries(t *testing.T) {
	runner := newFakeRunner()
	rejected := atmosgit.RunResult{ExitCode: 1, StderrTail: "non-fast-forward"}
	runner.on("push origin main", rejected, exitErr(1))
	provider := New(WithRunner(runner))

	err := provider.Push(context.Background(), &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Branch: "main"},
		Retries:     2,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitPushRejected))
	assert.Contains(t, err.Error(), "origin/main")
	assert.Contains(t, err.Error(), "2 retries")

	// 3 push attempts (initial + 2 retries), 2 rebases between them.
	calls := runner.joinedCalls()
	assert.Equal(t, []string{
		"push origin main",
		"pull --rebase origin main",
		"push origin main",
		"pull --rebase origin main",
		"push origin main",
	}, calls)
}

func TestPushZeroRetriesFailsImmediately(t *testing.T) {
	runner := newFakeRunner()
	runner.on("push origin main", atmosgit.RunResult{ExitCode: 1, StderrTail: "non-fast-forward"}, exitErr(1))
	provider := New(WithRunner(runner))

	err := provider.Push(context.Background(), &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Branch: "main"},
		Retries:     0,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitPushRejected))
	assert.Len(t, runner.calls, 1)
}

func TestPushNonRejectionErrorNotRetried(t *testing.T) {
	runner := newFakeRunner()
	runner.on("push origin main", atmosgit.RunResult{ExitCode: 128, StderrTail: "fatal: could not read Username"}, exitErr(128))
	provider := New(WithRunner(runner))

	err := provider.Push(context.Background(), &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Branch: "main"},
		Retries:     3,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitAuthFailed))
	assert.Len(t, runner.calls, 1) // No retry on auth failure.
}

func TestClassifyPatterns(t *testing.T) {
	assert.True(t, isRejectedPush(atmosgit.RunResult{StderrTail: "hint: Updates were rejected... non-fast-forward"}))
	assert.True(t, isRejectedPush(atmosgit.RunResult{StderrTail: "! [rejected]"}))
	assert.False(t, isRejectedPush(atmosgit.RunResult{StderrTail: "fatal: repository not found"}))

	base := exitErr(128)
	authErr := classify(base, atmosgit.RunResult{StderrTail: "remote: Invalid username or password"}, "push")
	assert.True(t, errors.Is(authErr, errUtils.ErrGitAuthFailed))

	plain := classify(base, atmosgit.RunResult{StderrTail: "fatal: not a git repository"}, "status")
	assert.False(t, errors.Is(plain, errUtils.ErrGitAuthFailed))
	assert.True(t, errors.Is(plain, errUtils.ErrGitCommandExited))

	assert.NoError(t, classify(nil, atmosgit.RunResult{}, "noop"))
}
