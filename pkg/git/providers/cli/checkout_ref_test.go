package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/stretchr/testify/require"
)

func TestCloneExactCommit(t *testing.T) {
	requireGitBinary(t)
	env := gitTestEnv(t)
	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	gitRun(t, env, "", "init", origin)
	writeFile(t, filepath.Join(origin, "file"), "first")
	gitRun(t, env, origin, "add", ".")
	gitRun(t, env, origin, "-c", "commit.gpgsign=false", "commit", "-m", "first")
	first := strings.TrimSpace(gitRun(t, env, origin, "rev-parse", "HEAD"))
	writeFile(t, filepath.Join(origin, "file"), "second")
	gitRun(t, env, origin, "add", ".")
	gitRun(t, env, origin, "-c", "commit.gpgsign=false", "commit", "-m", "second")
	second := strings.TrimSpace(gitRun(t, env, origin, "rev-parse", "HEAD"))
	p := New()
	opts := &atmosgit.CloneOptions{URI: "file://" + origin, Depth: 1, Ref: first, RepoContext: atmosgit.RepoContext{Workdir: filepath.Join(root, "checkout"), Env: env}}
	require.NoError(t, p.Clone(context.Background(), opts))
	require.Equal(t, first, strings.TrimSpace(gitRun(t, env, opts.Workdir, "rev-parse", "HEAD")))
	require.Equal(t, "HEAD", strings.TrimSpace(gitRun(t, env, opts.Workdir, "rev-parse", "--abbrev-ref", "HEAD")))
	// Reconcile both forward and backward without fast-forwarding past the event revision.
	for _, sha := range []string{second, first} {
		opts.Ref = sha
		require.NoError(t, p.Clone(context.Background(), opts))
		require.Equal(t, sha, strings.TrimSpace(gitRun(t, env, opts.Workdir, "rev-parse", "HEAD")))
	}
	gitRun(t, env, origin, "tag", "v1", first)
	for _, ref := range []string{"main", "refs/heads/main", "v1", "refs/tags/v1"} {
		opts.Ref = ref
		require.NoError(t, p.Clone(context.Background(), opts))
		expected := first
		if strings.Contains(ref, "main") {
			expected = second
		}
		require.Equal(t, expected, strings.TrimSpace(gitRun(t, env, opts.Workdir, "rev-parse", "HEAD")))
		require.Equal(t, "HEAD", strings.TrimSpace(gitRun(t, env, opts.Workdir, "rev-parse", "--abbrev-ref", "HEAD")))
	}
	writeFile(t, filepath.Join(opts.Workdir, "file"), "uncommitted")
	opts.Ref = second
	require.Error(t, p.Clone(context.Background(), opts))
	require.Equal(t, first, strings.TrimSpace(gitRun(t, env, opts.Workdir, "rev-parse", "HEAD")))
}

func TestCloneExactCommitNonDefaultRemote(t *testing.T) {
	requireGitBinary(t)
	env := gitTestEnv(t)
	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	gitRun(t, env, "", "init", origin)
	writeFile(t, filepath.Join(origin, "file"), "first")
	gitRun(t, env, origin, "add", ".")
	gitRun(t, env, origin, "-c", "commit.gpgsign=false", "commit", "-m", "first")
	first := strings.TrimSpace(gitRun(t, env, origin, "rev-parse", "HEAD"))
	opts := &atmosgit.CloneOptions{
		URI: "file://" + origin, Ref: first,
		RepoContext: atmosgit.RepoContext{Workdir: filepath.Join(root, "checkout"), Remote: "upstream", Env: env},
	}
	require.NoError(t, New().Clone(context.Background(), opts))
	require.Equal(t, first, strings.TrimSpace(gitRun(t, env, opts.Workdir, "rev-parse", "HEAD")))
	require.Equal(t, "upstream", strings.TrimSpace(gitRun(t, env, opts.Workdir, "remote")))
}

func TestCloneExactCommitWithConfiguredGitDefaultRemote(t *testing.T) {
	requireGitBinary(t)
	env := append(gitTestEnv(t), "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=clone.defaultRemoteName", "GIT_CONFIG_VALUE_0=upstream")
	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	gitRun(t, env, "", "init", origin)
	writeFile(t, filepath.Join(origin, "file"), "first")
	gitRun(t, env, origin, "add", ".")
	gitRun(t, env, origin, "-c", "commit.gpgsign=false", "commit", "-m", "first")
	first := strings.TrimSpace(gitRun(t, env, origin, "rev-parse", "HEAD"))
	opts := &atmosgit.CloneOptions{
		URI: "file://" + origin, Ref: first,
		RepoContext: atmosgit.RepoContext{Workdir: filepath.Join(root, "checkout"), Env: env},
	}
	require.NoError(t, New().Clone(context.Background(), opts))
	require.Equal(t, first, strings.TrimSpace(gitRun(t, env, opts.Workdir, "rev-parse", "HEAD")))
	require.Equal(t, "origin", strings.TrimSpace(gitRun(t, env, opts.Workdir, "remote")))
	require.NoError(t, New().Clone(context.Background(), opts))
}

func TestCheckoutRefFailureAndSubmodules(t *testing.T) {
	for _, failing := range []string{"fetch", "checkout", "submodule"} {
		t.Run(failing, func(t *testing.T) {
			runner := newFakeRunner()
			runner.on(failing, atmosgit.RunResult{}, exitErr(1))
			opts := &atmosgit.CloneOptions{Ref: "refs/tags/v1", Submodules: true, Depth: 1, Filter: "blob:none", RepoContext: atmosgit.RepoContext{Workdir: t.TempDir()}}
			require.Error(t, New(WithRunner(runner)).checkoutRef(context.Background(), opts))
		})
	}
	runner := newFakeRunner()
	opts := &atmosgit.CloneOptions{Ref: "refs/tags/v1", Submodules: true, RepoContext: atmosgit.RepoContext{Workdir: t.TempDir()}}
	require.NoError(t, New(WithRunner(runner)).checkoutRef(context.Background(), opts))
	require.Equal(t, []string{"fetch -- origin refs/tags/v1", "checkout --detach FETCH_HEAD", "submodule update --init --recursive"}, runner.joinedCalls())
	opts.Branch = "main"
	require.Error(t, New().Clone(context.Background(), opts))
}
