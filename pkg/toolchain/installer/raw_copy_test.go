package installer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	before, err := rawBinarySnapshot(destination)
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
			before, err := rawBinarySnapshot(destination)
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

func TestRawBinaryMatchesRejectsReplacedDestination(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	staged, destination, previous := filepath.Join(dir, "staged"), filepath.Join(dir, "installed"), filepath.Join(dir, "previous")
	content := []byte("identical bytes in different files")
	require.NoError(t, os.WriteFile(staged, content, 0o600))
	require.NoError(t, os.WriteFile(destination, content, 0o755))
	expected, err := rawBinarySnapshot(destination)
	require.NoError(t, err)
	// Snapshot identity through the same handle-based path as installation.
	// Keep the original inode alive so creating its replacement cannot reuse it.
	require.NoError(t, os.Rename(destination, previous))
	require.NoError(t, os.WriteFile(destination, content, 0o755))

	matches, err := rawBinaryMatches(staged, destination, expected)
	require.ErrorIs(t, err, ErrFileOperation)
	require.ErrorContains(t, err, "destination changed while opening")
	require.False(t, matches, "equal bytes must not bypass the destination identity check")
	for _, path := range []string{staged, destination, previous} {
		actual, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, content, actual, "comparison must not change either executable")
	}
}

func TestRawBinaryMatchesMissingInput(t *testing.T) {
	t.Parallel()
	for _, missing := range []string{"staged", "installed"} {
		t.Run(missing, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			staged, destination := filepath.Join(dir, "staged"), filepath.Join(dir, "installed")
			content := []byte("verified binary")
			require.NoError(t, os.WriteFile(staged, content, 0o600))
			require.NoError(t, os.WriteFile(destination, content, 0o755))
			expected, err := rawBinarySnapshot(destination)
			require.NoError(t, err)
			require.NoError(t, os.Remove(filepath.Join(dir, missing)))

			matches, err := rawBinaryMatches(staged, destination, expected)
			require.ErrorIs(t, err, os.ErrNotExist)
			require.False(t, matches, "a disappeared input must not count as an identical executable")
			remaining := destination
			if missing == "installed" {
				remaining = staged
			}
			actual, err := os.ReadFile(remaining)
			require.NoError(t, err)
			require.Equal(t, content, actual)
		})
	}
}

func TestRawBinaryMatchesRejectsUnreadableStagedContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	staged, destination := filepath.Join(dir, "staged"), filepath.Join(dir, "installed")
	require.NoError(t, os.Mkdir(staged, 0o755))
	content := []byte("installed executable")
	require.NoError(t, os.WriteFile(destination, content, 0o755))
	expected, err := rawBinarySnapshot(destination)
	require.NoError(t, err)

	matches, err := rawBinaryMatches(staged, destination, expected)
	require.Error(t, err)
	require.False(t, matches, "a failed digest read must not count as matching content")
	actual, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, content, actual)
}

func TestCopyRawBinaryInvalidDestinationPreservesSource(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"missing parent", "filename too long"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			source := filepath.Join(dir, "download")
			content := []byte("verified binary")
			require.NoError(t, os.WriteFile(source, content, 0o755))
			destination := filepath.Join(dir, "missing", "installed")
			if name == "filename too long" {
				destination = filepath.Join(dir, strings.Repeat("x", 300))
			}

			require.ErrorIs(t, (&Installer{}).copyFile(source, destination), ErrFileOperation)
			actual, err := os.ReadFile(source)
			require.NoError(t, err)
			require.Equal(t, content, actual, "failed installation must preserve the downloaded binary")
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 1, "failed installation must leave no staging directory")
			require.Equal(t, "download", entries[0].Name())
		})
	}
}

func TestCopyRawBinaryRejectsDirectoryDestination(t *testing.T) {
	t.Parallel()
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprintf("populated_%t", populated), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			source, destination := filepath.Join(dir, "download"), filepath.Join(dir, "installed")
			content := []byte("verified binary")
			require.NoError(t, os.WriteFile(source, content, 0o755))
			require.NoError(t, os.Mkdir(destination, 0o755))
			if populated {
				require.NoError(t, os.WriteFile(filepath.Join(destination, "sentinel"), content, 0o600))
			}
			require.ErrorIs(t, (&Installer{}).copyFile(source, destination), ErrFileOperation)
			require.DirExists(t, destination, "even an empty destination directory must not be replaced")
			if populated {
				actual, err := os.ReadFile(filepath.Join(destination, "sentinel"))
				require.NoError(t, err)
				require.Equal(t, content, actual)
			}
			actual, err := os.ReadFile(source)
			require.NoError(t, err)
			require.Equal(t, content, actual)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 2, "failed inspection must clean up staged bytes")
		})
	}
}

func TestCopyRawBinaryCannotInspectUnreadableDestination(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits do not deny Windows file reads")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := t.TempDir()
	source, destination := filepath.Join(dir, "download"), filepath.Join(dir, "installed")
	require.NoError(t, os.WriteFile(source, []byte("new executable"), 0o755))
	require.NoError(t, os.WriteFile(destination, []byte("preserved executable"), 0o600))
	require.NoError(t, os.Chmod(destination, 0))
	t.Cleanup(func() { require.NoError(t, os.Chmod(destination, 0o600)) })
	err := (&Installer{}).copyFile(source, destination)
	require.ErrorIs(t, err, ErrFileOperation)
	require.ErrorIs(t, err, os.ErrPermission)
	require.NoError(t, os.Chmod(destination, 0o600))
	actual, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "preserved executable", string(actual))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2, "failed identity capture must remove staged bytes")
}
