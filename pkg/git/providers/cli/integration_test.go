package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

// gitTestEnv builds an isolated environment: a temp global config (so the
// developer's gitconfig, signing setup, and default branch cannot leak in)
// with a deterministic "main" default branch and a known fallback user.
func gitTestEnv(t *testing.T) []string {
	t.Helper()

	globalCfg := filepath.Join(t.TempDir(), "gitconfig")
	cfg := "[init]\n\tdefaultBranch = main\n[user]\n\tname = Global User\n\temail = global@example.com\n"
	require.NoError(t, os.WriteFile(globalCfg, []byte(cfg), 0o600))

	return append(
		os.Environ(),
		"GIT_CONFIG_GLOBAL="+globalCfg,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
}

// gitRun executes a raw git command for test setup/verification.
func gitRun(t *testing.T, env []string, dir string, args ...string) string {
	t.Helper()

	result, err := atmosgit.NewExecRunner().Run(context.Background(), "git", args, atmosgit.RunOptions{Dir: dir, Env: env})
	require.NoError(t, err, "git %v: stderr tail: %s", args, result.StderrTail)
	return result.Stdout
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func requireGitBinary(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

// TestIntegrationPublishFlow exercises the full publish lifecycle against a
// local bare repository (no network): clone, commit with author/trailers,
// push, concurrent-publisher push rejection with rebase retry, and
// reconcile of an existing workdir.
func TestIntegrationPublishFlow(t *testing.T) {
	requireGitBinary(t)

	ctx := context.Background()
	env := gitTestEnv(t)
	provider := New()
	root := t.TempDir()

	// Bare "remote" with deterministic default branch.
	bare := filepath.Join(root, "origin.git")
	gitRun(t, env, "", "init", "--bare", bare)
	gitRun(t, env, bare, "symbolic-ref", "HEAD", "refs/heads/main")

	workA := filepath.Join(root, "workA")
	workB := filepath.Join(root, "workB")
	author := &atmosgit.Author{Name: "atmos[bot]", Email: "bot@acme.com"}

	// Clone the empty repository and create the initial commit.
	rcA := atmosgit.RepoContext{Workdir: workA, Branch: "main", Env: env}
	require.NoError(t, provider.Clone(ctx, &atmosgit.CloneOptions{
		RepoContext: atmosgit.RepoContext{Workdir: workA, Env: env}, // no branch: empty repo.
		URI:         bare,
	}))

	writeFile(t, filepath.Join(workA, "clusters", "prod", "app.yaml"), "v: 1\n")
	commit, err := provider.Commit(ctx, &atmosgit.CommitOptions{
		RepoContext: rcA,
		Message:     "Render argocd for prod",
		Paths:       []string{"clusters/prod"},
		Signing:     atmosgit.SigningNever,
		Author:      author,
		Trailers:    map[string]string{"Atmos-Stack": "prod", "Atmos-Component": "argocd"},
	})
	require.NoError(t, err)
	require.True(t, commit.Committed)
	require.NotEmpty(t, commit.SHA)
	require.NoError(t, provider.Push(ctx, &atmosgit.PushOptions{RepoContext: rcA, Retries: 0}))

	// Author injection wins over the global config user; trailers are present.
	body := gitRun(t, env, workA, "log", "-1", "--format=%an <%ae>%n%B", commit.SHA)
	assert.Contains(t, body, "atmos[bot] <bot@acme.com>")
	assert.Contains(t, body, "Atmos-Stack: prod")
	assert.Contains(t, body, "Atmos-Component: argocd")

	// Second consumer clones at main.
	rcB := atmosgit.RepoContext{Workdir: workB, Branch: "main", Env: env}
	require.NoError(t, provider.Clone(ctx, &atmosgit.CloneOptions{RepoContext: rcB, URI: bare}))
	assert.FileExists(t, filepath.Join(workB, "clusters", "prod", "app.yaml"))

	// Concurrent publisher: A pushes again before B.
	writeFile(t, filepath.Join(workA, "clusters", "prod", "app.yaml"), "v: 2\n")
	_, err = provider.Commit(ctx, &atmosgit.CommitOptions{
		RepoContext: rcA, Message: "bump", Paths: []string{"clusters/prod"},
		Signing: atmosgit.SigningNever, Author: author,
	})
	require.NoError(t, err)
	require.NoError(t, provider.Push(ctx, &atmosgit.PushOptions{RepoContext: rcA, Retries: 0}))

	// B's diff sees its own pending change before commit.
	writeFile(t, filepath.Join(workB, "clusters", "prod", "second.yaml"), "v: 1\n")
	diff, err := provider.Diff(ctx, &atmosgit.DiffOptions{RepoContext: rcB})
	require.NoError(t, err)
	assert.True(t, diff.HasChanges)
	assert.Contains(t, diff.Untracked, "clusters/prod/second.yaml")

	// B commits and pushes: rejected (A moved the remote), rebased, retried.
	_, err = provider.Commit(ctx, &atmosgit.CommitOptions{
		RepoContext: rcB, Message: "add second", Paths: []string{"clusters/prod"},
		Signing: atmosgit.SigningNever, Author: author,
	})
	require.NoError(t, err)
	require.NoError(t, provider.Push(ctx, &atmosgit.PushOptions{RepoContext: rcB, Retries: 2}))

	// Reconcile A's existing workdir: fast-forwards to B's published state.
	require.NoError(t, provider.Clone(ctx, &atmosgit.CloneOptions{RepoContext: rcA, URI: bare}))
	assert.FileExists(t, filepath.Join(workA, "clusters", "prod", "second.yaml"))

	content, err := os.ReadFile(filepath.Join(workA, "clusters", "prod", "app.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "v: 2\n", string(content))

	// Everything published: status is clean, commit is a no-op.
	status, err := provider.Status(ctx, &atmosgit.StatusOptions{RepoContext: rcA})
	require.NoError(t, err)
	assert.True(t, status.Clean)

	noop, err := provider.Commit(ctx, &atmosgit.CommitOptions{
		RepoContext: rcA, Message: "noop", Paths: []string{"clusters/prod"},
		Signing: atmosgit.SigningNever, Author: author,
	})
	require.NoError(t, err)
	assert.False(t, noop.Committed)
}

// TestIntegrationPullFFOnly verifies pull fast-forwards and refuses divergence.
func TestIntegrationPullFFOnly(t *testing.T) {
	requireGitBinary(t)

	ctx := context.Background()
	env := gitTestEnv(t)
	provider := New()
	root := t.TempDir()

	bare := filepath.Join(root, "origin.git")
	gitRun(t, env, "", "init", "--bare", bare)
	gitRun(t, env, bare, "symbolic-ref", "HEAD", "refs/heads/main")

	workA := filepath.Join(root, "workA")
	workB := filepath.Join(root, "workB")
	author := &atmosgit.Author{Name: "atmos[bot]", Email: "bot@acme.com"}

	rcA := atmosgit.RepoContext{Workdir: workA, Branch: "main", Env: env}
	require.NoError(t, provider.Clone(ctx, &atmosgit.CloneOptions{RepoContext: atmosgit.RepoContext{Workdir: workA, Env: env}, URI: bare}))
	writeFile(t, filepath.Join(workA, "a.txt"), "1\n")
	_, err := provider.Commit(ctx, &atmosgit.CommitOptions{RepoContext: rcA, Message: "init", Paths: []string{"a.txt"}, Signing: atmosgit.SigningNever, Author: author})
	require.NoError(t, err)
	require.NoError(t, provider.Push(ctx, &atmosgit.PushOptions{RepoContext: rcA, Retries: 0}))

	rcB := atmosgit.RepoContext{Workdir: workB, Branch: "main", Env: env}
	require.NoError(t, provider.Clone(ctx, &atmosgit.CloneOptions{RepoContext: rcB, URI: bare}))

	// A advances the remote; B pulls fast-forward.
	writeFile(t, filepath.Join(workA, "a.txt"), "2\n")
	_, err = provider.Commit(ctx, &atmosgit.CommitOptions{RepoContext: rcA, Message: "bump", Paths: []string{"a.txt"}, Signing: atmosgit.SigningNever, Author: author})
	require.NoError(t, err)
	require.NoError(t, provider.Push(ctx, &atmosgit.PushOptions{RepoContext: rcA, Retries: 0}))

	require.NoError(t, provider.Pull(ctx, &atmosgit.PullOptions{RepoContext: rcB}))
	content, err := os.ReadFile(filepath.Join(workB, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "2\n", string(content))

	// Divergence: both sides commit; B's ff-only pull must fail, not merge.
	writeFile(t, filepath.Join(workA, "a.txt"), "3\n")
	_, err = provider.Commit(ctx, &atmosgit.CommitOptions{RepoContext: rcA, Message: "a3", Paths: []string{"a.txt"}, Signing: atmosgit.SigningNever, Author: author})
	require.NoError(t, err)
	require.NoError(t, provider.Push(ctx, &atmosgit.PushOptions{RepoContext: rcA, Retries: 0}))

	writeFile(t, filepath.Join(workB, "b.txt"), "1\n")
	_, err = provider.Commit(ctx, &atmosgit.CommitOptions{RepoContext: rcB, Message: "b1", Paths: []string{"b.txt"}, Signing: atmosgit.SigningNever, Author: author})
	require.NoError(t, err)

	err = provider.Pull(ctx, &atmosgit.PullOptions{RepoContext: rcB})
	require.Error(t, err, "ff-only pull must refuse to merge diverged histories")
	assert.False(t, strings.Contains(err.Error(), bare), "error must not echo repository paths from stderr")
}
