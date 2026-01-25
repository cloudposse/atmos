package workdir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for fileNeedsCopy function.

func TestFileNeedsCopy_DestNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.txt")
	dstFile := filepath.Join(tmpDir, "dst.txt")

	require.NoError(t, os.WriteFile(srcFile, []byte("content"), 0o644))

	hasher := NewDefaultHasher()
	assert.True(t, fileNeedsCopy(srcFile, dstFile, hasher))
}

func TestFileNeedsCopy_SameContent(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.txt")
	dstFile := filepath.Join(tmpDir, "dst.txt")

	require.NoError(t, os.WriteFile(srcFile, []byte("content"), 0o644))
	require.NoError(t, os.WriteFile(dstFile, []byte("content"), 0o644))

	hasher := NewDefaultHasher()
	assert.False(t, fileNeedsCopy(srcFile, dstFile, hasher))
}

func TestFileNeedsCopy_DifferentContent(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.txt")
	dstFile := filepath.Join(tmpDir, "dst.txt")

	require.NoError(t, os.WriteFile(srcFile, []byte("content-src"), 0o644))
	require.NoError(t, os.WriteFile(dstFile, []byte("content-dst"), 0o644))

	hasher := NewDefaultHasher()
	assert.True(t, fileNeedsCopy(srcFile, dstFile, hasher))
}

func TestFileNeedsCopy_DifferentPermissions(t *testing.T) {
	if os.Getenv("GOOS") == "windows" || filepath.Separator == '\\' {
		t.Skip("Skipping permission test on Windows - Unix file permissions not supported")
	}
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.txt")
	dstFile := filepath.Join(tmpDir, "dst.txt")

	require.NoError(t, os.WriteFile(srcFile, []byte("content"), 0o755))
	require.NoError(t, os.WriteFile(dstFile, []byte("content"), 0o644))

	hasher := NewDefaultHasher()
	assert.True(t, fileNeedsCopy(srcFile, dstFile, hasher))
}

func TestFileNeedsCopy_SourceNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "nonexistent.txt")
	dstFile := filepath.Join(tmpDir, "dst.txt")

	require.NoError(t, os.WriteFile(dstFile, []byte("content"), 0o644))

	hasher := NewDefaultHasher()
	// Returns true because source can't be read.
	assert.True(t, fileNeedsCopy(srcFile, dstFile, hasher))
}

// Tests for deleteRemovedFiles function.

func TestDeleteRemovedFiles_SkipsAtmosDir(t *testing.T) {
	tmpDir := t.TempDir()
	dstDir := filepath.Join(tmpDir, "dst")
	require.NoError(t, os.MkdirAll(dstDir, 0o755))

	// Create .atmos directory with a file.
	atmosDir := filepath.Join(dstDir, AtmosDir)
	require.NoError(t, os.MkdirAll(atmosDir, 0o755))
	atmosFile := filepath.Join(atmosDir, "metadata.json")
	require.NoError(t, os.WriteFile(atmosFile, []byte("{}"), 0o644))

	// Create a regular file that should be deleted.
	regularFile := filepath.Join(dstDir, "old.txt")
	require.NoError(t, os.WriteFile(regularFile, []byte("old content"), 0o644))

	srcFiles := map[string]bool{} // Empty - all files should be considered for deletion.

	deleted, err := deleteRemovedFiles(dstDir, srcFiles)
	require.NoError(t, err)
	assert.True(t, deleted)

	// .atmos file should still exist.
	_, err = os.Stat(atmosFile)
	assert.NoError(t, err, ".atmos/metadata.json should not be deleted")

	// Regular file should be deleted.
	_, err = os.Stat(regularFile)
	assert.True(t, os.IsNotExist(err), "old.txt should be deleted")
}

func TestDeleteRemovedFiles_KeepsFilesInSrcFiles(t *testing.T) {
	tmpDir := t.TempDir()
	dstDir := filepath.Join(tmpDir, "dst")
	require.NoError(t, os.MkdirAll(dstDir, 0o755))

	// Create files.
	keepFile := filepath.Join(dstDir, "keep.txt")
	deleteFile := filepath.Join(dstDir, "delete.txt")
	require.NoError(t, os.WriteFile(keepFile, []byte("keep"), 0o644))
	require.NoError(t, os.WriteFile(deleteFile, []byte("delete"), 0o644))

	srcFiles := map[string]bool{
		"keep.txt": true, // This one should be kept.
	}

	deleted, err := deleteRemovedFiles(dstDir, srcFiles)
	require.NoError(t, err)
	assert.True(t, deleted)

	// keep.txt should still exist.
	_, err = os.Stat(keepFile)
	assert.NoError(t, err, "keep.txt should not be deleted")

	// delete.txt should be deleted.
	_, err = os.Stat(deleteFile)
	assert.True(t, os.IsNotExist(err), "delete.txt should be deleted")
}

func TestDeleteRemovedFiles_NoFilesToDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dstDir := filepath.Join(tmpDir, "dst")
	require.NoError(t, os.MkdirAll(dstDir, 0o755))

	keepFile := filepath.Join(dstDir, "keep.txt")
	require.NoError(t, os.WriteFile(keepFile, []byte("keep"), 0o644))

	srcFiles := map[string]bool{
		"keep.txt": true,
	}

	deleted, err := deleteRemovedFiles(dstDir, srcFiles)
	require.NoError(t, err)
	assert.False(t, deleted, "no files should be deleted")
}

// Tests for copyFile function.

func TestCopyFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.txt")
	dstFile := filepath.Join(tmpDir, "dst.txt")

	content := []byte("test content")
	require.NoError(t, os.WriteFile(srcFile, content, 0o644))

	err := copyFile(srcFile, dstFile)
	require.NoError(t, err)

	// Verify content was copied.
	dstContent, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, content, dstContent)
}

func TestCopyFile_CreatesParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.txt")
	dstFile := filepath.Join(tmpDir, "subdir", "nested", "dst.txt")

	content := []byte("test content")
	require.NoError(t, os.WriteFile(srcFile, content, 0o644))

	err := copyFile(srcFile, dstFile)
	require.NoError(t, err)

	// Verify content was copied.
	dstContent, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, content, dstContent)
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "nonexistent.txt")
	dstFile := filepath.Join(tmpDir, "dst.txt")

	err := copyFile(srcFile, dstFile)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestCopyFile_PreservesPermissions(t *testing.T) {
	if os.Getenv("GOOS") == "windows" || filepath.Separator == '\\' {
		t.Skip("Skipping permission test on Windows - Unix file permissions not supported")
	}
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.txt")
	dstFile := filepath.Join(tmpDir, "dst.txt")

	content := []byte("executable content")
	require.NoError(t, os.WriteFile(srcFile, content, 0o755))

	err := copyFile(srcFile, dstFile)
	require.NoError(t, err)

	// Verify permissions were preserved.
	srcInfo, err := os.Stat(srcFile)
	require.NoError(t, err)
	dstInfo, err := os.Stat(dstFile)
	require.NoError(t, err)

	assert.Equal(t, srcInfo.Mode().Perm(), dstInfo.Mode().Perm())
}

// Tests for SyncDir function.

func TestSyncDir_NewFilesCopied(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.MkdirAll(dstDir, 0o755))

	// Create source file.
	srcFile := filepath.Join(srcDir, "new.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("new content"), 0o644))

	fs := NewDefaultFileSystem()
	hasher := NewDefaultHasher()

	changed, err := fs.SyncDir(srcDir, dstDir, hasher)
	require.NoError(t, err)
	assert.True(t, changed)

	// Verify file was copied.
	dstFile := filepath.Join(dstDir, "new.txt")
	_, err = os.Stat(dstFile)
	assert.NoError(t, err)
}

func TestSyncDir_NoChanges(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.MkdirAll(dstDir, 0o755))

	// Create identical files in both directories.
	content := []byte("same content")
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file.txt"), content, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "file.txt"), content, 0o644))

	fs := NewDefaultFileSystem()
	hasher := NewDefaultHasher()

	changed, err := fs.SyncDir(srcDir, dstDir, hasher)
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestSyncDir_RemovesDeletedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.MkdirAll(dstDir, 0o755))

	// Create a file in dst that doesn't exist in src.
	oldFile := filepath.Join(dstDir, "old.txt")
	require.NoError(t, os.WriteFile(oldFile, []byte("old content"), 0o644))

	fs := NewDefaultFileSystem()
	hasher := NewDefaultHasher()

	changed, err := fs.SyncDir(srcDir, dstDir, hasher)
	require.NoError(t, err)
	assert.True(t, changed)

	// Verify file was deleted.
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))
}

func TestSyncDir_SkipsAtmosDir(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.MkdirAll(dstDir, 0o755))

	// Create .atmos in src - should be skipped during sync.
	srcAtmos := filepath.Join(srcDir, AtmosDir)
	require.NoError(t, os.MkdirAll(srcAtmos, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcAtmos, "src.json"), []byte("{}"), 0o644))

	// Create .atmos in dst - should not be deleted.
	dstAtmos := filepath.Join(dstDir, AtmosDir)
	require.NoError(t, os.MkdirAll(dstAtmos, 0o755))
	dstMetaFile := filepath.Join(dstAtmos, "metadata.json")
	require.NoError(t, os.WriteFile(dstMetaFile, []byte(`{"test": true}`), 0o644))

	fs := NewDefaultFileSystem()
	hasher := NewDefaultHasher()

	changed, err := fs.SyncDir(srcDir, dstDir, hasher)
	require.NoError(t, err)
	// No changes since .atmos is skipped in both src and dst.
	assert.False(t, changed)

	// dst .atmos file should still exist with original content.
	content, err := os.ReadFile(dstMetaFile)
	require.NoError(t, err)
	assert.Equal(t, `{"test": true}`, string(content))
}
