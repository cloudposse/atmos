package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeBase_FindsForkPoint creates a git repo with two branches diverging
// from a common ancestor and verifies MergeBase returns the fork point.
func TestMergeBase_FindsForkPoint(t *testing.T) {
	repoDir := t.TempDir()
	repo := initTestRepo(t, repoDir)

	// Create initial commit on main.
	forkPointHash := commitFile(t, repo, repoDir, "initial.txt", "initial content", "initial commit")

	// Create a feature branch from the fork point.
	err := repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/feature", forkPointHash))
	require.NoError(t, err)

	// Add a commit to main (simulates target branch moving forward).
	commitFile(t, repo, repoDir, "main-change.txt", "main content", "main commit")

	// Create origin/main remote ref pointing to current main HEAD.
	mainHead, err := repo.Head()
	require.NoError(t, err)
	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/main", mainHead.Hash()))
	require.NoError(t, err)

	// Switch to the feature branch and add a commit.
	wt, err := repo.Worktree()
	require.NoError(t, err)
	err = wt.Checkout(&gogit.CheckoutOptions{Branch: "refs/heads/feature"})
	require.NoError(t, err)
	commitFile(t, repo, repoDir, "feature-change.txt", "feature content", "feature commit")

	// Change to the repo directory so MergeBase can find it.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	err = os.Chdir(repoDir)
	require.NoError(t, err)

	// MergeBase should return the fork point.
	sha, err := MergeBase("main")
	require.NoError(t, err)
	assert.Equal(t, forkPointHash.String(), sha)
}

// TestMergeBase_ErrorWhenTargetRefMissing verifies that MergeBase returns an error
// when the target branch ref doesn't exist (e.g., shallow checkout).
func TestMergeBase_ErrorWhenTargetRefMissing(t *testing.T) {
	repoDir := t.TempDir()
	repo := initTestRepo(t, repoDir)
	commitFile(t, repo, repoDir, "initial.txt", "content", "initial commit")

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	err = os.Chdir(repoDir)
	require.NoError(t, err)

	_, err = MergeBase("nonexistent-branch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resolving")
}

// TestMergeBase_ErrorWhenHeadOnTargetBranch verifies that MergeBase returns
// ErrHeadOnTargetBranch when HEAD is on the target branch (e.g., merge commit checkout).
func TestMergeBase_ErrorWhenHeadOnTargetBranch(t *testing.T) {
	repoDir := t.TempDir()
	repo := initTestRepo(t, repoDir)
	commitFile(t, repo, repoDir, "initial.txt", "content", "initial commit")

	// Create origin/main pointing to the same commit as HEAD.
	head, err := repo.Head()
	require.NoError(t, err)
	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/main", head.Hash()))
	require.NoError(t, err)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	err = os.Chdir(repoDir)
	require.NoError(t, err)

	_, err = MergeBase("main")
	assert.ErrorIs(t, err, ErrHeadOnTargetBranch)
}

// initTestRepo creates a bare-minimum git repo with an initial config.
func initTestRepo(t *testing.T, dir string) *gogit.Repository {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	// Set up minimal config so commits work.
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test"
	cfg.User.Email = "test@example.com"
	cfg.Remotes["origin"] = &config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/example/repo.git"},
	}
	err = repo.SetConfig(cfg)
	require.NoError(t, err)

	return repo
}

// commitFile creates a file in the repo and commits it.
func commitFile(t *testing.T, repo *gogit.Repository, dir, name, content, msg string) plumbing.Hash {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(name)
	require.NoError(t, err)

	hash, err := wt.Commit(msg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	return hash
}
