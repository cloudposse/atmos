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
