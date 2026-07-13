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
	"github.com/cloudposse/atmos/pkg/generator/storage"
)

// gitTestRepo holds common test repository setup.
type gitTestRepo struct {
	tmpDir     string
	configPath string
	processor  *Processor
}

// setupGitTestRepo creates a git repository with an initial commit and user modifications.
func setupGitTestRepo(t *testing.T, initialContent, userContent string) *gitTestRepo {
	t.Helper()

	const fileName = "config.yaml"
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

	testRepo := setupGitTestRepo(t, initialContent, userContent)

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

	testRepo := setupGitTestRepo(t, initialContent, userContent)

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

	testRepo := setupGitTestRepo(t, initialContent, userContent)

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

func TestProcessorSetMaxChangesAndDirectMerge(t *testing.T) {
	processor := NewProcessor()
	processor.SetMaxChanges(100)

	result, err := processor.Merge("name: old\n", "name: user\n", "name: template\n", "config.yaml")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HasConflicts)
	assert.Contains(t, result.Content, "name: user")
}

func TestProcessorSetupGitStorageInvalidBaseRef(t *testing.T) {
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	configPath := filepath.Join(repoDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("name: demo\n"), 0o644))
	_, err = worktree.Add("config.yaml")
	require.NoError(t, err)
	_, err = worktree.Commit("initial", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com"},
	})
	require.NoError(t, err)

	processor := NewProcessor()
	err = processor.SetupGitStorage(repoDir, "missing-ref")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidBaseRef)
	// A failed SetupGitStorage call must not leave the Processor's state
	// half-mutated: targetPath/gitStorage should remain at their zero values.
	assert.Empty(t, processor.targetPath)
	assert.Nil(t, processor.gitStorage)
}

func TestProcessorMergeFileReadError(t *testing.T) {
	processor := NewProcessor()

	err := processor.mergeFile(filepath.Join(t.TempDir(), "missing.yaml"), File{Path: "missing.yaml", Permissions: 0o644}, t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadFile)
}

func TestProcessorMergeFileTemplateProcessingError(t *testing.T) {
	initialContent := "name: demo\n"
	testRepo := setupGitTestRepo(t, initialContent, initialContent)

	templateFile := File{
		Path:        "config.yaml",
		Content:     "{{",
		IsTemplate:  true,
		Permissions: 0o644,
	}

	err := testRepo.processor.mergeFile(testRepo.configPath, templateFile, testRepo.tmpDir)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTemplateExecution)
}

func TestProcessorDetermineBaseContentWithoutGitStorage(t *testing.T) {
	processor := NewProcessor()

	_, shouldSkip, err := processor.determineBaseContent(File{Path: "config.yaml"}, filepath.Join(t.TempDir(), "config.yaml"))

	require.Error(t, err)
	assert.False(t, shouldSkip)
	assert.ErrorIs(t, err, errUtils.ErrThreeWayMerge)
}

func TestProcessorDetermineBaseContentRelFallback(t *testing.T) {
	initialContent := "name: demo\n"
	testRepo := setupGitTestRepo(t, initialContent, initialContent)
	testRepo.processor.targetPath = "relative"

	base, shouldSkip, err := testRepo.processor.determineBaseContent(File{Path: "config.yaml"}, testRepo.configPath)

	require.NoError(t, err)
	assert.False(t, shouldSkip)
	assert.Equal(t, initialContent, base)
}

// TestProcessorSetupGitStorage_CorruptedRepoError covers the branch where
// git.PlainOpenWithOptions fails with an error other than git.ErrRepositoryNotExists.
func TestProcessorSetupGitStorage_CorruptedRepoError(t *testing.T) {
	repoDir := t.TempDir()
	// A .git that exists but is not a valid object database triggers an open
	// error distinct from "repository does not exist".
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git"), []byte("not a real git dir"), 0o644))

	processor := NewProcessor()
	err := processor.SetupGitStorage(repoDir, "HEAD")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrThreeWayMerge)
	assert.NotErrorIs(t, err, git.ErrRepositoryNotExists)
}

// TestProcessorMergeFile_DetermineBaseContentErrorPropagates covers mergeFile's
// propagation of a determineBaseContent error (as opposed to testing
// determineBaseContent in isolation). The base storage is pointed at a
// nonexistent ref by bypassing SetupGitStorage's own ref validation, so
// LoadBase fails when mergeFile calls into it.
func TestProcessorMergeFile_DetermineBaseContentErrorPropagates(t *testing.T) {
	initialContent := "name: demo\n"
	testRepo := setupGitTestRepo(t, initialContent, initialContent)

	repo, err := git.PlainOpen(testRepo.tmpDir)
	require.NoError(t, err)
	testRepo.processor.gitStorage = storage.NewGitBaseStorage(repo, "nonexistent-ref")

	templateFile := File{Path: "config.yaml", Content: "name: template\n", Permissions: 0o644}
	err = testRepo.processor.mergeFile(testRepo.configPath, templateFile, testRepo.tmpDir)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrThreeWayMerge)
}

// TestProcessorMergeFile_TemplateProcessingSuccess covers the success path of
// template processing inside mergeFile (IsTemplate: true with content that
// renders cleanly), as opposed to the existing failure-path-only coverage.
func TestProcessorMergeFile_TemplateProcessingSuccess(t *testing.T) {
	initialContent := "name: demo\n"
	testRepo := setupGitTestRepo(t, initialContent, initialContent)

	templateFile := File{
		Path:        "config.yaml",
		Content:     "name: demo # rendered via a valid, variable-free template\n",
		IsTemplate:  true,
		Permissions: 0o644,
	}

	mergeErr := testRepo.processor.mergeFile(testRepo.configPath, templateFile, testRepo.tmpDir)

	require.NoError(t, mergeErr)

	mergedContent, readErr := os.ReadFile(testRepo.configPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(mergedContent), "name: demo")
}

// TestProcessorMergeFile_ConflictBranchReturnsError raises the merge threshold
// so that a genuine conflict is not rejected earlier by TextMerger's own
// threshold check (ErrMergeThresholdExceeded); this isolates mergeFile's own
// result.HasConflicts branch (ErrMergeConflict).
func TestProcessorMergeFile_ConflictBranchReturnsError(t *testing.T) {
	initialContent := "setting: original\n"
	userContent := "setting: user-change\n"
	testRepo := setupGitTestRepo(t, initialContent, userContent)
	testRepo.processor.SetMaxChanges(100)

	templateFile := File{
		Path:        "config.yaml",
		Content:     "setting: template-change\n",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	err := testRepo.processor.mergeFile(testRepo.configPath, templateFile, testRepo.tmpDir)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMergeConflict)
}

// Note: mergeFile's os.WriteFile failure branch (writing merged content back
// to existingPath) is not covered here. Reaching it requires existingPath to
// remain a valid, readable regular file through os.ReadFile at the top of
// mergeFile, then fail specifically at the write step — the "directory
// already exists at this path" trick used elsewhere (e.g.
// templating_coverage_test.go's TestWriteFileErrors) does not apply here,
// since that trick fails at the read step instead for a path mergeFile
// requires to already be a regular file. Forcing this branch portably would
// need either a chmod-based permission trick (root/Windows-unsafe, per repo
// convention) or a new injectable write seam, both out of scope here.

// TestProcessorDetermineBaseContent_LoadBaseError covers LoadBase returning a
// non-nil error (as opposed to the found=true and gitStorage==nil cases
// already covered), by pointing base storage at an unresolvable ref.
func TestProcessorDetermineBaseContent_LoadBaseError(t *testing.T) {
	initialContent := "name: demo\n"
	testRepo := setupGitTestRepo(t, initialContent, initialContent)

	repo, err := git.PlainOpen(testRepo.tmpDir)
	require.NoError(t, err)
	testRepo.processor.gitStorage = storage.NewGitBaseStorage(repo, "nonexistent-ref")

	_, shouldSkip, err := testRepo.processor.determineBaseContent(File{Path: "config.yaml"}, testRepo.configPath)

	require.Error(t, err)
	assert.False(t, shouldSkip)
	assert.ErrorIs(t, err, errUtils.ErrThreeWayMerge)
}
