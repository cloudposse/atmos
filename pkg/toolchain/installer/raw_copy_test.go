package installer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopyRawBinaryPreservesPreviousExecutable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "download")
	destination := filepath.Join(dir, "installed")
	previous := filepath.Join(dir, "previous")
	require.NoError(t, os.WriteFile(source, []byte("replacement binary"), 0o755))
	require.NoError(t, os.WriteFile(destination, []byte("previous binary"), 0o755))
	before, err := os.Stat(destination)
	require.NoError(t, err)
	require.NoError(t, os.Link(destination, previous))

	require.NoError(t, (&Installer{}).copyFile(source, destination))

	installed, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "replacement binary", string(installed))
	after, err := os.Stat(destination)
	require.NoError(t, err)
	require.Equal(t, before.Mode().Perm(), after.Mode().Perm())
	old, err := os.ReadFile(previous)
	require.NoError(t, err)
	require.Equal(t, "previous binary", string(old),
		"installing a replacement must not modify the previous executable's inode")
	download, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, "replacement binary", string(download))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 3, "staging files must be cleaned up")
}

func TestCopyRawBinaryFailurePreservesDestination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source func(*testing.T, string) string
	}{
		{
			name: "missing source",
			source: func(_ *testing.T, dir string) string {
				return filepath.Join(dir, "missing")
			},
		},
		{
			name: "unreadable source directory",
			source: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "source")
				require.NoError(t, os.Mkdir(path, 0o755))
				return path
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			destination := filepath.Join(dir, "installed")
			require.NoError(t, os.WriteFile(destination, []byte("previous binary"), 0o755))
			source := tt.source(t, dir)
			before, err := os.ReadDir(dir)
			require.NoError(t, err)

			require.ErrorIs(t, (&Installer{}).copyFile(source, destination), ErrFileOperation)

			content, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, "previous binary", string(content))
			after, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Equal(t, before, after, "failed copies must clean up their staging directory")
		})
	}
}

func TestCopyRawBinaryIdenticalReinstallPreservesOpenExecutable(t *testing.T) {
	t.Parallel()
	for _, content := range [][]byte{{}, []byte("verified binary"), bytes.Repeat([]byte("verified binary"), 10000)} {
		t.Run(fmt.Sprintf("bytes_%d", len(content)), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			source, destination := filepath.Join(dir, "download"), filepath.Join(dir, "installed")
			require.NoError(t, os.WriteFile(source, content, 0o600))
			require.NoError(t, os.WriteFile(destination, content, 0o755))
			before, err := os.Stat(destination)
			require.NoError(t, err)
			// Windows opens this without FILE_SHARE_DELETE, modeling a scanner's
			// read handle. Reinstalling identical verified bytes needs no rename.
			opened, err := os.Open(destination)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, opened.Close()) })
			require.NoError(t, (&Installer{}).copyFile(source, destination))
			after, err := os.Stat(destination)
			require.NoError(t, err)
			require.True(t, os.SameFile(before, after), "an identical reinstall must preserve the existing executable")
			require.Equal(t, before.Mode().Perm(), after.Mode().Perm())
			actual, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, content, actual)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 2, "the unused staging copy must be removed")
		})
	}
}

func TestCopyRawBinaryRejectsSymlinkDestination(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires privileges on Windows")
	}
	dir := t.TempDir()
	source, destination, outside := filepath.Join(dir, "download"), filepath.Join(dir, "installed"), filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.WriteFile(source, []byte("verified binary"), 0o755))
	require.NoError(t, os.WriteFile(outside, []byte("verified binary"), 0o755))
	require.NoError(t, os.Symlink(outside, destination))
	require.ErrorIs(t, (&Installer{}).copyFile(source, destination), ErrFileOperation)
	link, err := os.Readlink(destination)
	require.NoError(t, err)
	require.Equal(t, outside, link)
	value, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "verified binary", string(value))
}

func TestCopyRawBinaryChangedContentDoesNotReuseOpenDestination(t *testing.T) {
	t.Parallel()
	for _, suffix := range []string{"different", ""} {
		t.Run("suffix_"+suffix, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			source, destination := filepath.Join(dir, "download"), filepath.Join(dir, "installed")
			prefix := bytes.Repeat([]byte("common bytes"), 10000)
			previous := append(append([]byte(nil), prefix...), []byte("previous!")...)
			replacement := append(append([]byte(nil), prefix...), []byte(suffix)...)
			require.NoError(t, os.WriteFile(source, replacement, 0o755))
			require.NoError(t, os.WriteFile(destination, previous, 0o755))
			opened, err := os.Open(destination)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, opened.Close()) })
			err = (&Installer{}).copyFile(source, destination)
			if runtime.GOOS == "windows" {
				require.ErrorIs(t, err, ErrFileOperation, "different bytes must not bypass a locked replacement")
			} else {
				require.NoError(t, err)
			}
			retained, err := io.ReadAll(opened)
			require.NoError(t, err)
			require.Equal(t, previous, retained, "the prior executable must never be rewritten in place")
			actual, err := os.ReadFile(destination)
			require.NoError(t, err)
			if runtime.GOOS == "windows" {
				require.Equal(t, previous, actual, "failed replacement must preserve the destination")
			} else {
				require.Equal(t, replacement, actual, "a common prefix must not be mistaken for identical content")
			}
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 2, "staging files must be cleaned on success and failure")
		})
	}
}
