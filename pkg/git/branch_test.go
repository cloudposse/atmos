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

	require.NoError(t, PrepareBranch(ctx, PrepareBranchOptions{Workdir: workdir, Base: "main", Branch: "atmos/component-updater/all"}))
	assert.Equal(t, "atmos/component-updater/all", strings.TrimSpace(runGitCommand(t, workdir, "branch", "--show-current")))

	require.NoError(t, os.WriteFile(filepath.Join(workdir, "feature.txt"), []byte("remote feature\n"), 0o644))
	runGitCommand(t, workdir, "add", "feature.txt")
	runGitCommand(t, workdir, "commit", "-m", "feature")
	runGitCommand(t, workdir, "push", "-u", "origin", "atmos/component-updater/all")
	runGitCommand(t, workdir, "checkout", "main")

	require.NoError(t, PrepareBranch(ctx, PrepareBranchOptions{Workdir: workdir, Remote: "origin", Base: "main", Branch: "atmos/component-updater/all"}))
	_, err := os.Stat(filepath.Join(workdir, "feature.txt"))
	require.NoError(t, err, "an existing remote feature branch must be reused")
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

	runGitCommand(t, workdir, "remote", "set-url", "origin", "git@github.com:cloudposse/atmos.git")
	owner, repository, err := GitHubRepository(context.Background(), workdir, "origin")
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

	runGitCommand(t, workdir, "remote", "set-url", "origin", "https://github.com/cloudposse/")
	_, _, err = GitHubRepository(context.Background(), workdir, "origin")
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
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
