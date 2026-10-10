package automation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalFileSystem(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs := LocalFileSystem{}
	dir := t.TempDir()
	for _, name := range []string{"b.md", "a.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("café"), 0o600))
	}
	matches, err := fs.Glob(ctx, filepath.Join(dir, "*.md"))
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")}, matches)
	info, err := fs.Stat(ctx, matches[0], true)
	require.NoError(t, err)
	assert.Equal(t, FileInfo{Size: 5, IsFile: true}, info)
	info, err = fs.Stat(ctx, dir, true)
	require.NoError(t, err)
	assert.True(t, info.IsDir)
	exists, err := fs.Exists(ctx, filepath.Join(dir, "missing"))
	require.NoError(t, err)
	assert.False(t, exists)
	exists, err = fs.Exists(ctx, dir)
	require.NoError(t, err)
	assert.True(t, exists)
	_, err = fs.Stat(ctx, filepath.Join(dir, "missing"), true)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = fs.Glob(ctx, "[")
	require.ErrorIs(t, err, filepath.ErrBadPattern)
	_, err = fs.Exists(ctx, "\x00")
	require.Error(t, err, "invalid paths must not be reported as missing")
	_, err = fs.Readlink(ctx, matches[0])
	require.Error(t, err)
}

func TestLocalFileSystemSymlinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs := LocalFileSystem{}
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	exists, err := fs.Exists(ctx, link)
	require.NoError(t, err)
	assert.False(t, exists)
	target, err := fs.Readlink(ctx, link)
	require.NoError(t, err)
	assert.Equal(t, "target", target)
	info, err := fs.Stat(ctx, link, false)
	require.NoError(t, err)
	assert.True(t, info.IsSymlink)
	require.NoError(t, os.WriteFile(filepath.Join(dir, target), []byte("hello"), 0o600))
	info, err = fs.Stat(ctx, link, true)
	require.NoError(t, err)
	assert.Equal(t, FileInfo{Size: 5, IsFile: true}, info)
}

func TestLocalFileSystemCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fs := LocalFileSystem{}
	_, err := fs.Glob(ctx, "*")
	require.ErrorIs(t, err, context.Canceled)
	_, err = fs.Stat(ctx, ".", true)
	require.ErrorIs(t, err, context.Canceled)
	_, err = fs.Exists(ctx, ".")
	require.ErrorIs(t, err, context.Canceled)
	_, err = fs.Readlink(ctx, "link")
	require.ErrorIs(t, err, context.Canceled)
}
