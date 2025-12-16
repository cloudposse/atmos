package git

import (
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
	tests := []struct {
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

	for _, tt := range tests {
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
