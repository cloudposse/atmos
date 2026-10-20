package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCreationPreflightsAllOutputs(t *testing.T) {
	for _, paths := range [][]string{{"first.txt", "existing.txt"}, {"same.txt", "./same.txt"}, {"parent", "parent/child"}, {"../escape"}} {
		t.Run(paths[0], func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "existing.txt"), []byte("keep"), 0o644))
			var files []File
			for _, path := range paths {
				files = append(files, File{Path: path, Content: "new", Permissions: 0o644})
			}
			require.Error(t, ApplyCreation(context.Background(), root, files))
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, "existing.txt", entries[0].Name())
			content, err := os.ReadFile(filepath.Join(root, "existing.txt"))
			require.NoError(t, err)
			assert.Equal(t, "keep", string(content))
		})
	}
}

func TestCreationWritesRenderedBytes(t *testing.T) {
	root := t.TempDir()
	files := []File{{Path: "a/file.yaml", Content: "vars: {}\n", Permissions: 0o640}, {Path: "b/file.txt", Content: "literal {{ untouched }}", Permissions: 0o644}}
	require.NoError(t, ApplyCreation(context.Background(), root, files))
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(root, file.Path))
		require.NoError(t, err)
		assert.Equal(t, file.Content, string(content))
	}
	require.Error(t, ApplyCreation(context.Background(), root, files))
}

func TestValidateCreationRejectsWriteThroughEscapingSymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	err := ValidateCreation(root, []File{{Path: filepath.Join("link", "file.txt"), Content: "x", Permissions: 0o644}})
	require.ErrorIs(t, err, errUtils.ErrPathTraversal)
	assert.NoFileExists(t, filepath.Join(outside, "file.txt"))

	// Negative path: a symlink that stays inside the target is allowed.
	inside := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(inside, 0o755))
	if err := os.Symlink(inside, filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	require.NoError(t, ValidateCreation(root, []File{{Path: filepath.Join("alias", "file.txt"), Content: "x", Permissions: 0o644}}))
}

func TestValidateCreationReportsUninspectableDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a path below a regular file as missing instead of failing")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "occupied"), []byte("file"), 0o644))

	err := ValidateCreation(root, []File{{Path: filepath.Join("occupied", "child.txt"), Content: "x", Permissions: 0o644}})
	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrScaffoldDuplicateOutputPath)
}

func TestApplyCreationStopsWhenContextIsCanceled(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	files := []File{{Path: "a.txt", Content: "a", Permissions: 0o644}, {Path: "b.txt", Content: "b", Permissions: 0o644}}

	require.ErrorIs(t, ApplyCreation(ctx, root, files), context.Canceled)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "a canceled creation writes nothing")

	// Negative path: the same plan applies with a live context.
	require.NoError(t, ApplyCreation(context.Background(), root, files))
	assert.FileExists(t, filepath.Join(root, "b.txt"))
}

func TestDryRunPreviewReceivesRenderedFileWithoutWriting(t *testing.T) {
	root := t.TempDir()
	var previewed []File
	processor := NewProcessor()
	processor.SetDryRun(true)
	processor.PreviewFile = func(f File) { previewed = append(previewed, f) }
	file := File{Path: filepath.Join("conf", "{{ .Config.name }}.txt"), Content: "name={{ .Config.name }}\n", IsTemplate: true, Permissions: 0o640}
	values := map[string]any{"name": "demo"}

	require.NoError(t, processor.ProcessFile(file, root, false, false, nil, values))
	require.Len(t, previewed, 1)
	assert.Equal(t, filepath.Join("conf", "demo.txt"), previewed[0].Path, "the preview path is relative to the target")
	assert.Equal(t, "name=demo\n", previewed[0].Content)
	assert.Equal(t, os.FileMode(0o640), previewed[0].Permissions)
	assert.NoDirExists(t, filepath.Join(root, "conf"), "a dry run writes nothing")

	// Negative path: without a preview callback the dry run still renders and writes nothing.
	processor.PreviewFile = nil
	require.NoError(t, processor.ProcessFile(file, root, false, false, nil, values))
	assert.Len(t, previewed, 1)
}

func TestApplyCreationReportsWriteFailuresWithoutPartialOutput(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires POSIX permissions enforced for the current user")
	}
	root := t.TempDir()
	// A read-only target accepts validation but rejects every write.
	require.NoError(t, os.Chmod(root, 0o500))
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	err := ApplyCreation(context.Background(), root, []File{{Path: filepath.Join("nested", "a.txt"), Content: "a", Permissions: 0o644}})
	require.ErrorIs(t, err, errUtils.ErrDirectoryCreation)

	err = ApplyCreation(context.Background(), root, []File{{Path: "a.txt", Content: "a", Permissions: 0o644}})
	require.ErrorIs(t, err, errUtils.ErrFileWrite)
	assert.Contains(t, err.Error(), "a.txt")

	require.NoError(t, os.Chmod(root, 0o700))
	entries, readErr := os.ReadDir(root)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "a failed creation leaves nothing behind")
}

func TestCreationRollsBackLateFailureAndCanRetry(t *testing.T) {
	root := t.TempDir()
	files := []File{{Path: "manifest.yaml", Content: "manifest", Permissions: 0o644}, {Path: "later.txt", Content: "later", Permissions: 0o644}}
	write := func(path string, file *File) (*createdFile, error) {
		if file.Path == "later.txt" {
			return nil, os.ErrPermission
		}
		return writeCreationFile(path, file)
	}
	err := applyCreation(context.Background(), root, files, write)
	require.ErrorIs(t, err, os.ErrPermission)
	assert.NoFileExists(t, filepath.Join(root, "manifest.yaml"))
	require.NoError(t, ApplyCreation(context.Background(), root, files))
}

func TestCreationRollbackPreservesConcurrentChanges(t *testing.T) {
	for _, change := range []string{"edit", "replace", "hook file"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			files := []File{{Path: "first", Content: "ours", Permissions: 0o644}, {Path: "second", Content: "ours", Permissions: 0o644}}
			write := func(path string, file *File) (*createdFile, error) {
				if file.Path == "first" {
					return writeCreationFile(path, file)
				}
				first := filepath.Join(root, "first")
				switch change {
				case "replace":
					require.NoError(t, os.Rename(first, first+".moved"))
					require.NoError(t, os.WriteFile(first, []byte("replacement"), 0o644))
				case "edit":
					require.NoError(t, os.WriteFile(first, []byte("edited"), 0o644))
				case "hook file":
					require.NoError(t, os.WriteFile(filepath.Join(root, "hook"), []byte("keep"), 0o644))
				}
				return nil, os.ErrPermission
			}
			err := applyCreation(context.Background(), root, files, write)
			require.ErrorIs(t, err, os.ErrPermission)
			if change == "hook file" {
				assert.NoFileExists(t, filepath.Join(root, "first"))
				assert.FileExists(t, filepath.Join(root, "hook"))
			} else {
				assert.Contains(t, err.Error(), "rollback left")
				content, readErr := os.ReadFile(filepath.Join(root, "first"))
				require.NoError(t, readErr)
				assert.NotEqual(t, "ours", string(content))
			}
		})
	}
}

func TestCreationRollsBackCancellationAndPartialWrites(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprint(partial), func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			files := []File{{Path: "first", Content: "complete", Permissions: 0o644}, {Path: "second", Content: "complete", Permissions: 0o644}}
			write := func(path string, file *File) (*createdFile, error) {
				if partial && file.Path == "second" {
					file.Content = "part"
					owned, err := writeCreationFile(path, file)
					require.NoError(t, err)
					return owned, io.ErrShortWrite
				}
				owned, err := writeCreationFile(path, file)
				if !partial {
					cancel()
				}
				return owned, err
			}
			err := applyCreation(ctx, root, files, write)
			if partial {
				require.ErrorIs(t, err, io.ErrShortWrite)
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			assert.NoFileExists(t, filepath.Join(root, "first"))
			assert.NoFileExists(t, filepath.Join(root, "second"))
		})
	}
}
