package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// GetOrCreateNamedSandbox returns an existing named sandbox or creates a new one.
// Named sandboxes are shared across tests and cleaned up by TestMain.
// Workdir must be an absolute path.
func getOrCreateNamedSandbox(t *testing.T, name string, workdir string) *testhelpers.SandboxEnvironment {
	sandboxMutex.Lock()
	defer sandboxMutex.Unlock()

	if env, exists := sandboxRegistry[name]; exists {
		t.Logf("Reusing existing sandbox %q", name)
		return env
	}

	t.Logf("Creating new sandbox %q", name)
	env, err := testhelpers.SetupSandbox(t, workdir)
	if err != nil {
		t.Fatalf("Failed to setup sandbox %q: %v", name, err)
	}
	sandboxRegistry[name] = env
	return env
}

// CreateIsolatedSandbox creates a new isolated sandbox for a single test.
// Not added to registry, caller must clean up.
// Workdir must be an absolute path.
func createIsolatedSandbox(t *testing.T, workdir string) *testhelpers.SandboxEnvironment {
	t.Logf("Creating isolated sandbox")
	env, err := testhelpers.SetupSandbox(t, workdir)
	if err != nil {
		t.Fatalf("Failed to setup isolated sandbox: %v", err)
	}
	return env
}

// cleanupSandboxes cleans up all registered sandboxes.
func cleanupSandboxes() {
	sandboxMutex.Lock()
	defer sandboxMutex.Unlock()

	for name, env := range sandboxRegistry {
		env.Cleanup()
		delete(sandboxRegistry, name)
	}
}

// Clean up untracked files in the working directory.
func cleanDirectory(t *testing.T, workdir string) error {
	// Find the root of the Git repository
	repoRoot, err := findGitRepoRoot(workdir)
	if err != nil {
		return fmt.Errorf("failed to locate git repository from %q: %w", workdir, err)
	}

	// Open the repository
	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get the worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Get the repository status
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get git status: %w", err)
	}

	// Clean only files in the provided working directory.
	// Use workdir + separator to avoid matching directories with a shared prefix
	// (e.g., "native-ci" should not match "native-ci-gha-plan").
	workdirPrefix := workdir + string(filepath.Separator)
	for file, statusEntry := range status {
		if statusEntry.Worktree == git.Untracked {
			fullPath := filepath.Join(repoRoot, file)
			if strings.HasPrefix(fullPath, workdirPrefix) || fullPath == workdir {
				t.Logf("Removing untracked file: %q", fullPath)
				if err := os.RemoveAll(fullPath); err != nil {
					return fmt.Errorf("failed to remove %q: %w", fullPath, err)
				}
			}
		}
	}

	return nil
}

// findGitRepoRoot finds the Git repository root.
func findGitRepoRoot(path string) (string, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", fmt.Errorf("failed to find git repository: %w", err)
	}

	// Get the repository's working tree
	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Return the absolute path to the root of the working tree
	root, err := filepath.Abs(worktree.Filesystem.Root())
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path of repository root: %w", err)
	}

	return root, nil
}
