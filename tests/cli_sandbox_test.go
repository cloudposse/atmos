package tests

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

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
// Each sandbox's Cleanup() is guarded with a recover() so that a panic in one
// entry does not prevent the remaining sandboxes from being cleaned up.
func cleanupSandboxes() {
	sandboxMutex.Lock()
	defer sandboxMutex.Unlock()

	for name, env := range sandboxRegistry {
		name, env := name, env // capture for closure
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic during sandbox cleanup", "name", name, "panic", r)
				}
			}()
			env.Cleanup()
		}()
		delete(sandboxRegistry, name)
	}
}

// Clean up untracked files in the working directory.
//
// Uses `git clean -fd` scoped to the workdir subtree, which is significantly faster than
// calling go-git's Worktree.Status() which scans the entire repository.
func cleanDirectory(t *testing.T, workdir string) error {
	// Find the root of the Git repository so we can compute a relative path.
	repoRoot, err := findGitRepoRoot(workdir)
	if err != nil {
		return fmt.Errorf("failed to locate git repository from %q: %w", workdir, err)
	}

	// Compute the path relative to the repo root so git clean only touches
	// files within the workdir subtree (not the entire repository).
	relWorkdir, err := filepath.Rel(repoRoot, workdir)
	if err != nil {
		return fmt.Errorf("failed to compute relative path from %q to %q: %w", repoRoot, workdir, err)
	}

	// -f: force, -d: recurse into untracked directories.
	// The pathspec `-- <relWorkdir>` limits the clean to the specified subtree only.
	// Note: the '-x' flag is deliberately omitted so .gitignore-matched files are preserved.
	cmd := exec.Command("git", "-C", repoRoot, "clean", "-fd", "--", relWorkdir) //nolint:gosec
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clean failed in %q: %w\n%s", workdir, err, out)
	}

	t.Logf("Cleaned untracked files in %q", workdir)
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
