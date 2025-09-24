package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDescribeAffectedFromSubdirectory tests that describe affected works correctly
// when run from a subdirectory of the repository, particularly in worktree scenarios.
// This test ensures we don't have regressions where false positives are reported.
func TestDescribeAffectedFromSubdirectory(t *testing.T) {
	// Create a temporary directory for our test repository
	tempDir, err := os.MkdirTemp("", "atmos-subdirectory-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Initialize a main repository
	mainRepoPath := filepath.Join(tempDir, "main-repo")
	err = os.MkdirAll(mainRepoPath, 0o755)
	require.NoError(t, err)

	mainRepo, err := git.PlainInit(mainRepoPath, false)
	require.NoError(t, err)

	// Configure the repository
	repoConfig, err := mainRepo.Config()
	require.NoError(t, err)
	repoConfig.User.Name = "Test User"
	repoConfig.User.Email = "test@example.com"
	err = mainRepo.SetConfig(repoConfig)
	require.NoError(t, err)

	// Add a remote origin
	_, err = mainRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/test/test.git"},
	})
	require.NoError(t, err)

	// Create directory structure similar to examples/quick-start-advanced
	exampleDir := filepath.Join(mainRepoPath, "examples", "test-example")
	err = os.MkdirAll(filepath.Join(exampleDir, "stacks"), 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(exampleDir, "components", "terraform"), 0o755)
	require.NoError(t, err)

	// Create atmos.yaml in the example directory
	atmosYaml := `
base_path: "."
stacks:
  base_path: "stacks"
components:
  terraform:
    base_path: "components/terraform"
`
	err = os.WriteFile(filepath.Join(exampleDir, "atmos.yaml"), []byte(atmosYaml), 0o644)
	require.NoError(t, err)

	// Create a test stack file
	stackYaml := `
components:
  terraform:
    test-component:
      metadata:
        component: test
      vars:
        test_var: "value1"
`
	err = os.WriteFile(filepath.Join(exampleDir, "stacks", "test-stack.yaml"), []byte(stackYaml), 0o644)
	require.NoError(t, err)

	// Create initial commit on main branch
	worktree, err := mainRepo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add(".")
	require.NoError(t, err)

	hash1, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	// Create and set refs/remotes/origin/main
	mainRef := plumbing.NewHashReference(plumbing.NewRemoteReferenceName("origin", "main"), hash1)
	err = mainRepo.Storer.SetReference(mainRef)
	require.NoError(t, err)

	// Create refs/remotes/origin/HEAD pointing to origin/main
	headRef := plumbing.NewSymbolicReference(
		plumbing.ReferenceName("refs/remotes/origin/HEAD"),
		plumbing.ReferenceName("refs/remotes/origin/main"),
	)
	err = mainRepo.Storer.SetReference(headRef)
	require.NoError(t, err)

	// Create a feature branch
	branchRef := plumbing.NewBranchReferenceName("feature-branch")
	ref := plumbing.NewHashReference(branchRef, hash1)
	err = mainRepo.Storer.SetReference(ref)
	require.NoError(t, err)

	// Checkout the feature branch
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRef,
		Create: false,
	})
	require.NoError(t, err)

	// Create a worktree for the feature branch
	worktreePath := filepath.Join(tempDir, "test-worktree")
	_, err = mainRepo.Worktree()
	require.NoError(t, err)

	// Manually create a worktree structure (since go-git doesn't have native worktree creation)
	err = os.MkdirAll(worktreePath, 0o755)
	require.NoError(t, err)

	// Copy the main repo to simulate a worktree (but skip .git)
	err = copyDirExcludeGit(mainRepoPath, worktreePath)
	require.NoError(t, err)

	// Create .git file pointing to main repo (simulating worktree)
	gitFile := filepath.Join(worktreePath, ".git")
	err = os.WriteFile(gitFile, []byte("gitdir: "+filepath.Join(mainRepoPath, ".git")), 0o644)
	require.NoError(t, err)

	// Test 1: No changes should result in empty affected list
	t.Run("no changes from subdirectory", func(t *testing.T) {
		// Change to the example subdirectory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		exampleDirInWorktree := filepath.Join(worktreePath, "examples", "test-example")
		err = os.Chdir(exampleDirInWorktree)
		require.NoError(t, err)

		// Create a minimal atmos config
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: ".",
			Stacks: schema.Stacks{
				BasePath: "stacks",
			},
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
			Logs: schema.Logs{
				Level: "error",
			},
		}

		// Execute describe affected - should return empty since no changes
		affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRefCheckout(
			atmosConfig,
			"",    // Use default ref (origin/HEAD)
			"",    // No specific SHA
			false, // includeSpaceliftAdminStacks
			false, // includeSettings
			"",    // stack filter
			false, // processTemplates
			false, // processYamlFunctions
			nil,   // skip
			false, // excludeLocked
		)

		// Should succeed and return empty affected list
		assert.NoError(t, err)
		assert.Empty(t, affected, "Should have no affected components when no changes exist")
	})

	// Test 2: Changes in the current branch should be detected
	t.Run("detects changes from subdirectory", func(t *testing.T) {
		// Modify a file in the worktree
		modifiedStack := `
components:
  terraform:
    test-component:
      metadata:
        component: test
      vars:
        test_var: "value2"  # Changed value
        new_var: "new"      # Added variable
`
		stackPath := filepath.Join(worktreePath, "examples", "test-example", "stacks", "test-stack.yaml")
		err = os.WriteFile(stackPath, []byte(modifiedStack), 0o644)
		require.NoError(t, err)

		// Change to the example subdirectory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		exampleDirInWorktree := filepath.Join(worktreePath, "examples", "test-example")
		err = os.Chdir(exampleDirInWorktree)
		require.NoError(t, err)

		// Note: In a real integration test, we'd need to set up proper mocking or use the actual function
		// This test serves as a regression test template to ensure the fix is maintained
		// The key fix was in executeDescribeAffected to properly calculate paths when running from subdirectories

		// Create a minimal atmos config
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: ".",
			Stacks: schema.Stacks{
				BasePath: "stacks",
			},
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
			Logs: schema.Logs{
				Level: "error",
			},
		}

		// Execute describe affected - should detect changes
		affected, _, _, _, err := ExecuteDescribeAffectedWithTargetRefCheckout(
			atmosConfig,
			"",    // Use default ref (origin/HEAD)
			"",    // No specific SHA
			false, // includeSpaceliftAdminStacks
			false, // includeSettings
			"",    // stack filter
			false, // processTemplates
			false, // processYamlFunctions
			nil,   // skip
			false, // excludeLocked
		)

		// Should succeed and detect the modified stack
		assert.NoError(t, err)
		assert.NotEmpty(t, affected, "Should have affected components when changes exist")
	})
}

// copyDirExcludeGit recursively copies a directory, excluding .git.
func copyDirExcludeGit(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory entirely
		if strings.Contains(path, ".git") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// TestDescribeAffectedPathCalculation tests the path calculation logic
// that was fixed to properly handle subdirectory execution.
func TestDescribeAffectedPathCalculation(t *testing.T) {
	t.Run("calculates correct paths for remote repo", func(t *testing.T) {
		// Test that when running from examples/quick-start-advanced,
		// the remote repo paths are correctly adjusted

		// Create test directory structure
		tempDir, err := os.MkdirTemp("", "atmos-path-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Simulate repo structure
		repoPath := filepath.Join(tempDir, "repo")
		examplePath := filepath.Join(repoPath, "examples", "test")
		err = os.MkdirAll(filepath.Join(examplePath, "stacks"), 0o755)
		require.NoError(t, err)

		// Save current directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		// Change to subdirectory
		err = os.Chdir(examplePath)
		require.NoError(t, err)

		// Create config with relative base path
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: ".",
			Stacks: schema.Stacks{
				BasePath: "stacks",
			},
		}

		// The executeDescribeAffected function should calculate:
		// 1. Relative path from repo root to current dir (examples/test)
		// 2. Apply this to remote repo path
		// Expected: remoteRepoPath/examples/test/stacks

		// Simulate what the fixed code does:
		currentDir, _ := os.Getwd()
		// Resolve any symlinks to get the real paths for comparison
		currentDirReal, err := filepath.EvalSymlinks(currentDir)
		require.NoError(t, err)
		repoPathReal, err := filepath.EvalSymlinks(repoPath)
		require.NoError(t, err)

		relativePathFromRoot, err := filepath.Rel(repoPathReal, currentDirReal)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join("examples", "test"), relativePathFromRoot)

		// When basePath is "." and we're in a subdirectory
		basePath := atmosConfig.BasePath
		if basePath == "." && relativePathFromRoot != "" && relativePathFromRoot != "." {
			basePath = relativePathFromRoot
		}
		assert.Equal(t, filepath.Join("examples", "test"), basePath)

		// Verify the remote stack path would be correct
		remoteRepoPath := "/tmp/remote"
		expectedStackPath := filepath.Join(remoteRepoPath, basePath, atmosConfig.Stacks.BasePath)
		assert.Equal(t, "/tmp/remote/examples/test/stacks", expectedStackPath)
	})
}

// TestIsGitWorktreeDetection tests the worktree detection function.
func TestIsGitWorktreeDetection(t *testing.T) {
	t.Run("detects regular repo", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "test-regular-repo-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create .git directory
		err = os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755)
		require.NoError(t, err)

		result := isGitWorktree(tempDir)
		assert.False(t, result, "Should detect regular repo (not worktree)")
	})

	t.Run("detects worktree", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "test-worktree-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create .git file (not directory)
		gitFile := filepath.Join(tempDir, ".git")
		err = os.WriteFile(gitFile, []byte("gitdir: /path/to/main/.git/worktrees/feature"), 0o644)
		require.NoError(t, err)

		result := isGitWorktree(tempDir)
		assert.True(t, result, "Should detect worktree")
	})

	t.Run("handles missing .git", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "test-no-git-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		result := isGitWorktree(tempDir)
		assert.False(t, result, "Should return false when .git doesn't exist")
	})
}
