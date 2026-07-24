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
