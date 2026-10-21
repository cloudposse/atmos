package installer

import (
	"os"
	"path/filepath"
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
