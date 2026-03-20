//go:build !windows

package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestGetGlobMatches_CacheHit verifies that GetGlobMatches returns cached results
// on a second call with the same pattern, without re-reading the filesystem.
func TestGetGlobMatches_CacheHit(t *testing.T) {
	// Use a fresh cache state by clearing it.
	ResetGlobMatchesCache()

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.yaml"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b.yaml"), []byte(""), 0o644))

	pattern := filepath.Join(tmpDir, "*.yaml")

	// First call - cache miss.
	first, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Len(t, first, 2)

	// Second call with same pattern - should hit cache.
	second, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Len(t, second, 2)

	// Results should be equal.
	assert.ElementsMatch(t, first, second)
}

// TestGetGlobMatches_CacheIsolation verifies that cached results are cloned, so
// mutating the returned slice does not corrupt subsequent calls.
func TestGetGlobMatches_CacheIsolation(t *testing.T) {
	ResetGlobMatchesCache()

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "c.yaml"), []byte(""), 0o644))

	pattern := filepath.Join(tmpDir, "*.yaml")

	first, err := GetGlobMatches(pattern)
	require.NoError(t, err)

	// Mutate the returned slice.
	if len(first) > 0 {
		first[0] = "mutated"
	}

	// Second call should still return the original value.
	second, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	if len(second) > 0 {
		assert.NotEqual(t, "mutated", second[0])
	}
}

// TestGetGlobMatches_NonExistentBaseDir verifies that GetGlobMatches returns an
// appropriate error when the base directory does not exist.
func TestGetGlobMatches_NonExistentBaseDir(t *testing.T) {
	ResetGlobMatchesCache()

	// Build a path guaranteed to not exist by using a non-existent sub-directory
	// of a fresh t.TempDir() (which will be cleaned up, but we never create the subdir).
	pattern := filepath.Join(t.TempDir(), "nonexistent", "*.yaml")
	_, err := GetGlobMatches(pattern)
	require.Error(t, err, "expected error for non-existent base directory")
	assert.True(t, errors.Is(err, errUtils.ErrFailedToFindImport), "expected ErrFailedToFindImport, got: %v", err)
}

// TestGetGlobMatches_EmptyResults verifies that a pattern matching no files returns
// an empty slice (not an error).
func TestGetGlobMatches_EmptyResults(t *testing.T) {
	ResetGlobMatchesCache()

	tmpDir := t.TempDir()
	// No files created in tmpDir.

	pattern := filepath.Join(tmpDir, "*.yaml")
	matches, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

// TestGetGlobMatches_EmptyResultsCache verifies that empty results are cached and
// retrieved without hitting the filesystem again.
func TestGetGlobMatches_EmptyResultsCache(t *testing.T) {
	ResetGlobMatchesCache()

	tmpDir := t.TempDir()

	pattern := filepath.Join(tmpDir, "*.nonexistent")

	// First call - should return empty (not nil) slice and cache it.
	first, err := GetGlobMatches(pattern)
	require.NoError(t, err)

	// Second call should use cache.
	second, err := GetGlobMatches(pattern)
	require.NoError(t, err)

	// Both should be strictly equal — same type (non-nil empty slice), same content.
	// This catches a nil vs []string{} inconsistency between the first and cached call.
	assert.Equal(t, first, second)
}

// TestPathMatch_CacheHit verifies that the PathMatch cache is used on repeated calls.
func TestPathMatch_CacheHit(t *testing.T) {
	// Clear the path match cache using the exported test helper.
	ResetPathMatchCache()

	pattern := "stacks/**/*.yaml"
	name := "stacks/dev/vpc.yaml"

	// First call - cache miss.
	first, err := PathMatch(pattern, name)
	require.NoError(t, err)

	// Second call - should hit cache.
	second, err := PathMatch(pattern, name)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.True(t, first, "pattern should match the name")
}

// TestPathMatch_CacheHit_NoMatch verifies that cache entries for non-matching patterns
// are also cached and returned correctly.
func TestPathMatch_CacheHit_NoMatch(t *testing.T) {
	ResetPathMatchCache()

	pattern := "*.go"
	name := "file.yaml"

	// First call - cache miss.
	first, err := PathMatch(pattern, name)
	require.NoError(t, err)
	assert.False(t, first)

	// Second call - should hit cache.
	second, err := PathMatch(pattern, name)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

// TestPathMatch_InvalidPattern verifies that an invalid glob pattern returns an error.
func TestPathMatch_InvalidPattern(t *testing.T) {
	ResetPathMatchCache()

	// An invalid pattern with unclosed bracket.
	_, err := PathMatch("[invalid", "file.yaml")
	// doublestar.Match returns an error for invalid patterns.
	assert.Error(t, err)
}

// TestWriteFileAtomic verifies that WriteFileAtomic writes file contents correctly.
func TestWriteFileAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "atomic-test.txt")
	content := []byte("hello atomic world")

	err := WriteFileAtomicUnix(filePath, content, 0o644)
	require.NoError(t, err)

	// Verify file was written.
	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestWriteFileAtomic_Overwrite verifies that WriteFileAtomic correctly overwrites
// an existing file atomically.
func TestWriteFileAtomic_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "atomic-overwrite.txt")

	// Write initial content.
	require.NoError(t, os.WriteFile(filePath, []byte("initial content"), 0o644))

	// Overwrite with atomic write.
	newContent := []byte("new content")
	err := WriteFileAtomicUnix(filePath, newContent, 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, newContent, got)
}

// TestOSFileSystem_WriteFileAtomic verifies that OSFileSystem.WriteFileAtomic works.
func TestOSFileSystem_WriteFileAtomic(t *testing.T) {
	fs := NewOSFileSystem()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "os-atomic.txt")
	content := []byte("atomic content via OSFileSystem")

	err := fs.WriteFileAtomic(filePath, content, 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestOSFileSystem_WriteFileAtomic_Overwrite verifies that OSFileSystem.WriteFileAtomic
// correctly overwrites an existing file atomically.
func TestOSFileSystem_WriteFileAtomic_Overwrite(t *testing.T) {
	fs := NewOSFileSystem()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "os-atomic-overwrite.txt")

	// Write initial content.
	require.NoError(t, os.WriteFile(filePath, []byte("initial content"), 0o644))

	// Overwrite with atomic write.
	newContent := []byte("overwritten content via OSFileSystem")
	err := fs.WriteFileAtomic(filePath, newContent, 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, newContent, got)
}
