package updater

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// newGitFixture stands up a bare remote plus a cloned workdir with an initial "vendor.yaml" commit
// already pushed to "main", the shape every branch/publish test in this package needs.
func newGitFixture(t *testing.T) (remote, workdir string) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	workdir = filepath.Join(root, "workdir")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, output)
	}
	run(root, "init", "--bare", remote)
	run(remote, "symbolic-ref", "HEAD", "refs/heads/main")
	run(root, "clone", remote, workdir)
	run(workdir, "config", "user.name", "Atmos Test")
	run(workdir, "config", "user.email", "atmos-test@example.com")
	run(workdir, "config", "commit.gpgSign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte("before\n"), 0o644))
	run(workdir, "add", "vendor.yaml")
	run(workdir, "commit", "-m", "base")
	run(workdir, "branch", "-M", "main")
	run(workdir, "push", "-u", "origin", "main")
	return remote, workdir
}

func TestPrepareBranch(t *testing.T) {
	_, workdir := newGitFixture(t)

	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "main", "updates", "all")
	require.NoError(t, err)
	assert.Equal(t, "updates/all", branch)
	assert.Equal(t, "main", base)
}

func TestPrepareBranchResolvesDefaultBase(t *testing.T) {
	_, workdir := newGitFixture(t)

	// baseBranch is intentionally left empty so PrepareBranch must resolve it via
	// atmosgit.DefaultBranch, and branchPrefix left empty so it must fall back to
	// "atmos/component-updater".
	branch, base, err := PrepareBranch(context.Background(), workdir, "origin", "", "", "all")
	require.NoError(t, err)
	assert.Equal(t, "main", base)
	assert.Equal(t, "atmos/component-updater/all", branch)
}

func TestPrepareBranchDefaultBranchError(t *testing.T) {
	_, workdir := newGitFixture(t)
	cmd := exec.Command("git", "remote", "remove", "origin")
	cmd.Dir = workdir
	require.NoError(t, cmd.Run())

	_, _, err := PrepareBranch(context.Background(), workdir, "origin", "", "", "all")
	assert.Error(t, err)
}

func TestPrepareBranchPrepareBranchError(t *testing.T) {
	_, workdir := newGitFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "dirty.txt"), []byte("dirty\n"), 0o644))

	_, _, err := PrepareBranch(context.Background(), workdir, "origin", "main", "", "all")
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterDirtyWorktree)
}
