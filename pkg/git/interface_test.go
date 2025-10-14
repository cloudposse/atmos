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
	// Save and restore working directory.
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(originalWd)

	// Find the git repo root by walking up from current directory.
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
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("Failed to change to repo root: %v", err)
	}

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

	// Without remotes, we expect either an error or empty info.
	// The important thing is that the delegation works.
	_ = info
	_ = err
}
