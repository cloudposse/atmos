package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	t.Chdir(repoDir)

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

	t.Chdir(repoDir)

	_, err := MergeBase("nonexistent-branch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resolving")
}

// TestMergeBaseWithAutoFetch_RecoversFromMissingRef verifies that
// MergeBaseWithAutoFetch fetches the target branch and retries when
// origin/<target> is missing from the local repo (the common shallow-clone
// case that produced the customer-reported false-positives bug).
func TestMergeBaseWithAutoFetch_RecoversFromMissingRef(t *testing.T) {
	// Origin repo with main + feature branch diverging from a fork point.
	originDir := t.TempDir()
	runGit(t, originDir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "initial.txt"), []byte("initial"), 0o644))
	runGit(t, originDir, "add", "initial.txt")
	runGit(t, originDir, "commit", "-m", "fork point")
	// Capture the fork point SHA after commit.
	forkPointSHA := strings.TrimSpace(runGitOutput(t, originDir, "rev-parse", "HEAD"))

	// Branch off feature, add a commit.
	runGit(t, originDir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "feature.txt"), []byte("feature"), 0o644))
	runGit(t, originDir, "add", "feature.txt")
	runGit(t, originDir, "commit", "-m", "feature commit")

	// Advance main with another commit (simulates main moving forward
	// after the PR branch was created).
	runGit(t, originDir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "main.txt"), []byte("main"), 0o644))
	runGit(t, originDir, "add", "main.txt")
	runGit(t, originDir, "commit", "-m", "main moved forward")

	// Clone and check out feature, simulating the PR-branch checkout.
	cloneDir := t.TempDir()
	runGit(t, cloneDir, "clone", originDir, ".")
	runGit(t, cloneDir, "checkout", "-b", "feature", "origin/feature")

	// Remove origin/main locally to simulate a shallow CI checkout that
	// did not fetch the target branch.
	runGit(t, cloneDir, "update-ref", "-d", "refs/remotes/origin/main")

	t.Chdir(cloneDir)

	// Pure MergeBase fails with "ref not found".
	_, err := MergeBase("main")
	require.Error(t, err)

	// MergeBaseWithAutoFetch self-heals by fetching origin/main and retrying.
	sha, err := MergeBaseWithAutoFetch(cloneDir, "main")
	require.NoError(t, err, "auto-fetch should recover from missing origin/main")
	assert.Equal(t, forkPointSHA, sha, "merge-base after fetch should be the fork point, not main HEAD")
}

// TestMergeBaseWithAutoFetch_PropagatesHeadOnTargetBranch verifies that
// ErrHeadOnTargetBranch is returned without attempting any fetch — fetching
// cannot help that case.
func TestMergeBaseWithAutoFetch_PropagatesHeadOnTargetBranch(t *testing.T) {
	repoDir := t.TempDir()
	repo := initTestRepo(t, repoDir)
	commitFile(t, repo, repoDir, "initial.txt", "content", "initial commit")

	head, err := repo.Head()
	require.NoError(t, err)
	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/main", head.Hash()))
	require.NoError(t, err)

	t.Chdir(repoDir)

	_, err = MergeBaseWithAutoFetch(repoDir, "main")
	assert.ErrorIs(t, err, ErrHeadOnTargetBranch)
}

// TestMergeBaseWithAutoFetch_DeepenPathExhausted exercises the deepen branch
// of MergeBaseWithAutoFetch when the histories on both sides are fully
// walkable but share no common ancestor (orphan branches). The first
// MergeBase call returns ErrNoCommonAncestor, so the function attempts a
// deepen — which succeeds as a no-op on a non-shallow repo — and the retry
// MergeBase still fails with ErrNoCommonAncestor. We verify the function
// propagates the error rather than returning a bogus SHA, exercising the
// deepen branch end-to-end.
func TestMergeBaseWithAutoFetch_DeepenPathExhausted(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")

	// Real main with one commit, exposed as origin/main.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.txt"), []byte("M"), 0o644))
	runGit(t, dir, "add", "main.txt")
	runGit(t, dir, "commit", "-m", "main commit")
	mainSHA := strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "HEAD"))
	runGit(t, dir, "update-ref", "refs/remotes/origin/main", mainSHA)

	// Orphan branch with completely independent history — same walker, no
	// shared ancestor. This is what triggers ErrNoCommonAncestor (rather
	// than the shallow-clone "object not found" path).
	runGit(t, dir, "checkout", "--orphan", "feature")
	runGit(t, dir, "rm", "-rf", ".")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("F"), 0o644))
	runGit(t, dir, "add", "feature.txt")
	runGit(t, dir, "commit", "-m", "orphan feature commit")

	t.Chdir(dir)

	// Pure MergeBase returns ErrNoCommonAncestor because both histories are
	// walkable but truly unrelated.
	_, err := MergeBase("main")
	require.ErrorIs(t, err, ErrNoCommonAncestor)

	// MergeBaseWithAutoFetch hits the deepen branch (noAncestor=true), the
	// deepen succeeds (origin is a local file, no-op deepen), then the retry
	// MergeBase still returns ErrNoCommonAncestor and the function propagates
	// it. The deepen branch executes regardless of recovery success.
	_, err = MergeBaseWithAutoFetch(dir, "main")
	require.Error(t, err)
}

// TestMergeBaseWithAutoFetch_ReturnsErrorWhenFetchImpossible verifies that
// the function does not silently succeed when neither merge-base nor any
// fetch can recover (e.g., target branch does not exist on remote).
// The original MergeBase error is propagated so the caller can fall through.
func TestMergeBaseWithAutoFetch_ReturnsErrorWhenFetchImpossible(t *testing.T) {
	originDir := t.TempDir()
	runGit(t, originDir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "f.txt"), []byte("x"), 0o644))
	runGit(t, originDir, "add", "f.txt")
	runGit(t, originDir, "commit", "-m", "initial")

	cloneDir := t.TempDir()
	runGit(t, cloneDir, "clone", originDir, ".")

	t.Chdir(cloneDir)

	// "release" branch does not exist on origin; fetch will fail.
	_, err := MergeBaseWithAutoFetch(cloneDir, "release")
	assert.Error(t, err)
}

// runGitOutput runs a git command and returns its stdout as a string.
func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.Output()
	require.NoError(t, err, "git %v failed", args)
	return string(out)
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

	t.Chdir(repoDir)

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
