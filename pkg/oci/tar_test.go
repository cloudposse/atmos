package oci

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestTar(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	return &buf
}

func TestExtractTarball_WritesFiles(t *testing.T) {
	dest := t.TempDir()
	tarBuf := writeTestTar(t, map[string]string{
		"main.tf":          "resource \"null_resource\" \"x\" {}\n",
		"nested/output.tf": "output \"x\" {}\n",
	})

	require.NoError(t, extractTarball(tarBuf, dest))

	content, err := os.ReadFile(filepath.Join(dest, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "null_resource")

	nested, err := os.ReadFile(filepath.Join(dest, "nested", "output.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(nested), "output")
}

func TestExtractTarball_SkipsDirectoryTraversal(t *testing.T) {
	dest := t.TempDir()
	tarBuf := writeTestTar(t, map[string]string{
		"../../etc/passwd": "malicious\n",
	})

	require.NoError(t, extractTarball(tarBuf, dest))

	entries, err := os.ReadDir(dest)
	require.NoError(t, err)
	assert.Empty(t, entries, "traversal entry must not be written to dest")
}

// TestExtractTarball_RejectsAbsolutePath exercises processTarHeader's
// SafeJoin containment check directly, rather than untar's earlier ".."
// substring filter, since an absolute entry name contains no "..", so it
// reaches processTarHeader, where SafeJoin must still reject it.
func TestExtractTarball_RejectsAbsolutePath(t *testing.T) {
	dest := t.TempDir()
	tarBuf := writeTestTar(t, map[string]string{
		"/etc/passwd": "malicious\n",
	})

	err := extractTarball(tarBuf, dest)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFilePath)

	entries, readErr := os.ReadDir(dest)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "absolute-path entry must not be written to dest")
}

// TestExtractTarball_CreatesDirectoryEntries exercises processTarHeader's
// tar.TypeDir case, which writeTestTar's regular-file-only helper can't
// produce.
func TestExtractTarball_CreatesDirectoryEntries(t *testing.T) {
	dest := t.TempDir()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "emptydir/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}))
	require.NoError(t, tw.Close())

	require.NoError(t, extractTarball(&buf, dest))

	info, err := os.Stat(filepath.Join(dest, "emptydir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestExtractTarball_SkipsUnsupportedType exercises processTarHeader's
// default case (an entry type that's neither a regular file nor a
// directory, e.g. a symlink) -- it's logged and skipped, not an error.
func TestExtractTarball_SkipsUnsupportedType(t *testing.T) {
	dest := t.TempDir()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "link",
		Typeflag: tar.TypeSymlink,
		Linkname: "main.tf",
		Mode:     0o644,
	}))
	require.NoError(t, tw.Close())

	require.NoError(t, extractTarball(&buf, dest))

	entries, err := os.ReadDir(dest)
	require.NoError(t, err)
	assert.Empty(t, entries, "unsupported entry type must not be written to dest")
}

// TestExtractTarball_FailsWhenParentDirBlocked exercises createFileFromTar's
// os.MkdirAll error branch: a regular file sitting where a parent directory
// needs to be created makes MkdirAll fail.
func TestExtractTarball_FailsWhenParentDirBlocked(t *testing.T) {
	dest := t.TempDir()
	blocker := filepath.Join(dest, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	tarBuf := writeTestTar(t, map[string]string{
		"blocker/nested/file.txt": "content\n",
	})

	err := extractTarball(tarBuf, dest)
	require.Error(t, err)
}

// TestExtractTarball_RejectsMalformedArchive exercises untar's tarReader.Next
// parse-error branch.
func TestExtractTarball_RejectsMalformedArchive(t *testing.T) {
	dest := t.TempDir()
	err := extractTarball(strings.NewReader("this is not a tar file, and it is long enough to look like a header block but isn't"), dest)
	require.Error(t, err)
}

// TestExtractTarball_DirectoryEntryFailsWhenParentDirBlocked exercises
// createDirectory's os.MkdirAll error branch via the tar.TypeDir case: a
// regular file sitting where a parent directory needs to be created makes
// MkdirAll fail.
func TestExtractTarball_DirectoryEntryFailsWhenParentDirBlocked(t *testing.T) {
	dest := t.TempDir()
	blocker := filepath.Join(dest, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "blocker/subdir/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}))
	require.NoError(t, tw.Close())

	err := extractTarball(&buf, dest)
	require.Error(t, err)
}
