package git

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
)

// TestDefaultRepositoryOperations_GetLocalRepo verifies that DefaultRepositoryOperations delegates to GetLocalRepo.
func TestDefaultRepositoryOperations_GetLocalRepo(t *testing.T) {
	// Find the git repo root by walking up from current directory.
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	wd := originalWd
	for {
		if _, err := os.Stat(filepath.Join(wd, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(wd)
		if parent == wd || parent == "/" || parent == "." {
			t.Skipf("Not in a git repository, skipping test")
		}
		wd = parent
	}

	// Change to repo root.
	t.Chdir(wd)

	ops := &DefaultRepositoryOperations{}

	// GetLocalRepo should succeed when in a git repository.
	repo, err := ops.GetLocalRepo()
	assert.NoError(t, err)
	assert.NotNil(t, repo)
}

// TestDefaultRepositoryOperations_GetRepoInfo verifies that DefaultRepositoryOperations delegates to GetRepoInfo.
func TestDefaultRepositoryOperations_GetRepoInfo(t *testing.T) {
	// Create a test repo in memory.
	tempDir := t.TempDir()
	repo, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("Failed to create test repo: %v", err)
	}

	ops := &DefaultRepositoryOperations{}

	// GetRepoInfo should handle a repo without remotes gracefully.
	info, err := ops.GetRepoInfo(repo)

	// Should succeed even without remotes.
	assert.NoError(t, err)

	// Without remotes, GetRepoInfo returns an empty RepoInfo struct.
	// This verifies the function handles the edge case without panicking.
	assert.Empty(t, info.RepoUrl)
	assert.Empty(t, info.RepoOwner)
	assert.Empty(t, info.RepoName)
	assert.Empty(t, info.RepoHost)
	assert.Empty(t, info.LocalRepoPath)
	assert.Empty(t, info.LocalWorktreePath)
	assert.Nil(t, info.LocalWorktree)
}
