package oci

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestZip(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return &buf
}

func TestExtractZip_WritesFiles(t *testing.T) {
	dest := t.TempDir()
	zipBuf := writeTestZip(t, map[string]string{
		"main.tf":          "resource \"null_resource\" \"x\" {}\n",
		"nested/output.tf": "output \"x\" {}\n",
	})

	require.NoError(t, extractZip(zipBuf, dest))

	content, err := os.ReadFile(filepath.Join(dest, "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "null_resource")

	nested, err := os.ReadFile(filepath.Join(dest, "nested", "output.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(nested), "output")
}

// TestExtractZip_RejectsDirectoryTraversal exercises processZipFile's
// SafeJoin containment check: extractZip has no ".."-substring prefilter of
// its own (removed -- it was overbroad and dropped valid filenames like
// "..hidden.txt", see TestExtractZip_ExtractsFilenameContainingDotDot), so
// SafeJoin is the sole gate and must fail loudly rather than silently skip.
func TestExtractZip_RejectsDirectoryTraversal(t *testing.T) {
	dest := t.TempDir()
	zipBuf := writeTestZip(t, map[string]string{
		"../../etc/passwd": "malicious\n",
	})

	err := extractZip(zipBuf, dest)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFilePath)

	entries, readErr := os.ReadDir(dest)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "traversal entry must not be written to dest")
}

// TestExtractZip_ExtractsFilenameContainingDotDot is a regression test: a
// valid filename that merely contains ".." as a substring without it being a
// full path-traversal component (e.g. "..hidden.txt") must still be
// extracted. SafeJoin already rejects only actual ".." path components (see
// TestSafeJoin's "name that merely starts with dotdot" case); the removed
// prefilter in extractZip used to reject this too, via a much broader
// substring check.
func TestExtractZip_ExtractsFilenameContainingDotDot(t *testing.T) {
	dest := t.TempDir()
	zipBuf := writeTestZip(t, map[string]string{
		"..hidden.txt": "content\n",
	})

	require.NoError(t, extractZip(zipBuf, dest))

	content, err := os.ReadFile(filepath.Join(dest, "..hidden.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content\n", string(content))
}

// TestExtractZip_RejectsAbsolutePath exercises processZipFile's SafeJoin
// containment check directly, rather than extractZip's earlier ".."
// substring filter, since an absolute entry name contains no "..", so it
// reaches processZipFile, where SafeJoin must still reject it.
func TestExtractZip_RejectsAbsolutePath(t *testing.T) {
	dest := t.TempDir()
	zipBuf := writeTestZip(t, map[string]string{
		"/etc/passwd": "malicious\n",
	})

	err := extractZip(zipBuf, dest)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFilePath)

	entries, readErr := os.ReadDir(dest)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "absolute-path entry must not be written to dest")
}

// TestExtractZip_CreatesDirectoryEntries exercises processZipFile's
// file.FileInfo().IsDir() branch: a zip entry name ending in "/" is a
// directory marker (archive/zip sets fs.ModeDir for any such name), which
// must be created as an empty directory rather than passed to
// createFileFromZip.
func TestExtractZip_CreatesDirectoryEntries(t *testing.T) {
	dest := t.TempDir()
	zipBuf := writeTestZip(t, map[string]string{
		"emptydir/": "",
	})

	require.NoError(t, extractZip(zipBuf, dest))

	info, err := os.Stat(filepath.Join(dest, "emptydir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestExtractZip_RejectsMalformedArchive exercises extractZip's zip.NewReader
// parse-error branch.
func TestExtractZip_RejectsMalformedArchive(t *testing.T) {
	dest := t.TempDir()
	err := extractZip(strings.NewReader("this is not a zip file"), dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse zip archive")
}

// TestExtractZip_FailsWhenParentDirBlocked exercises createFileFromZip's
// os.MkdirAll error branch: a regular file sitting where a parent directory
// needs to be created makes MkdirAll fail.
func TestExtractZip_FailsWhenParentDirBlocked(t *testing.T) {
	dest := t.TempDir()
	blocker := filepath.Join(dest, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	zipBuf := writeTestZip(t, map[string]string{
		"blocker/nested/file.txt": "content\n",
	})

	err := extractZip(zipBuf, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create parent directory")
}

// TestExtractZip_DirectoryEntryFailsWhenParentDirBlocked exercises
// createDirectory's os.MkdirAll error branch via the IsDir() case: a regular
// file sitting where a parent directory needs to be created makes MkdirAll
// fail.
func TestExtractZip_DirectoryEntryFailsWhenParentDirBlocked(t *testing.T) {
	dest := t.TempDir()
	blocker := filepath.Join(dest, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	zipBuf := writeTestZip(t, map[string]string{
		"blocker/subdir/": "",
	})

	err := extractZip(zipBuf, dest)
	require.Error(t, err)
}

// TestExtractZip_RejectsWriteThroughPreExistingSymlink is a regression test
// for CWE-59 (symlink-following): SafeJoin only validates an entry name
// lexically, so before extraction switched to os.Root, a pre-existing
// symlink inside dest pointing outside it (e.g. left over from a prior run,
// or planted via an unrelated mechanism) let a plain-looking entry name like
// "link/evil.txt" escape dest via os.MkdirAll/os.Create following the
// symlink -- os.Root refuses to resolve a path through a symlink that would
// leave the root.
func TestExtractZip_RejectsWriteThroughPreExistingSymlink(t *testing.T) {
	dest := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(dest, "link")
	require.NoError(t, os.Symlink(outside, link))

	zipBuf := writeTestZip(t, map[string]string{
		"link/evil.txt": "escaped\n",
	})

	err := extractZip(zipBuf, dest)
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(outside, "evil.txt"))
	assert.True(t, os.IsNotExist(statErr), "entry must not be written through the symlink to outside")
}
