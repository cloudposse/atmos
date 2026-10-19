package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func runGitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	return string(out)
}

func newGitRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runGitCommand(t, root, "init", "--bare", remote)
	runGitCommand(t, remote, "symbolic-ref", "HEAD", "refs/heads/main")

	workdir := filepath.Join(root, "workdir")
	runGitCommand(t, root, "clone", remote, workdir)
	runGitCommand(t, workdir, "config", "user.name", "Atmos Test")
	runGitCommand(t, workdir, "config", "user.email", "atmos-test@example.com")
	runGitCommand(t, workdir, "config", "commit.gpgSign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "README.md"), []byte("base\n"), 0o644))
	runGitCommand(t, workdir, "add", "README.md")
	runGitCommand(t, workdir, "commit", "-m", "base")
	runGitCommand(t, workdir, "branch", "-M", "main")
	runGitCommand(t, workdir, "push", "-u", "origin", "main")
	return workdir
}

func TestPrepareBranchCreatesAndReusesFeatureBranch(t *testing.T) {
	workdir := newGitRemote(t)
	ctx := context.Background()
	baseSHA := strings.TrimSpace(runGitCommand(t, workdir, "rev-parse", "main"))

	require.NoError(t, PrepareBranch(ctx, PrepareBranchOptions{Workdir: workdir, Base: "main", Branch: "atmos/component-updater/all"}))
	assert.Equal(t, "atmos/component-updater/all", strings.TrimSpace(runGitCommand(t, workdir, "branch", "--show-current")))
	assert.Equal(t, baseSHA, strings.TrimSpace(runGitCommand(t, workdir, "rev-parse", "main")))
	assert.Equal(t, baseSHA, strings.TrimSpace(runGitCommand(t, workdir, "rev-parse", "origin/main")))

	require.NoError(t, os.WriteFile(filepath.Join(workdir, "feature.txt"), []byte("remote feature\n"), 0o644))
	runGitCommand(t, workdir, "add", "feature.txt")
	runGitCommand(t, workdir, "commit", "-m", "feature")
	runGitCommand(t, workdir, "push", "-u", "origin", "atmos/component-updater/all")
	runGitCommand(t, workdir, "checkout", "main")

	require.NoError(t, PrepareBranch(ctx, PrepareBranchOptions{Workdir: workdir, Remote: "origin", Base: "main", Branch: "atmos/component-updater/all"}))
	_, err := os.Stat(filepath.Join(workdir, "feature.txt"))
	require.NoError(t, err, "an existing remote feature branch must be reused")
	assert.Equal(t, baseSHA, strings.TrimSpace(runGitCommand(t, workdir, "rev-parse", "main")))
	assert.Equal(t, baseSHA, strings.TrimSpace(runGitCommand(t, workdir, "rev-parse", "origin/main")))
}

func TestPrepareBranchRejectsUnsafeInputs(t *testing.T) {
	ctx := context.Background()
	require.ErrorIs(t, PrepareBranch(ctx, PrepareBranchOptions{}), errUtils.ErrComponentUpdaterConfig)
	err := PrepareBranch(ctx, PrepareBranchOptions{Workdir: t.TempDir(), Base: "main", Branch: "updates"})
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)

	workdir := newGitRemote(t)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "dirty.txt"), []byte("dirty\n"), 0o644))
	err = PrepareBranch(ctx, PrepareBranchOptions{Workdir: workdir, Base: "main", Branch: "updates"})
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterDirtyWorktree)
}

func TestDefaultBranchAndGitHubRepository(t *testing.T) {
	workdir := newGitRemote(t)
	branch, err := DefaultBranch(context.Background(), workdir, "origin")
	require.NoError(t, err)
	assert.Equal(t, "main", branch)

	// An empty remote must default to "origin", same as passing it explicitly.
	branch, err = DefaultBranch(context.Background(), workdir, "")
	require.NoError(t, err)
	assert.Equal(t, "main", branch)

	runGitCommand(t, workdir, "remote", "set-url", "origin", "git@github.com:cloudposse/atmos.git")
	owner, repository, err := GitHubRepository(context.Background(), workdir, "origin")
	require.NoError(t, err)
	assert.Equal(t, "cloudposse", owner)
	assert.Equal(t, "atmos", repository)

	owner, repository, err = GitHubRepository(context.Background(), workdir, "")
	require.NoError(t, err)
	assert.Equal(t, "cloudposse", owner)
	assert.Equal(t, "atmos", repository)

	runGitCommand(t, workdir, "remote", "set-url", "origin", "https://github.com/cloudposse/atmos.git")
	owner, repository, err = GitHubRepository(context.Background(), workdir, "origin")
	require.NoError(t, err)
	assert.Equal(t, "cloudposse", owner)
	assert.Equal(t, "atmos", repository)

	runGitCommand(t, workdir, "remote", "set-url", "origin", "https://gitlab.com/cloudposse/atmos.git")
	_, _, err = GitHubRepository(context.Background(), workdir, "origin")
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)

	runGitCommand(t, workdir, "remote", "set-url", "origin", "https://example.com/github.com/cloudposse/atmos.git")
	_, _, err = GitHubRepository(context.Background(), workdir, "origin")
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)

	runGitCommand(t, workdir, "remote", "set-url", "origin", "https://github.com/cloudposse/")
	_, _, err = GitHubRepository(context.Background(), workdir, "origin")
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
}

func TestPrepareBranchFetchBaseFailure(t *testing.T) {
	workdir := newGitRemote(t)
	err := PrepareBranch(context.Background(), PrepareBranchOptions{Workdir: workdir, Remote: "nonexistent-remote", Base: "main", Branch: "updates"})
	assert.Error(t, err)
}

// TestPrepareBranchCheckoutFailures forces a git ref-update failure by
// pre-creating the ref's lock file: git's atomic ref update refuses to
// proceed while a sibling ".lock" file already exists, so this deterministic
// technique (used by git's own test suite) reproduces "checkout -B" failing
// without needing a real concurrent writer.
func TestPrepareBranchCheckoutFailures(t *testing.T) {
	t.Run("new branch from base", func(t *testing.T) {
		workdir := newGitRemote(t)
		lockPath := filepath.Join(workdir, ".git", "refs", "heads", "updates.lock")
		require.NoError(t, os.WriteFile(lockPath, []byte(""), 0o644))

		err := PrepareBranch(context.Background(), PrepareBranchOptions{Workdir: workdir, Base: "main", Branch: "updates"})
		assert.Error(t, err)
	})

	t.Run("existing remote branch", func(t *testing.T) {
		workdir := newGitRemote(t)
		runGitCommand(t, workdir, "checkout", "-b", "feature")
		require.NoError(t, os.WriteFile(filepath.Join(workdir, "feature.txt"), []byte("remote feature\n"), 0o644))
		runGitCommand(t, workdir, "add", "feature.txt")
		runGitCommand(t, workdir, "commit", "-m", "feature")
		runGitCommand(t, workdir, "push", "-u", "origin", "feature")
		runGitCommand(t, workdir, "checkout", "main")

		lockPath := filepath.Join(workdir, ".git", "refs", "heads", "feature.lock")
		require.NoError(t, os.WriteFile(lockPath, []byte(""), 0o644))

		err := PrepareBranch(context.Background(), PrepareBranchOptions{Workdir: workdir, Base: "main", Branch: "feature"})
		assert.Error(t, err)
	})
}

func TestDefaultBranchLsRemoteFailure(t *testing.T) {
	workdir := newGitRemote(t)
	_, err := DefaultBranch(context.Background(), workdir, "nonexistent-remote")
	assert.Error(t, err)
}

func TestGitHubRepositoryRemoteGetURLFailure(t *testing.T) {
	workdir := newGitRemote(t)
	_, _, err := GitHubRepository(context.Background(), workdir, "nonexistent-remote")
	assert.Error(t, err)
}

func TestDefaultBranchRejectsRemoteWithoutAdvertisedHead(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runGitCommand(t, root, "init", "--bare", remote)
	runGitCommand(t, remote, "symbolic-ref", "HEAD", "refs/heads/main")
	workdir := filepath.Join(root, "workdir")
	runGitCommand(t, root, "init", workdir)
	runGitCommand(t, workdir, "remote", "add", "origin", remote)

	_, err := DefaultBranch(context.Background(), workdir, "origin")
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
}

// TestPrepareBranchProtectsDivergedLocalBranch proves the no-remote-branch path never
// force-resets a local branch carrying commits absent from the base: PrepareBranch must fail
// with ErrGitLocalBranchDiverged instead of silently discarding the committed work.
func TestPrepareBranchProtectsDivergedLocalBranch(t *testing.T) {
	workdir := newGitRemote(t)
	ctx := context.Background()

	runGitCommand(t, workdir, "checkout", "-b", "atmos/component-updater/all")
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "unpushed.txt"), []byte("unpushed\n"), 0o644))
	runGitCommand(t, workdir, "add", "unpushed.txt")
	runGitCommand(t, workdir, "commit", "-m", "unpushed work")
	localSHA := strings.TrimSpace(runGitCommand(t, workdir, "rev-parse", "HEAD"))
	runGitCommand(t, workdir, "checkout", "main")

	err := PrepareBranch(ctx, PrepareBranchOptions{Workdir: workdir, Base: "main", Branch: "atmos/component-updater/all"})
	require.ErrorIs(t, err, errUtils.ErrGitLocalBranchDiverged)
	assert.Equal(t, localSHA, strings.TrimSpace(runGitCommand(t, workdir, "rev-parse", "atmos/component-updater/all")),
		"diverged local branch must be left untouched")
}

// TestPrepareBranchResetsLocalBranchAtBase is the negative path: a stale local branch with no
// commits beyond the fetched base holds no unique work, so PrepareBranch still reuses it.
func TestPrepareBranchResetsLocalBranchAtBase(t *testing.T) {
	workdir := newGitRemote(t)
	ctx := context.Background()

	runGitCommand(t, workdir, "branch", "atmos/component-updater/all", "main")

	require.NoError(t, PrepareBranch(ctx, PrepareBranchOptions{Workdir: workdir, Base: "main", Branch: "atmos/component-updater/all"}))
	assert.Equal(t, "atmos/component-updater/all", strings.TrimSpace(runGitCommand(t, workdir, "branch", "--show-current")))
}

// TestGithubRepositoryPath_SCPStylePort pins CodeRabbit thread PRRT_kwDOEW4XoM6h6mmi: an
// SCP-style remote (git@host:org/repo.git) carries no port of its own, so the comparison must
// use the portless hostname (endpoints.Hostname()) rather than endpoints.Host, which keeps a
// non-default port. Without this, a GHES remote configured with a port (e.g.
// "https://ghe.example.com:8443") would never match its own SCP-style remote.
func TestGithubRepositoryPath_SCPStylePort(t *testing.T) {
	t.Run("SCP-style remote matches a GHES host configured with a non-default port", func(t *testing.T) {
		t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com:8443")

		path, ok := githubRepositoryPath("git@ghe.example.com:org/repo.git")
		require.True(t, ok, "expected the SCP-style remote to match the configured GHES host")
		assert.Equal(t, "org/repo.git", path)
	})

	t.Run("SCP-style remote for an unrelated host is still rejected", func(t *testing.T) {
		t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com:8443")

		_, ok := githubRepositoryPath("git@some-other-host.example.com:org/repo.git")
		assert.False(t, ok, "expected an unrelated host not to match the configured GHES host")
	})

	t.Run("URL-style remote still requires the port to match", func(t *testing.T) {
		t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com:8443")

		_, ok := githubRepositoryPath("https://ghe.example.com:9999/org/repo.git")
		assert.False(t, ok, "expected a URL-style remote on a different port not to match")

		path, ok := githubRepositoryPath("https://ghe.example.com:8443/org/repo.git")
		require.True(t, ok, "expected a URL-style remote on the configured port to match")
		assert.Equal(t, "org/repo.git", path)
	})

	t.Run("no port is unchanged", func(t *testing.T) {
		t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com")

		path, ok := githubRepositoryPath("git@ghe.example.com:org/repo.git")
		require.True(t, ok)
		assert.Equal(t, "org/repo.git", path)
	})

	t.Run("public github.com SCP-style remote is unaffected", func(t *testing.T) {
		path, ok := githubRepositoryPath("git@github.com:cloudposse/atmos.git")
		require.True(t, ok)
		assert.Equal(t, "cloudposse/atmos.git", path)
	})
}
