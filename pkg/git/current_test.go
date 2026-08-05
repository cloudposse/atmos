package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentGitMetadata(t *testing.T) {
	repoDir, expectedSHA := initCurrentTestGitRepo(t, "feature/test")
	t.Chdir(repoDir)
	expectedRoot, err := filepath.EvalSymlinks(repoDir)
	require.NoError(t, err)

	sha, err := GetCurrentCommitSHA()
	require.NoError(t, err)
	assert.Equal(t, expectedSHA, sha)

	branch, err := GetCurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "feature/test", branch)

	root, err := GetRoot()
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, root)

	tagRoot, err := ProcessTagRoot(YAMLFuncRoot)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, tagRoot)

	legacyTagRoot, err := ProcessTagRoot(YAMLFuncRepoRoot)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, legacyTagRoot)

	tagSHA, err := ProcessTagSHA(YAMLFuncSHA)
	require.NoError(t, err)
	assert.Equal(t, expectedSHA, tagSHA)

	tagRef, err := ProcessTagRef(YAMLFuncRef)
	require.NoError(t, err)
	assert.Equal(t, expectedSHA, tagRef)

	tagBranch, err := ProcessTagBranch(YAMLFuncBranch)
	require.NoError(t, err)
	assert.Equal(t, "feature/test", tagBranch)
}

func TestGetCurrentBranchDetachedHead(t *testing.T) {
	repoDir, expectedSHA := initCurrentTestGitRepo(t, "main")
	t.Chdir(repoDir)

	repo, err := gogit.PlainOpen(repoDir)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, worktree.Checkout(&gogit.CheckoutOptions{
		Hash: plumbing.NewHash(expectedSHA),
	}))

	_, err = GetCurrentBranch()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "detached")

	branch, err := ProcessTagBranch(YAMLFuncBranch + " detached")
	require.NoError(t, err)
	assert.Equal(t, "detached", branch)
}

func TestGitTagFallbacksOutsideRepository(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	sha, err := ProcessTagSHA(YAMLFuncSHA + " unknown")
	require.NoError(t, err)
	assert.Equal(t, "unknown", sha)

	ref, err := ProcessTagRef(YAMLFuncRef + " unknown")
	require.NoError(t, err)
	assert.Equal(t, "unknown", ref)

	branch, err := ProcessTagBranch(YAMLFuncBranch + " detached")
	require.NoError(t, err)
	assert.Equal(t, "detached", branch)

	fallbackRoot := filepath.Join("fallback", "root")
	root, err := ProcessTagRoot(YAMLFuncRoot + " " + fallbackRoot)
	require.NoError(t, err)
	assert.Equal(t, fallbackRoot, root)
}

func initCurrentTestGitRepo(t *testing.T, branch string) (string, string) {
	t.Helper()

	repoDir := t.TempDir()
	repo, err := gogit.PlainInit(repoDir, false)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	filePath := filepath.Join(repoDir, "README.md")
	require.NoError(t, os.WriteFile(filePath, []byte("test\n"), 0o644))

	_, err = worktree.Add("README.md")
	require.NoError(t, err)

	hash, err := worktree.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Atmos Test",
			Email: "test@example.com",
			When:  time.Unix(1, 0),
		},
	})
	require.NoError(t, err)

	if branch != "" && branch != "master" {
		require.NoError(t, worktree.Checkout(&gogit.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName(branch),
			Create: true,
		}))
	}

	return repoDir, hash.String()
}
