package target

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestLocalPublishOpenRejectsReplacementAfterPathCheck(t *testing.T) {
	t.Parallel()
	for _, replacement := range []string{"regular file", "symlink", "directory", "resized file"} {
		t.Run(replacement, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			filename := filepath.Join(root, "source")
			require.NoError(t, os.WriteFile(filename, []byte("safe"), 0o600))
			info, err := os.Lstat(filename)
			require.NoError(t, err)
			other := filepath.Join(root, "other")
			require.NoError(t, os.WriteFile(other, []byte("evil"), 0o600))
			if replacement == "symlink" {
				probe := filepath.Join(root, "symlink-probe")
				if err := os.Symlink(other, probe); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			var opened *os.File
			reader, err := openLocalPublishFile(filename, info, func(path string) (*os.File, error) {
				// Replace the source exactly after the pathname check, without racing goroutines.
				if replacement == "resized file" {
					require.NoError(t, os.WriteFile(path, []byte("changed size"), 0o600))
				} else {
					require.NoError(t, os.Rename(path, filepath.Join(root, "original")))
					switch replacement {
					case "regular file":
						require.NoError(t, os.Rename(other, path))
					case "symlink":
						require.NoError(t, os.Symlink(other, path))
					case "directory":
						require.NoError(t, os.Mkdir(path, 0o700))
					}
				}
				opened, err = os.Open(path)
				if err == nil {
					t.Cleanup(func() { _ = opened.Close() })
				}
				return opened, err
			})
			require.ErrorIs(t, err, errUtils.ErrPublishSource)
			require.Nil(t, reader)
			require.NotNil(t, opened)
			err = opened.Close()
			require.ErrorIs(t, err, os.ErrClosed, "rejected source handles must be closed")
		})
	}
}

func TestLocalPublishOpenKeepsValidatedHandle(t *testing.T) {
	t.Parallel()
	filename := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.WriteFile(filename, []byte("safe"), 0o600))
	info, err := os.Lstat(filename)
	require.NoError(t, err)
	var opened *os.File
	reader, err := openLocalPublishFile(filename, info, func(path string) (*os.File, error) {
		opened, err = os.Open(path)
		return opened, err
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	require.Same(t, opened, reader)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "safe", string(data))
}

func TestLocalPublishOpenFailures(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"open", "stat"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			filename := filepath.Join(t.TempDir(), "source")
			require.NoError(t, os.WriteFile(filename, []byte("safe"), 0o600))
			info, err := os.Lstat(filename)
			require.NoError(t, err)
			reader, err := openLocalPublishFile(filename, info, func(path string) (*os.File, error) {
				if failure == "open" {
					return nil, os.ErrPermission
				}
				file, openErr := os.Open(path)
				require.NoError(t, openErr)
				require.NoError(t, file.Close())
				return file, nil
			})
			require.ErrorIs(t, err, errUtils.ErrPublishSource)
			require.Nil(t, reader)
			if failure == "open" {
				require.ErrorIs(t, err, os.ErrPermission)
			} else {
				require.ErrorIs(t, err, os.ErrClosed)
			}
		})
	}
}

func TestLocalPublishOpenRejectsSymlinkToInventoriedFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	filename := filepath.Join(root, "source")
	require.NoError(t, os.WriteFile(filename, []byte("safe"), 0o600))
	files, err := LocalPublishFiles(filename, "")
	require.NoError(t, err)
	require.Len(t, files, 1)
	original := filepath.Join(root, "original")
	require.NoError(t, os.Rename(filename, original))
	if err := os.Symlink(original, filename); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	reader, err := files[0].Open()
	require.ErrorIs(t, err, errUtils.ErrPublishSource)
	require.Nil(t, reader)
}
