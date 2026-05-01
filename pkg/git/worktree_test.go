package git

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/tests"
)

func TestGetWorktreeParentDir(t *testing.T) {
	testCases := []struct {
		name         string
		worktreePath string
		expected     string
	}{
		{
			name:         "path with worktree suffix",
			worktreePath: "/tmp/atmos-worktree-123/worktree",
			expected:     "/tmp/atmos-worktree-123",
		},
		{
			name:         "path without suffix returns path unchanged",
			worktreePath: "/tmp/some-other-path",
			expected:     "/tmp/some-other-path",
		},
		{
			name:         "empty string returns empty string",
			worktreePath: "",
			expected:     "",
		},
		{
			name:         "worktree at root",
			worktreePath: "/worktree",
			expected:     "",
		},
		{
			name:         "worktree in nested path",
			worktreePath: "/var/tmp/nested/atmos-worktree-abc/worktree",
			expected:     "/var/tmp/nested/atmos-worktree-abc",
		},
		{
			name:         "path ending with worktree but not as suffix",
			worktreePath: "/tmp/myworktree",
			expected:     "/tmp/myworktree", // Not a proper suffix
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// On Windows, path separators are different.
			worktreePath := tt.worktreePath
			expected := tt.expected
			if runtime.GOOS == "windows" {
				worktreePath = strings.ReplaceAll(worktreePath, "/", "\\")
				expected = strings.ReplaceAll(expected, "/", "\\")
			}

			result := GetWorktreeParentDir(worktreePath)
			assert.Equal(t, expected, result)
		})
	}
}

func TestGetWorktreeParentDir_CrossPlatform(t *testing.T) {
	// Test using filepath.Join for proper cross-platform path handling.
	t.Run("platform-native path", func(t *testing.T) {
		parentDir := filepath.Join(os.TempDir(), "atmos-worktree-test")
		worktreePath := filepath.Join(parentDir, worktreeSubdir)

		result := GetWorktreeParentDir(worktreePath)
		assert.Equal(t, parentDir, result)
	})
}

func TestCreateWorktree(t *testing.T) {
	// Skip if git is not available or not configured.
	tests.RequireGitCommitConfig(t)

	t.Run("creates worktree with valid commit", func(t *testing.T) {
		// Create a temporary git repository.
		repoDir := t.TempDir()
		repo, err := git.PlainInit(repoDir, false)
		require.NoError(t, err)

		// Create an initial commit.
		worktree, err := repo.Worktree()
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		_, err = worktree.Add("test.txt")
		require.NoError(t, err)

		commitHash, err := worktree.Commit("Initial commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
			},
		})
		require.NoError(t, err)

		// Create worktree using the commit SHA.
		worktreePath, err := CreateWorktree(repoDir, commitHash.String())
		require.NoError(t, err)
		assert.NotEmpty(t, worktreePath)

		// Verify the worktree was created.
		_, err = os.Stat(worktreePath)
		assert.NoError(t, err)

		// Verify it contains the expected files.
		testFileInWorktree := filepath.Join(worktreePath, "test.txt")
		_, err = os.Stat(testFileInWorktree)
		assert.NoError(t, err)

		// Clean up.
		parentDir := GetWorktreeParentDir(worktreePath)
		RemoveWorktree(repoDir, worktreePath)
		os.RemoveAll(parentDir)
	})

	t.Run("creates worktree with branch name", func(t *testing.T) {
		// Create a temporary git repository.
		repoDir := t.TempDir()
		repo, err := git.PlainInit(repoDir, false)
		require.NoError(t, err)

		// Create an initial commit on master/main.
		worktree, err := repo.Worktree()
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		_, err = worktree.Add("test.txt")
		require.NoError(t, err)

		_, err = worktree.Commit("Initial commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
			},
		})
		require.NoError(t, err)

		// Get the default branch name.
		head, err := repo.Head()
		require.NoError(t, err)

		// Create worktree using the branch name.
		worktreePath, err := CreateWorktree(repoDir, head.Name().Short())
		require.NoError(t, err)
		assert.NotEmpty(t, worktreePath)

		// Clean up.
		parentDir := GetWorktreeParentDir(worktreePath)
		RemoveWorktree(repoDir, worktreePath)
		os.RemoveAll(parentDir)
	})

	t.Run("fails with invalid ref", func(t *testing.T) {
		// Create a temporary git repository.
		repoDir := t.TempDir()
		repo, err := git.PlainInit(repoDir, false)
		require.NoError(t, err)

		// Create an initial commit.
		worktree, err := repo.Worktree()
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		_, err = worktree.Add("test.txt")
		require.NoError(t, err)

		_, err = worktree.Commit("Initial commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
			},
		})
		require.NoError(t, err)

		// Try to create worktree with an invalid ref.
		_, err = CreateWorktree(repoDir, "nonexistent-ref-12345")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrGitRefNotFound)
	})

	t.Run("fails with empty repo", func(t *testing.T) {
		// Create an empty git repository (no commits).
		repoDir := t.TempDir()
		_, err := git.PlainInit(repoDir, false)
		require.NoError(t, err)

		// Try to create worktree with HEAD (which doesn't exist yet).
		_, err = CreateWorktree(repoDir, "HEAD")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrGitRefNotFound)
	})
}

func TestRemoveWorktree(t *testing.T) {
	// Skip if git is not available or not configured.
	tests.RequireGitCommitConfig(t)

	t.Run("removes existing worktree", func(t *testing.T) {
		// Create a temporary git repository.
		repoDir := t.TempDir()
		repo, err := git.PlainInit(repoDir, false)
		require.NoError(t, err)

		// Create an initial commit.
		worktree, err := repo.Worktree()
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		_, err = worktree.Add("test.txt")
		require.NoError(t, err)

		commitHash, err := worktree.Commit("Initial commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
			},
		})
		require.NoError(t, err)

		// Create worktree.
		worktreePath, err := CreateWorktree(repoDir, commitHash.String())
		require.NoError(t, err)

		// Verify it exists.
		_, err = os.Stat(worktreePath)
		require.NoError(t, err)

		// Remove the worktree.
		RemoveWorktree(repoDir, worktreePath)

		// Verify it's removed.
		_, err = os.Stat(worktreePath)
		assert.True(t, os.IsNotExist(err))

		// Clean up parent dir.
		parentDir := GetWorktreeParentDir(worktreePath)
		os.RemoveAll(parentDir)
	})

	t.Run("handles non-existent worktree gracefully", func(t *testing.T) {
		// Create a temporary git repository.
		repoDir := t.TempDir()
		_, err := git.PlainInit(repoDir, false)
		require.NoError(t, err)

		// Try to remove a worktree that doesn't exist.
		// This should not panic, just log a warning.
		RemoveWorktree(repoDir, "/nonexistent/worktree/path")
		// If we get here without panicking, the test passes.
	})

	t.Run("handles empty paths gracefully", func(t *testing.T) {
		// Try to remove with empty paths.
		// This should not panic.
		RemoveWorktree("", "")
		// If we get here without panicking, the test passes.
	})
}

func TestWorktreeSubdirConstant(t *testing.T) {
	t.Run("worktreeSubdir has expected value", func(t *testing.T) {
		assert.Equal(t, "worktree", worktreeSubdir)
	})
}

// TestCreateWorktreeWithFetchRecovery_SuccessNoFetchNeeded verifies that
// when CreateWorktree succeeds on the first attempt, the helper returns
// without performing any fetch — even if a targetBranch is supplied.
func TestCreateWorktreeWithFetchRecovery_SuccessNoFetchNeeded(t *testing.T) {
	tests.RequireGitCommitConfig(t)

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("x"), 0o644))
	runGit(t, repoDir, "add", "f.txt")
	runGit(t, repoDir, "commit", "-m", "initial")

	commitSHA := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "HEAD"))

	// Pass a non-empty targetBranch — but since the first CreateWorktree
	// call succeeds, the fetch path must not run.
	worktreePath, err := CreateWorktreeWithFetchRecovery(repoDir, commitSHA, "main")
	require.NoError(t, err)
	require.NotEmpty(t, worktreePath)

	// Cleanup.
	parentDir := GetWorktreeParentDir(worktreePath)
	RemoveWorktree(repoDir, worktreePath)
	require.NoError(t, os.RemoveAll(parentDir))
}

// TestCreateWorktreeWithFetchRecovery_SuccessAfterFetch is the customer-bug
// scenario: the worktree's target SHA exists on origin's <targetBranch>
// but the clone hasn't fetched it yet (e.g., the PR's base SHA from the
// event payload is newer than the local origin/<target> tracking ref).
// The helper must fetch origin/<targetBranch> and retry, succeeding on
// the second attempt.
func TestCreateWorktreeWithFetchRecovery_SuccessAfterFetch(t *testing.T) {
	tests.RequireGitCommitConfig(t)

	originDir := t.TempDir()
	runGit(t, originDir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "f1.txt"), []byte("x"), 0o644))
	runGit(t, originDir, "add", "f1.txt")
	runGit(t, originDir, "commit", "-m", "main commit 1")

	// Clone origin BEFORE the second commit lands. The clone tracks
	// commit 1 as origin/main; commit 2 will be added after the clone
	// and is what we'll try to check out as the worktree target.
	cloneDir := t.TempDir()
	originURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(originDir)}).String()
	runGit(t, cloneDir, "clone", originURL, ".")

	// New commit on origin/main — this SHA exists on the remote but is
	// NOT in the clone's local object DB until we fetch.
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "f2.txt"), []byte("y"), 0o644))
	runGit(t, originDir, "add", "f2.txt")
	runGit(t, originDir, "commit", "-m", "main commit 2 (post-clone)")
	missingSHA := strings.TrimSpace(runGitOutput(t, originDir, "rev-parse", "HEAD"))

	// First attempt without a targetBranch: helper has no recovery option
	// and must propagate the worktree-creation error.
	_, err := CreateWorktreeWithFetchRecovery(cloneDir, missingSHA, "")
	require.Error(t, err, "expected error when commit is missing and no targetBranch supplied")

	// Second attempt with targetBranch: the helper fetches origin/main
	// (pulls in missingSHA) and the retry succeeds.
	worktreePath, err := CreateWorktreeWithFetchRecovery(cloneDir, missingSHA, "main")
	require.NoError(t, err, "auto-fetch should pull in missingSHA and the retry should succeed")
	require.NotEmpty(t, worktreePath)

	// Cleanup.
	parentDir := GetWorktreeParentDir(worktreePath)
	RemoveWorktree(cloneDir, worktreePath)
	require.NoError(t, os.RemoveAll(parentDir))
}

// TestCreateWorktreeWithFetchRecovery_FailsWhenFetchFails verifies that
// if the helper attempts a fetch and the fetch itself fails (e.g., the
// targetBranch does not exist on the remote), both the original
// CreateWorktree error and the fetch error are joined and returned.
func TestCreateWorktreeWithFetchRecovery_FailsWhenFetchFails(t *testing.T) {
	tests.RequireGitCommitConfig(t)

	originDir := t.TempDir()
	runGit(t, originDir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "f.txt"), []byte("x"), 0o644))
	runGit(t, originDir, "add", "f.txt")
	runGit(t, originDir, "commit", "-m", "initial")

	cloneDir := t.TempDir()
	runGit(t, cloneDir, "clone", originDir, ".")

	// Fake SHA that doesn't exist anywhere; targetBranch also doesn't exist.
	bogusSHA := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	_, err := CreateWorktreeWithFetchRecovery(cloneDir, bogusSHA, "no-such-branch-on-origin")
	require.Error(t, err)
	// Both the worktree creation and the fetch failures should be in the
	// joined error chain — verify the fetch sentinel is reachable.
	assert.ErrorIs(t, err, errUtils.ErrFetchOrigin)
}

// TestCreateWorktreeWithFetchRecovery_GateSkipsNonRefNotFoundError is the
// negative counterpart to the recovery tests: when CreateWorktree fails with
// an error that is NOT ErrGitRefNotFound (e.g., temp-dir creation failure
// from a missing TMPDIR), the helper must propagate the original error
// directly without attempting any fetch.
//
// We force os.MkdirTemp to fail by pointing TMPDIR / TEMP / TMP at a
// non-existent path. The infrastructure failure travels back un-wrapped
// (CreateWorktree returns the raw fs error before reaching the git step),
// so neither ErrGitRefNotFound nor ErrFetchOrigin should appear in the
// returned chain.
func TestCreateWorktreeWithFetchRecovery_GateSkipsNonRefNotFoundError(t *testing.T) {
	// Pre-compute a real temp dir path so t.TempDir / require.NoError
	// machinery still works, then poison the env for the call under test.
	bogusTmp := filepath.Join(t.TempDir(), "this-subdir-does-not-exist")
	// Sanity: this directory must NOT exist for MkdirTemp to fail.
	_, statErr := os.Stat(bogusTmp)
	require.True(t, os.IsNotExist(statErr), "test setup: bogusTmp must not exist")

	t.Setenv("TMPDIR", bogusTmp)
	t.Setenv("TEMP", bogusTmp)
	t.Setenv("TMP", bogusTmp)

	// Any non-empty values — they don't matter because CreateWorktree
	// fails at MkdirTemp before any git operation.
	_, err := CreateWorktreeWithFetchRecovery(t.TempDir(), "deadbeef", "main")
	require.Error(t, err)

	// Gate guarantee: the fetch path must NOT have run. If it had, the
	// joined chain would expose ErrFetchOrigin (since "main" cannot be
	// fetched from a non-repo). Both gate-disqualifying sentinels must
	// be absent from the returned chain.
	assert.NotErrorIs(t, err, errUtils.ErrFetchOrigin, "gate should skip fetch path for non-ErrGitRefNotFound errors")
	assert.NotErrorIs(t, err, errUtils.ErrGitRefNotFound, "this is an infrastructure failure, not a missing-ref failure")
}
