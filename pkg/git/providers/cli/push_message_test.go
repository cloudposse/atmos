package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// rejectedPushError runs a push that the fake remote always rejects.
func rejectedPushError(t *testing.T, remote string, retries int) error {
	t.Helper()

	runner := newFakeRunner()
	runner.on("push "+remoteOrDefault(remote)+" main", atmosgit.RunResult{ExitCode: 1, StderrTail: "! [rejected] main -> main (fetch first)"}, exitErr(1))
	err := New(WithRunner(runner)).Push(context.Background(), &atmosgit.PushOptions{
		RepoContext: atmosgit.RepoContext{Workdir: "/w", Remote: remote, Branch: "main"},
		Retries:     retries,
	})
	require.Error(t, err)
	return err
}

// The rejection message names the remote and branch and is identical on every run.
func TestPushRejectionMessageNamesTargetDeterministically(t *testing.T) {
	cases := []struct {
		name    string
		remote  string
		retries int
		want    string
	}{
		{"no retries", "", 0, "git push rejected: non-fast-forward: origin/main after 1 attempt(s), 0 retries"},
		{"one retry", "origin", 1, "git push rejected: non-fast-forward: origin/main after 2 attempt(s), 1 retry"},
		{"custom remote", "upstream", 2, "git push rejected: non-fast-forward: upstream/main after 3 attempt(s), 2 retries"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := rejectedPushError(t, tc.remote, tc.retries)
			require.ErrorIs(t, err, errUtils.ErrGitPushRejected)
			assert.Equal(t, tc.want, err.Error())
			assert.Equal(t, err.Error(), rejectedPushError(t, tc.remote, tc.retries).Error())
		})
	}
}

// A remote given as a URL is named without its credentials.
func TestPushRejectionMessageRedactsRemoteURL(t *testing.T) {
	err := rejectedPushError(t, "https://user:secret@git.example.com/acme/infra.git", 0)
	require.ErrorIs(t, err, errUtils.ErrGitPushRejected)
	assert.Equal(t, "git push rejected: non-fast-forward: https://git.example.com/acme/infra.git/main after 1 attempt(s), 0 retries", err.Error())
	assert.NotContains(t, err.Error(), "secret")
}

// A push without a branch names only the remote.
func TestPushTargetWithoutBranch(t *testing.T) {
	assert.Equal(t, "origin", pushTarget(&atmosgit.PushOptions{}))
	assert.Equal(t, "upstream/main", pushTarget(&atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Remote: "upstream", Branch: "main"}}))
}

// The elapsed time is still available, at debug level.
func TestPushFailureLogsElapsedAtDebug(t *testing.T) {
	var buf bytes.Buffer
	oldLevel := log.GetLevel()
	log.SetLevel(log.DebugLevel)
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(oldLevel)
	})

	err := rejectedPushError(t, "", 0)
	assert.NotContains(t, err.Error(), "elapsed")
	assert.Contains(t, buf.String(), "Git push failed")
	assert.Contains(t, buf.String(), "elapsed=")
	assert.Contains(t, buf.String(), "target=origin/main")
}

// A real rejected push against a local bare repository reports the same message, and a
// failed recovery rebase still names the target after the underlying Git error.
func TestPushRejectionMessageAgainstBareRepository(t *testing.T) {
	requireGitBinary(t)
	for _, mode := range []string{"rejected", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			env := gitTestEnv(t)
			root := t.TempDir()
			remote, local, competitor := filepath.Join(root, "origin.git"), filepath.Join(root, "local"), filepath.Join(root, "competitor")
			gitRun(t, env, "", "init", "--bare", remote)
			gitRun(t, env, "", "clone", remote, local)
			commitPushFile(t, env, local, "shared", "base\n")
			gitRun(t, env, local, "push", "origin", "main")
			gitRun(t, env, "", "clone", remote, competitor)
			commitPushFile(t, env, competitor, "shared", "competing\n")
			gitRun(t, env, competitor, "push", "origin", "main")
			commitPushFile(t, env, local, "shared", "local\n")

			retries := 0
			if mode == "conflict" {
				retries = 1
			}
			err := New().Push(context.Background(), &atmosgit.PushOptions{
				RepoContext: atmosgit.RepoContext{Workdir: local, Env: env, Branch: "main"},
				Retries:     retries,
			})
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "elapsed")
			assert.NotContains(t, err.Error(), "max attempts")
			if mode == "rejected" {
				require.ErrorIs(t, err, errUtils.ErrGitPushRejected)
				assert.Equal(t, "git push rejected: non-fast-forward: origin/main after 1 attempt(s), 0 retries", err.Error())
				return
			}
			require.ErrorIs(t, err, errUtils.ErrGitCommandExited)
			assert.Contains(t, err.Error(), "git pull")
			assert.Contains(t, err.Error(), ": origin/main after 1 attempt(s), 0 retries")
		})
	}
}
