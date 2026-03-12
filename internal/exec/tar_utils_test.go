package exec

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTar creates a tar archive in memory from the given entries.
func buildTar(t *testing.T, entries []tar.Header, contents map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i := range entries {
		h := &entries[i]
		data := contents[h.Name]
		h.Size = int64(len(data))
		require.NoError(t, tw.WriteHeader(h))
		if len(data) > 0 {
			_, err := tw.Write([]byte(data))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	return &buf
}

func TestExtractTarball_NormalFiles(t *testing.T) {
	dest := t.TempDir()
	buf := buildTar(t, []tar.Header{
		{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "dir/file.txt", Typeflag: tar.TypeReg, Mode: 0o644},
	}, map[string]string{
		"dir/file.txt": "hello",
	})

	err := extractTarball(buf, dest)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dest, "dir", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

func TestExtractTarball_DirectoryTraversal_DotDot(t *testing.T) {
	dest := t.TempDir()
	buf := buildTar(t, []tar.Header{
		{Name: "../escape.txt", Typeflag: tar.TypeReg, Mode: 0o644},
	}, map[string]string{
		"../escape.txt": "pwned",
	})

	// The .. check in untar() skips this entry silently.
	err := extractTarball(buf, dest)
	require.NoError(t, err)

	// File must NOT exist outside dest.
	_, err = os.Stat(filepath.Join(dest, "..", "escape.txt"))
	assert.True(t, os.IsNotExist(err), "traversal file should not exist")
}

func TestExtractTarball_SymlinkSkipped(t *testing.T) {
	dest := t.TempDir()
	buf := buildTar(t, []tar.Header{
		{Name: "legit.txt", Typeflag: tar.TypeReg, Mode: 0o644},
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777},
	}, map[string]string{
		"legit.txt": "safe",
	})

	err := extractTarball(buf, dest)
	require.NoError(t, err)

	// Legit file extracted.
	_, err = os.Stat(filepath.Join(dest, "legit.txt"))
	require.NoError(t, err)

	// Symlink must NOT be created.
	_, err = os.Lstat(filepath.Join(dest, "link"))
	assert.True(t, os.IsNotExist(err), "symlinks should be skipped")
}

func TestExtractTarball_HardlinkSkipped(t *testing.T) {
	dest := t.TempDir()
	buf := buildTar(t, []tar.Header{
		{Name: "legit.txt", Typeflag: tar.TypeReg, Mode: 0o644},
		{Name: "hardlink", Typeflag: tar.TypeLink, Linkname: "/etc/shadow", Mode: 0o644},
	}, map[string]string{
		"legit.txt": "safe",
	})

	err := extractTarball(buf, dest)
	require.NoError(t, err)

	_, err = os.Lstat(filepath.Join(dest, "hardlink"))
	assert.True(t, os.IsNotExist(err), "hardlinks should be skipped")
}

func TestProcessTarHeader_RejectsPathTraversal(t *testing.T) {
	// Directly call processTarHeader (bypassing untar's ".." filter)
	// to verify the boundary check rejects paths that escape the dest.
	parent := t.TempDir()
	destDir := filepath.Join(parent, "dest")
	require.NoError(t, os.MkdirAll(destDir, 0o755))

	// Build a minimal tar with a traversal entry.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	header := &tar.Header{
		Name:     "../sibling/evil.txt",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     int64(len("pwned")),
	}
	require.NoError(t, tw.WriteHeader(header))
	_, err := tw.Write([]byte("pwned"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	reader := tar.NewReader(&buf)
	_, err = reader.Next()
	require.NoError(t, err)

	// processTarHeader must reject this path.
	err = processTarHeader(header, reader, destDir)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidFilePath), "expected ErrInvalidFilePath, got: %v", err)

	// File must not exist.
	_, statErr := os.Stat(filepath.Join(parent, "sibling", "evil.txt"))
	assert.True(t, os.IsNotExist(statErr), "traversal file should not be created")
}

func TestProcessTarHeader_AcceptsValidPath(t *testing.T) {
	// Verify processTarHeader accepts a normal path within dest.
	parent := t.TempDir()
	destDir := filepath.Join(parent, "dest")
	require.NoError(t, os.MkdirAll(destDir, 0o755))

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	header := &tar.Header{
		Name:     "subdir/file.txt",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     int64(len("ok")),
	}
	require.NoError(t, tw.WriteHeader(header))
	_, err := tw.Write([]byte("ok"))
	require.NoError(t, err)
	reader := tar.NewReader(&buf)
	_, err = reader.Next()
	require.NoError(t, err)

	err = processTarHeader(header, reader, destDir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destDir, "subdir", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ok", string(content))
}

func TestExtractTarball_StripSetuidSetgid(t *testing.T) {
	dest := t.TempDir()
	buf := buildTar(t, []tar.Header{
		{Name: "setuid_file", Typeflag: tar.TypeReg, Mode: 0o6755},
	}, map[string]string{
		"setuid_file": "dangerous",
	})

	err := extractTarball(buf, dest)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dest, "setuid_file"))
	require.NoError(t, err)

	// Setuid and setgid bits must be stripped.
	mode := info.Mode()
	assert.Zero(t, mode&os.ModeSetuid, "setuid bit should be stripped")
	assert.Zero(t, mode&os.ModeSetgid, "setgid bit should be stripped")
}
