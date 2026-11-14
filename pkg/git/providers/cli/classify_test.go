package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPushRetriesHashesContainingHTTPStatusDigits(t *testing.T) {
	for _, hash := range []string{"11111111111114011111111111111111111111111", "11111111111114031111111111111111111111111"} {
		t.Run(hash, func(t *testing.T) {
			runner := newFakeRunner()
			runner.on("push", atmosgit.RunResult{StderrTail: "remote: error: cannot lock ref 'refs/heads/main': is at " + hash + " but expected 2222222222222222222222222222222222222222\n! [remote rejected] main -> main (failed to update ref)"}, exitErr(1))
			runner.on("push", atmosgit.RunResult{}, nil)
			err := New(WithRunner(runner)).Push(t.Context(), &atmosgit.PushOptions{
				RepoContext: atmosgit.RepoContext{Branch: "main"}, Retry: &schema.RetryConfig{MaxAttempts: new(2)},
			})
			require.NoError(t, err)
			require.Equal(t, []string{"push origin main", "pull --rebase origin main", "push origin main"}, runner.joinedCalls())
		})
	}
}

func TestClassifyHTTPAuthenticationStatus(t *testing.T) {
	for _, message := range []string{
		"fatal: The requested URL returned error: 401", "fatal: The requested URL returned error: 403",
		"remote: HTTP/2 403 Forbidden", "remote: HTTP 401 Unauthorized",
	} {
		t.Run(message, func(t *testing.T) {
			require.ErrorIs(t, classify(exitErr(1), atmosgit.RunResult{StderrTail: message}, "push"), errUtils.ErrGitAuthFailed)
		})
	}
	for _, message := range []string{
		"remote: error: cannot lock ref 'refs/heads/release-401': is at 123403abcd but expected 456401efab",
		"fatal: unable to access 'https://example.invalid/401/repo': The requested URL returned error: 429",
	} {
		require.NotErrorIs(t, classify(exitErr(1), atmosgit.RunResult{StderrTail: message}, "push"), errUtils.ErrGitAuthFailed)
	}
}
