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
)

// TestProcessorWithGitStorage_DeletedByUserNotRecreated verifies that a file
// present at the git base ref, but absent on disk (the user deleted it), is
// not recreated under --update-strategy=tracked -- see
// Processor.deletedByUser.
func TestProcessorWithGitStorage_DeletedByUserNotRecreated(t *testing.T) {
	tmpDir := t.TempDir()

	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	removedPath := filepath.Join(tmpDir, "removed.yaml")
	require.NoError(t, os.WriteFile(removedPath, []byte("version: 1.0\n"), 0o644))
	_, err = worktree.Add("removed.yaml")
	require.NoError(t, err)
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com"},
	})
	require.NoError(t, err)

	// The user deletes the file after it was committed.
	require.NoError(t, os.Remove(removedPath))

	processor := NewProcessor()
	require.NoError(t, processor.SetupGitStorage(tmpDir, "HEAD"))

	templateFile := File{
		Path:        "removed.yaml",
		Content:     "version: 2.0\n",
		Permissions: 0o644,
	}

	err = processor.ProcessFile(templateFile, tmpDir, false, true, nil, nil)
	require.Error(t, err)
	var skipErr *FileSkippedError
	require.True(t, errors.As(err, &skipErr), "expected a FileSkippedError, got: %v", err)
	assert.NotEmpty(t, skipErr.Reason)

	_, statErr := os.Stat(removedPath)
	require.True(t, os.IsNotExist(statErr), "the deleted file must not be recreated")
}

// TestProcessorWithGitStorage_NewFileStillWritten verifies that a path with
// no history at the base ref -- genuinely new -- is still written normally
// under --update-strategy=tracked, i.e. deletedByUser's "no base found" branch
// does not regress ordinary new-file creation.
func TestProcessorWithGitStorage_NewFileStillWritten(t *testing.T) {
	tmpDir := t.TempDir()

	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	existingPath := filepath.Join(tmpDir, "existing.yaml")
	require.NoError(t, os.WriteFile(existingPath, []byte("version: 1.0\n"), 0o644))
	_, err = worktree.Add("existing.yaml")
	require.NoError(t, err)
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com"},
	})
	require.NoError(t, err)

	processor := NewProcessor()
	require.NoError(t, processor.SetupGitStorage(tmpDir, "HEAD"))

	// A path with no history anywhere in the base ref.
	templateFile := File{
		Path:        "new-file.yaml",
		Content:     "brand: new\n",
		Permissions: 0o644,
	}

	err = processor.ProcessFile(templateFile, tmpDir, false, true, nil, nil)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(tmpDir, "new-file.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "brand: new\n", string(content))
}

// TestProcessorWithGitStorage_RecreateDeletedOptsBackIn confirms
// SetRecreateDeleted(true) restores the pre-deletion-handling behavior of
// always recreating a file the user deleted, independent of --force/
// --merge-strategy.
func TestProcessorWithGitStorage_RecreateDeletedOptsBackIn(t *testing.T) {
	tmpDir := t.TempDir()

	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	removedPath := filepath.Join(tmpDir, "removed.yaml")
	require.NoError(t, os.WriteFile(removedPath, []byte("version: 1.0\n"), 0o644))
	_, err = worktree.Add("removed.yaml")
	require.NoError(t, err)
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com"},
	})
	require.NoError(t, err)
	require.NoError(t, os.Remove(removedPath))

	processor := NewProcessor()
	require.NoError(t, processor.SetupGitStorage(tmpDir, "HEAD"))
	processor.SetRecreateDeleted(true)

	templateFile := File{
		Path:        "removed.yaml",
		Content:     "version: 2.0\n",
		Permissions: 0o644,
	}

	err = processor.ProcessFile(templateFile, tmpDir, false, true, nil, nil)
	require.NoError(t, err)

	content, err := os.ReadFile(removedPath)
	require.NoError(t, err)
	assert.Equal(t, "version: 2.0\n", string(content), "SetRecreateDeleted(true) must recreate the file")
}

// TestProcessorWithRenderedBaseStorage_DeletedByUserNotRecreated mirrors
// TestProcessorWithGitStorage_DeletedByUserNotRecreated for
// --update-strategy=rendered, confirming deletedByUser works uniformly across
// both baseContentLoader implementations.
func TestProcessorWithRenderedBaseStorage_DeletedByUserNotRecreated(t *testing.T) {
	renderRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(renderRoot, "removed.yaml"), []byte("version: 1.0\n"), 0o644))

	targetPath := t.TempDir()
	// The file is absent in targetPath: the user deleted it after a prior
	// generation at renderRoot's ref.

	processor := NewProcessor()
	processor.SetupRenderedBaseStorage(targetPath, renderRoot)

	templateFile := File{
		Path:        "removed.yaml",
		Content:     "version: 2.0\n",
		Permissions: 0o644,
	}

	err := processor.ProcessFile(templateFile, targetPath, false, true, nil, nil)
	require.Error(t, err)
	var skipErr *FileSkippedError
	require.True(t, errors.As(err, &skipErr), "expected a FileSkippedError, got: %v", err)

	_, statErr := os.Stat(filepath.Join(targetPath, "removed.yaml"))
	require.True(t, os.IsNotExist(statErr), "the deleted file must not be recreated")
}

// TestProcessorDeletedByUser_TargetRenameStillCreatesFreshFile is a
// regression test for a bug found during field-testing: deletedByUser used
// to call determineBaseContent, whose migration fallback (OriginalSourcePath)
// exists to recover a merge base for a file a spec.files[].target: rename has
// already moved to a new path where it ALREADY exists on disk (see
// determineBaseContentMigrationFallback's doc comment). Reached from
// deletedByUser -- where the file does NOT exist yet -- that fallback instead
// misreported the documented "first --update after target: changed" case as
// a user deletion, permanently blocking the renamed file from ever being
// created and blaming the user for it. To fix this, deletedByUser now looks
// up baseStorage.LoadBase directly at the current path only, ignoring
// OriginalSourcePath, so this scenario falls through to writeNewFile instead.
func TestProcessorDeletedByUser_TargetRenameStillCreatesFreshFile(t *testing.T) {
	renderRoot := t.TempDir()
	// Old ref's pristine render: the file lived at "a.txt" before the
	// template's spec.files[].target: renamed it to "b.txt".
	require.NoError(t, os.WriteFile(filepath.Join(renderRoot, "a.txt"), []byte("old content\n"), 0o644))

	targetPath := t.TempDir()
	// "b.txt" has never existed on disk -- this is the first generation
	// after the rename.

	processor := NewProcessor()
	processor.SetupRenderedBaseStorage(targetPath, renderRoot)

	// Mirrors ui.go's writeOneOutput: OriginalSourcePath is the file's
	// discovered source path ("a.txt"), Path is the rendered output path
	// after spec.Target is applied ("b.txt").
	templateFile := File{
		Path:               "b.txt",
		OriginalSourcePath: "a.txt",
		Content:            "new content\n",
		Permissions:        0o644,
	}

	err := processor.ProcessFile(templateFile, targetPath, false, true, nil, nil)
	require.NoError(t, err, "a target: rename's first post-rename write must succeed, not be treated as a user deletion")

	content, err := os.ReadFile(filepath.Join(targetPath, "b.txt"))
	require.NoError(t, err, "the renamed file must be created at its new path")
	assert.Equal(t, "new content\n", string(content))
}
