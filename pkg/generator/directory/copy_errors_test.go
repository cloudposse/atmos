package directory

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCopyRejectsTypeCollisionsBeforeWriting(t *testing.T) {
	for _, sourceDirectory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file-over-directory", true: "directory-over-file"}[sourceDirectory], func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(src, "a-first"), []byte("new"), 0o600))
			fileRoot, directoryRoot := src, dst
			if sourceDirectory {
				fileRoot, directoryRoot = dst, src
			}
			require.NoError(t, os.WriteFile(filepath.Join(fileRoot, "collision"), []byte("keep"), 0o600))
			require.NoError(t, os.Mkdir(filepath.Join(directoryRoot, "collision"), 0o755))
			err := Copy(context.Background(), src, dst, true)
			require.ErrorIs(t, err, errUtils.ErrInitialization)
			assert.Contains(t, err.Error(), "file/directory collision")
			assert.NoFileExists(t, filepath.Join(dst, "a-first"))
			content, err := os.ReadFile(filepath.Join(fileRoot, "collision"))
			require.NoError(t, err)
			assert.Equal(t, "keep", string(content))
		})
	}
}

func TestCopyInvalidPathsDoNotWrite(t *testing.T) {
	for _, invalidSource := range []bool{false, true} {
		targetRoot := t.TempDir()
		src, dst := t.TempDir(), filepath.Join(targetRoot, "target")
		if invalidSource {
			src = filepath.Join(src, "invalid") + "\x00"
		} else {
			dst = filepath.Join(dst, "invalid") + "\x00"
		}
		err := Copy(context.Background(), src, dst, true)
		// Windows can return EINVAL directly instead of wrapping it in os.PathError.
		require.ErrorIs(t, err, syscall.EINVAL)
		entries, err := os.ReadDir(targetRoot)
		require.NoError(t, err)
		assert.Empty(t, entries, "invalid paths must not create any destination entries")
	}
	src := filepath.Join(t.TempDir(), "missing")
	dst := filepath.Join(t.TempDir(), "target")
	require.ErrorIs(t, Copy(context.Background(), src, dst, false), os.ErrNotExist)
	assert.NoDirExists(t, dst)
}

func TestCopyRejectsFileDestination(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(dst, []byte("keep"), 0o600))
	require.Error(t, Copy(context.Background(), t.TempDir(), dst, false))
	content, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(content))
}

func TestCopyExcludesGitWorktreePointer(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(filepath.Join(src, ".git"), []byte("gitdir: private"), 0o600))
	require.NoError(t, Copy(context.Background(), src, dst, false))
	assert.DirExists(t, dst)
	assert.NoFileExists(t, filepath.Join(dst, ".git"))
}

func TestCopyEntriesHandlesChangesAfterInspection(t *testing.T) {
	for _, change := range []string{"cancel", "source-removed", "target-replaced"} {
		t.Run(change, func(t *testing.T) {
			src, dst := t.TempDir(), filepath.Join(t.TempDir(), "target")
			file := filepath.Join(src, "file")
			require.NoError(t, os.WriteFile(file, []byte("source"), 0o600))
			entries, err := inspectTree(context.Background(), src, dst)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch change {
			case "cancel":
				cancel()
			case "source-removed":
				require.NoError(t, os.Remove(file))
			case "target-replaced":
				require.NoError(t, os.WriteFile(dst, []byte("keep"), 0o600))
			}
			err = copyEntries(ctx, src, dst, entries)
			require.Error(t, err)
			if change == "cancel" {
				assert.ErrorIs(t, err, context.Canceled)
				assert.NoDirExists(t, dst)
			}
			if change == "source-removed" {
				assert.ErrorIs(t, err, os.ErrNotExist)
				assert.NoFileExists(t, filepath.Join(dst, "file"))
			}
			if change == "target-replaced" {
				content, readErr := os.ReadFile(dst)
				require.NoError(t, readErr)
				assert.Equal(t, "keep", string(content))
			}
		})
	}
}
