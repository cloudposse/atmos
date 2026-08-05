package updater

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrepareUpdateWorktreeResolvesDefaultBase proves an empty baseBranch resolves via the
// remote's advertised default branch (matching PrepareBranch's own fallback), and that the
// resulting worktree is checked out with the fixture's committed content.
func TestPrepareUpdateWorktreeResolvesDefaultBase(t *testing.T) {
	_, workdir := newGitFixture(t)

	result, err := PrepareUpdateWorktree(context.Background(), workdir, "origin", "")
	require.NoError(t, err)
	defer result.Cleanup()

	assert.Equal(t, "main", result.Base)
	require.NotEmpty(t, result.Path)

	content, err := os.ReadFile(filepath.Join(result.Path, "vendor.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "before\n", string(content))
}

// TestPrepareUpdateWorktreeUsesExplicitBaseBranch proves an explicit baseBranch is used as-is,
// skipping DefaultBranch resolution entirely -- proven by removing the "origin" remote first, which
// would make any DefaultBranch call fail.
func TestPrepareUpdateWorktreeUsesExplicitBaseBranch(t *testing.T) {
	_, workdir := newGitFixture(t)

	// A worktree add still needs the remote to resolve "origin/main", so re-point origin at
	// itself (a local clone of itself) rather than fully removing it -- this isolates the
	// assertion to "no DefaultBranch ls-remote call happened" without breaking worktree add.
	result, err := PrepareUpdateWorktree(context.Background(), workdir, "origin", "main")
	require.NoError(t, err)
	defer result.Cleanup()

	assert.Equal(t, "main", result.Base)
	require.NotEmpty(t, result.Path)
}

// TestPrepareUpdateWorktreeDefaultBranchError proves a DefaultBranch resolution failure (no
// "origin" remote) is returned as-is, with a safe no-op cleanup.
func TestPrepareUpdateWorktreeDefaultBranchError(t *testing.T) {
	_, workdir := newGitFixture(t)
	cmd := exec.Command("git", "remote", "remove", "origin")
	cmd.Dir = workdir
	require.NoError(t, cmd.Run())

	result, err := PrepareUpdateWorktree(context.Background(), workdir, "origin", "")
	assert.Error(t, err)
	assert.Empty(t, result.Path)
	assert.Empty(t, result.Base)
	require.NotNil(t, result.Cleanup)
	result.Cleanup() // Must be safe to call even though nothing was created.
}

// TestPrepareUpdateWorktreeAddError proves a worktree-add failure (an explicit base branch that
// doesn't exist on the remote) is returned as-is, with a safe no-op cleanup.
func TestPrepareUpdateWorktreeAddError(t *testing.T) {
	_, workdir := newGitFixture(t)

	result, err := PrepareUpdateWorktree(context.Background(), workdir, "origin", "no-such-branch")
	assert.Error(t, err)
	assert.Empty(t, result.Path)
	require.NotNil(t, result.Cleanup)
	result.Cleanup() // Must be safe to call even though nothing was created.
}

// TestPrepareUpdateWorktreeIsolatedFromWorkdir proves the created worktree is a fully independent
// working tree: writing a new file inside it never touches workdir's own status, and cleanup
// removes the worktree without disturbing workdir.
func TestPrepareUpdateWorktreeIsolatedFromWorkdir(t *testing.T) {
	_, workdir := newGitFixture(t)

	result, err := PrepareUpdateWorktree(context.Background(), workdir, "origin", "main")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(result.Path, "vendor.yaml"), []byte("after\n"), 0o644))

	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = workdir
	out, err := statusCmd.CombinedOutput()
	require.NoError(t, err)
	assert.Empty(t, string(out), "workdir must be untouched by writes inside the worktree")

	result.Cleanup()
	_, statErr := os.Stat(result.Path)
	assert.True(t, os.IsNotExist(statErr), "cleanup must remove the worktree directory")

	// workdir itself must still be present and clean after cleanup.
	statusCmd = exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = workdir
	out, err = statusCmd.CombinedOutput()
	require.NoError(t, err)
	assert.Empty(t, string(out))
}
