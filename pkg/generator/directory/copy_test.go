package directory

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCopyPreservesSource(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "target")
	files := map[string][]byte{
		"{{literal}}.tmpl": []byte("# atmos:template\n{{ .Config.value }}\n"),
		"binary":           {0, 1, 0xff, 0xfe},
		".tool-versions":   []byte("terraform 1.0.0\n"),
		"scaffold.yaml":    []byte("invalid: ["),
		"run.sh":           []byte("#!/bin/sh\necho hello\n"),
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(src, name), content, 0o755))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(src, "nested", "empty"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(src, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".git", "config"), []byte("private"), 0o600))
	require.NoError(t, Copy(context.Background(), src, dst, false))
	for name, content := range files {
		actual, err := os.ReadFile(filepath.Join(dst, name))
		require.NoError(t, err)
		assert.Equal(t, content, actual)
	}
	assert.DirExists(t, filepath.Join(dst, "nested", "empty"))
	assert.NoDirExists(t, filepath.Join(dst, ".git"))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dst, "run.sh"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	}
}

func TestCopyForceAndOverlap(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "file"), []byte("old"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "unrelated"), []byte("keep"), 0o600))
	assert.ErrorIs(t, Copy(context.Background(), src, dst, false), errUtils.ErrTargetDirectoryNotEmpty)
	require.NoError(t, Copy(context.Background(), src, dst, true))
	actual, err := os.ReadFile(filepath.Join(dst, "file"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(actual))
	assert.FileExists(t, filepath.Join(dst, "unrelated"))
	for _, target := range []string{src, filepath.Join(src, "child"), filepath.Dir(src)} {
		assert.ErrorIs(t, Copy(context.Background(), src, target, true), errUtils.ErrPathTraversal)
	}
	root := filepath.VolumeName(src) + string(filepath.Separator)
	assert.ErrorIs(t, Copy(context.Background(), root, dst, true), errUtils.ErrPathTraversal)
}

func TestCopyRejectsSymlinksBeforeWriting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	for _, link := range []string{"file", "directory", "root", "source"} {
		t.Run(link, func(t *testing.T) {
			src, dst, outside := t.TempDir(), t.TempDir(), t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(src, "a-first"), []byte("first"), 0o600))
			require.NoError(t, os.Mkdir(filepath.Join(src, "nested"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(src, "nested", "file"), []byte("new"), 0o600))
			switch link {
			case "file":
				require.NoError(t, os.Mkdir(filepath.Join(dst, "nested"), 0o755))
				require.NoError(t, os.Symlink(filepath.Join(outside, "untouched"), filepath.Join(dst, "nested", "file")))
			case "directory":
				require.NoError(t, os.Symlink(outside, filepath.Join(dst, "nested")))
			case "root":
				dst = filepath.Join(dst, "target")
				require.NoError(t, os.Symlink(outside, dst))
			case "source":
				require.NoError(t, os.Symlink(outside, filepath.Join(src, "linked")))
			}
			require.Error(t, Copy(context.Background(), src, dst, true))
			assert.NoFileExists(t, filepath.Join(dst, "a-first"))
			entries, err := os.ReadDir(outside)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func TestCopyCancellationLeavesTargetAbsent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dst := filepath.Join(t.TempDir(), "target")
	assert.ErrorIs(t, Copy(ctx, t.TempDir(), dst, false), context.Canceled)
	assert.NoDirExists(t, dst)
}
