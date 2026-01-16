package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// gitTestRepo holds common test repository setup.
type gitTestRepo struct {
	tmpDir     string
	configPath string
	processor  *Processor
}

// setupGitTestRepo creates a git repository with an initial commit and user modifications.
func setupGitTestRepo(t *testing.T, fileName, initialContent, userContent string) *gitTestRepo {
	t.Helper()

	tmpDir := t.TempDir()

	// Initialize git repository.
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	// Create initial file and commit (this is the "base").
	configPath := filepath.Join(tmpDir, fileName)
	err = os.WriteFile(configPath, []byte(initialContent), 0o644)
	require.NoError(t, err)

	_, err = worktree.Add(fileName)
	require.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	// User modifies the file.
	err = os.WriteFile(configPath, []byte(userContent), 0o644)
	require.NoError(t, err)

	// Create processor and setup git storage.
	processor := NewProcessor()
	err = processor.SetupGitStorage(tmpDir, "HEAD")
	require.NoError(t, err)

	return &gitTestRepo{
		tmpDir:     tmpDir,
		configPath: configPath,
		processor:  processor,
	}
}

// TestProcessorWithGitStorage tests the full update workflow with git storage.
func TestProcessorWithGitStorage(t *testing.T) {
	initialContent := "# Config\nversion: 1.0\nname: test\n"
	userContent := "# Config\nversion: 1.0\nname: test\n\n# User's custom section\ncustom: value\n"

	testRepo := setupGitTestRepo(t, "config.yaml", initialContent, userContent)

	// Simulate template update (new version adds a new section).
	templateFile := File{
		Path:        "config.yaml",
		Content:     "# Config\nversion: 2.0\nname: test\n\n# New feature from template\nfeature: enabled\n",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	// Process file in update mode.
	err := testRepo.processor.ProcessFile(templateFile, testRepo.tmpDir, false, true, nil, nil)
	require.NoError(t, err)

	// Read result.
	mergedContent, err := os.ReadFile(testRepo.configPath)
	require.NoError(t, err)

	merged := string(mergedContent)

	// Verify merge results.
	// Should have: new version (from template), user's custom section, and new feature.
	assert.Contains(t, merged, "version: 2.0", "Should have new version from template")
	assert.Contains(t, merged, "custom: value", "Should preserve user's custom section")
	assert.Contains(t, merged, "feature: enabled", "Should have new feature from template")
}

// TestProcessorWithGitStorage_UserAddedFile tests that user-added files are not touched.
func TestProcessorWithGitStorage_UserAddedFile(t *testing.T) {
	// Create a temporary directory for our git repo
	tmpDir := t.TempDir()

	// Initialize git repository
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	// Create and commit a README file (this exists in base)
	readmePath := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(readmePath, []byte("# Project\n"), 0o644)
	require.NoError(t, err)

	_, err = worktree.Add("README.md")
	require.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	// User adds a custom file (NOT in git base)
	customPath := filepath.Join(tmpDir, "custom.yaml")
	customContent := "# User's custom file\ncustom: true\n"
	err = os.WriteFile(customPath, []byte(customContent), 0o644)
	require.NoError(t, err)

	// Create processor and setup git storage
	processor := NewProcessor()
	err = processor.SetupGitStorage(tmpDir, "HEAD")
	require.NoError(t, err)

	// Template tries to create a file with same name
	templateFile := File{
		Path:        "custom.yaml",
		Content:     "# Template file\ntemplate: value\n",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	// Process file in update mode
	err = processor.ProcessFile(templateFile, tmpDir, false, true, nil, nil)
	// Should succeed (file is skipped, not an error)
	require.NoError(t, err)

	// Read result - should still be user's content
	resultContent, err := os.ReadFile(customPath)
	require.NoError(t, err)

	// Verify user's file was NOT modified
	assert.Equal(t, customContent, string(resultContent), "User's custom file should not be modified")
	assert.NotContains(t, string(resultContent), "template: value", "Should not contain template content")
}

// TestProcessorWithoutGitStorage tests that update mode requires git storage.
func TestProcessorWithoutGitStorage(t *testing.T) {
	// Create a temporary directory (NOT a git repo)
	tmpDir := t.TempDir()

	// Create existing file
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("version: 1.0\n"), 0o644)
	require.NoError(t, err)

	// Create processor (git storage setup will silently fail, that's OK)
	processor := NewProcessor()
	err = processor.SetupGitStorage(tmpDir, "main")
	// Should NOT error - just won't have git storage
	require.NoError(t, err)

	// Template file
	templateFile := File{
		Path:        "config.yaml",
		Content:     "version: 2.0\n",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	// Process file in update mode should fail without git storage.
	// This is a security/correctness measure: without git, we can't compute a meaningful
	// 3-way merge base, so the merge would be a no-op.
	err = processor.ProcessFile(templateFile, tmpDir, false, true, nil, nil)
	require.Error(t, err, "Update mode should require git storage")
	assert.ErrorIs(t, err, errUtils.ErrThreeWayMerge, "Should return ErrThreeWayMerge")
}

// TestProcessorWithGitStorage_TemplateFile tests merging with template processing (IsTemplate=true).
func TestProcessorWithGitStorage_TemplateFile(t *testing.T) {
	initialContent := "# Config\nversion: 1.0\n"
	userContent := "# Config\nversion: 1.0\ncustom: user-value\n"

	testRepo := setupGitTestRepo(t, "config.yaml", initialContent, userContent)

	// Template file with IsTemplate=true.
	// Using simple Go template syntax that doesn't require variables.
	templateFile := File{
		Path:        "config.yaml",
		Content:     "# Config\nversion: 2.0\nfeature: enabled\n",
		IsTemplate:  true, // This will trigger template processing code path.
		Permissions: 0o644,
	}

	// Process file in update mode.
	err := testRepo.processor.ProcessFile(templateFile, testRepo.tmpDir, false, true, nil, nil)
	require.NoError(t, err)

	// Read result.
	mergedContent, err := os.ReadFile(testRepo.configPath)
	require.NoError(t, err)

	merged := string(mergedContent)

	// Verify merge results.
	assert.Contains(t, merged, "version: 2.0", "Should have new version from template")
	assert.Contains(t, merged, "custom: user-value", "Should preserve user's custom value")
	assert.Contains(t, merged, "feature: enabled", "Should have new feature from template")
}

// TestProcessorWithGitStorage_MergeConflict tests conflict detection.
func TestProcessorWithGitStorage_MergeConflict(t *testing.T) {
	initialContent := "setting: original\n"
	userContent := "setting: user-change\n"

	testRepo := setupGitTestRepo(t, "config.yaml", initialContent, userContent)

	// Template also modifies the same setting (conflict!).
	templateFile := File{
		Path:        "config.yaml",
		Content:     "setting: template-change\n",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	// Process file in update mode - should detect conflict.
	err := testRepo.processor.ProcessFile(templateFile, testRepo.tmpDir, false, true, nil, nil)

	// Should error due to conflict or merge failure.
	assert.Error(t, err)
	// The error could be either "merge conflict" (if conflicts detected after merge)
	// or "three-way merge failed" (if merge fails during execution).
	errorMsg := err.Error()
	assert.True(t,
		strings.Contains(errorMsg, "merge conflict") || strings.Contains(errorMsg, "three-way merge failed"),
		"Error should mention merge conflict or three-way merge failure, got: %s", errorMsg)
}
