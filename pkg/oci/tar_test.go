package oci

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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

// TestExtractTarball_RejectsDirectoryTraversal exercises processTarHeader's
// SafeJoin containment check: untar has no ".."-substring prefilter of its
// own (removed -- it was overbroad and dropped valid filenames like
// "..hidden.txt", see TestExtractTarball_ExtractsFilenameContainingDotDot),
// so SafeJoin is the sole gate and must fail loudly rather than silently skip.
func TestExtractTarball_RejectsDirectoryTraversal(t *testing.T) {
	dest := t.TempDir()
	tarBuf := writeTestTar(t, map[string]string{
		"../../etc/passwd": "malicious\n",
	})

	err := extractTarball(tarBuf, dest)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFilePath)

	entries, readErr := os.ReadDir(dest)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "traversal entry must not be written to dest")
}

// TestExtractTarball_ExtractsFilenameContainingDotDot is a regression test: a
// valid filename that merely contains ".." as a substring without it being a
// full path-traversal component (e.g. "..hidden.txt") must still be
// extracted. SafeJoin already rejects only actual ".." path components; the
// removed prefilter in untar used to reject this too, via a much broader
// substring check.
func TestExtractTarball_ExtractsFilenameContainingDotDot(t *testing.T) {
	dest := t.TempDir()
	tarBuf := writeTestTar(t, map[string]string{
		"..hidden.txt": "content\n",
	})

	require.NoError(t, extractTarball(tarBuf, dest))

	content, err := os.ReadFile(filepath.Join(dest, "..hidden.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content\n", string(content))
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

// TestExtractTarball_RejectsWriteThroughPreExistingSymlink is a regression
// test for CWE-59 (symlink-following): SafeJoin only validates an entry name
// lexically, so before extraction switched to os.Root, a pre-existing
// symlink inside dest pointing outside it (e.g. left over from a prior run,
// or planted via an unrelated mechanism) let a plain-looking entry name like
// "link/evil.txt" escape dest via os.MkdirAll/os.Create following the
// symlink -- os.Root refuses to resolve a path through a symlink that would
// leave the root.
func TestExtractTarball_RejectsWriteThroughPreExistingSymlink(t *testing.T) {
	dest := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(dest, "link")
	require.NoError(t, os.Symlink(outside, link))

	tarBuf := writeTestTar(t, map[string]string{
		"link/evil.txt": "escaped\n",
	})

	err := extractTarball(tarBuf, dest)
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(outside, "evil.txt"))
	assert.True(t, os.IsNotExist(statErr), "entry must not be written through the symlink to outside")
}

// TestExtractTarball_FailsWhenExtractPathBlocked exercises untar's
// os.MkdirAll(extractPath) error branch: a regular file sitting where
// extractPath itself needs to be created makes MkdirAll fail.
func TestExtractTarball_FailsWhenExtractPathBlocked(t *testing.T) {
	parent := t.TempDir()
	blocked := filepath.Join(parent, "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))

	tarBuf := writeTestTar(t, map[string]string{"file.txt": "content\n"})

	err := extractTarball(tarBuf, filepath.Join(blocked, "nested"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create extraction directory")
}

// TestExtractTarball_FailsWhenExtractPathUnreadable exercises untar's
// os.OpenRoot(extractPath) error branch: MkdirAll succeeds (the directory
// already exists), but a directory with no permissions cannot be opened.
func TestExtractTarball_FailsWhenExtractPathUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permission bits")
	}

	dest := t.TempDir()
	require.NoError(t, os.Chmod(dest, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dest, 0o755) }) // Allow t.TempDir's own cleanup to remove it.

	tarBuf := writeTestTar(t, map[string]string{"file.txt": "content\n"})

	err := extractTarball(tarBuf, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open extraction root")
}

// TestExtractTarball_FailsWhenTargetIsExistingDir exercises
// createFileFromTar's root.Create error branch: a directory already sitting
// at the entry's path makes Create fail (can't open a directory for writing).
func TestExtractTarball_FailsWhenTargetIsExistingDir(t *testing.T) {
	dest := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dest, "file.txt"), 0o755))

	tarBuf := writeTestTar(t, map[string]string{"file.txt": "content\n"})

	err := extractTarball(tarBuf, dest)
	require.Error(t, err)
}
