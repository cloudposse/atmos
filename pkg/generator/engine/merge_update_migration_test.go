package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// commitFile creates a git repository under a fresh temp dir, writes relPath
// with content, and commits it -- the shared setup every test below needs to
// establish "this content is what the base ref has for this path".
func commitFile(t *testing.T, relPath, content string) (tmpDir string) {
	t.Helper()

	tmpDir = t.TempDir()

	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	fullPath := filepath.Join(tmpDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))

	_, err = worktree.Add(relPath)
	require.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com"},
	})
	require.NoError(t, err)

	return tmpDir
}

// TestProcessFileUpdate_MigratedTargetRecoversMergeBase is a regression test
// for CodeRabbit finding #2 on PR #3187: when a spec.files[] entry's target:
// changes (e.g. adopting a glob+.file.RelPath-based target on an entry that
// previously rendered verbatim to its own discovered path), the file's
// output moves to a new path that has no git history of its own -- only the
// OLD path was ever committed. Without OriginalSourcePath's fallback,
// determineBaseContent would find nothing at the new path and permanently
// treat the file as user-added, silently freezing it at whatever the first
// post-migration `--update` wrote and never applying template changes to it
// again. With the fallback, the base is recovered from the file's original
// discovered path, so `--update` performs a real 3-way merge instead.
func TestProcessFileUpdate_MigratedTargetRecoversMergeBase(t *testing.T) {
	const (
		oldPath = "components/vpc/main.tf"
		newPath = "environments/dev/vpc/main.tf"
	)
	baseContent := "# vpc\nversion = 1\n"

	tmpDir := commitFile(t, oldPath, baseContent)

	// Simulate the first post-migration `--update`: it already wrote the new
	// path fresh (writeNewFile, since it didn't exist yet), and the user has
	// since made their own edit to it. This is "ours" for the merge below.
	newFullPath := filepath.Join(tmpDir, newPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(newFullPath), 0o755))
	oursContent := "# vpc\nversion = 1\n\n# user custom block\ncustom = true\n"
	require.NoError(t, os.WriteFile(newFullPath, []byte(oursContent), 0o644))

	processor := NewProcessor()
	require.NoError(t, processor.SetupGitStorage(tmpDir, "HEAD"))

	// A second `--update`, after the template itself changed again
	// ("theirs"). OriginalSourcePath carries the file's own path as
	// discovered in the template's source tree -- exactly what
	// pkg/generator/ui's writeOneOutput populates it with before Path is
	// overwritten with the rendered target.
	templateFile := File{
		Path:               newPath,
		OriginalSourcePath: oldPath,
		Content:            "# vpc\nversion = 2\n",
		IsTemplate:         false,
		Permissions:        0o644,
	}

	err := processor.ProcessFile(templateFile, tmpDir, false, true, nil, nil)
	require.NoError(t, err)

	merged, err := os.ReadFile(newFullPath)
	require.NoError(t, err)

	// A real 3-way merge happened: the template's new version and the user's
	// custom addition are both present -- not a silent freeze (which would
	// keep only oursContent unchanged) and not a full overwrite (which would
	// drop the user's custom block).
	assert.Contains(t, string(merged), "version = 2", "should have the new version from the template")
	assert.Contains(t, string(merged), "custom = true", "should preserve the user's custom addition")
}

// TestProcessFileUpdate_NoOriginalSourcePathStillSkipsUserAddedFile is the
// backward-compatibility counterpart: when a caller never populates
// OriginalSourcePath (e.g. a direct engine caller/test with no notion of
// "discovered source path" vs. "rendered target"), a file missing from git
// history at its current path is still treated as user-added and left
// untouched, exactly like before this fallback was added.
func TestProcessFileUpdate_NoOriginalSourcePathStillSkipsUserAddedFile(t *testing.T) {
	tmpDir := commitFile(t, "README.md", "# Project\n")

	customPath := filepath.Join(tmpDir, "custom.yaml")
	customContent := "# User's custom file\ncustom: true\n"
	require.NoError(t, os.WriteFile(customPath, []byte(customContent), 0o644))

	processor := NewProcessor()
	require.NoError(t, processor.SetupGitStorage(tmpDir, "HEAD"))

	templateFile := File{
		Path:        "custom.yaml",
		Content:     "# Template file\ntemplate: value\n",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	err := processor.ProcessFile(templateFile, tmpDir, false, true, nil, nil)
	require.NoError(t, err)

	result, err := os.ReadFile(customPath)
	require.NoError(t, err)
	assert.Equal(t, customContent, string(result), "user-added file should be left untouched")
}

// TestProcessFileUpdate_MigrationFallbackAlsoMissingStillSkips covers the
// case where OriginalSourcePath is populated (this file did go through a
// target: change) but neither the current nor the original path has any git
// history yet -- e.g. the very first `--update` after a migration hasn't
// been committed at all. The file must still be treated as user-added
// (never silently mutated by a merge with no real base), matching
// determineBaseContentMigrationFallback's documented "still ambiguous, warn
// instead of silently skipping" branch.
func TestProcessFileUpdate_MigrationFallbackAlsoMissingStillSkips(t *testing.T) {
	tmpDir := commitFile(t, "README.md", "# Project\n")

	newFullPath := filepath.Join(tmpDir, "environments", "dev", "vpc", "main.tf")
	require.NoError(t, os.MkdirAll(filepath.Dir(newFullPath), 0o755))
	oursContent := "# vpc\nuser edit\n"
	require.NoError(t, os.WriteFile(newFullPath, []byte(oursContent), 0o644))

	processor := NewProcessor()
	require.NoError(t, processor.SetupGitStorage(tmpDir, "HEAD"))

	templateFile := File{
		Path:               filepath.Join("environments", "dev", "vpc", "main.tf"),
		OriginalSourcePath: filepath.Join("components", "vpc", "main.tf"), // Never committed either.
		Content:            "# vpc\nversion = 2\n",
		IsTemplate:         false,
		Permissions:        0o644,
	}

	err := processor.ProcessFile(templateFile, tmpDir, false, true, nil, nil)
	require.NoError(t, err)

	result, err := os.ReadFile(newFullPath)
	require.NoError(t, err)
	assert.Equal(t, oursContent, string(result), "should stay untouched when no base is found under either path")
}

// erroringOriginalPathLoader is a baseContentLoader test double that reports
// "not found" for every path except one, where it returns a genuine error --
// simulating a git/storage read failure (as opposed to "no base exists here")
// specifically for the fallback lookup at File.OriginalSourcePath.
type erroringOriginalPathLoader struct {
	errorPath string
	err       error
}

func (e *erroringOriginalPathLoader) LoadBase(filePath string) (string, bool, error) {
	if filePath == e.errorPath {
		return "", false, e.err
	}
	return "", false, nil
}

// TestDetermineBaseContent_OriginalPathLookupErrorPropagates is a regression
// test for CodeRabbit finding #3 on PR #3187: when the fallback lookup at
// File.OriginalSourcePath (added to recover the merge base after a target:
// migration) fails with a genuine storage/read error -- not merely "no base
// found at that path" -- determineBaseContentMigrationFallback must propagate
// that error as an `--update` failure, exactly like the primary current-path
// lookup does a few lines above. Before this fix, a non-nil error here was
// treated identically to "found=false": the file was silently left unchanged
// with only a warning, hiding a real failure to read the merge base.
func TestDetermineBaseContent_OriginalPathLookupErrorPropagates(t *testing.T) {
	const (
		relativePath = "environments/dev/vpc/main.tf"
		originalPath = "components/vpc/main.tf"
	)

	wantErr := errors.New("simulated storage read failure")

	processor := NewProcessor()
	processor.targetPath = t.TempDir()
	processor.baseStorage = &erroringOriginalPathLoader{errorPath: originalPath, err: wantErr}

	file := File{
		Path:               relativePath,
		OriginalSourcePath: originalPath,
	}
	existingPath := filepath.Join(processor.targetPath, relativePath)

	content, shouldSkip, err := processor.determineBaseContent(file, existingPath)

	require.Error(t, err, "a genuine storage error at the original path must propagate, not be swallowed as 'no base found'")
	assert.ErrorIs(t, err, errUtils.ErrThreeWayMerge)
	assert.ErrorIs(t, err, wantErr)
	assert.False(t, shouldSkip, "shouldSkip must not be set on the error path")
	assert.Empty(t, content)
}
