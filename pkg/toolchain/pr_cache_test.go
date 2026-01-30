package toolchain

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// setupTestInstallPath configures the toolchain to use a temporary directory.
// Returns a cleanup function that restores the original config.
func setupTestInstallPath(t *testing.T, tempDir string) func() {
	t.Helper()

	originalConfig := atmosConfig
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})
	return func() {
		atmosConfig = originalConfig
	}
}

func TestPRCacheMetadata_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create the PR directory structure.
	prDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "pr-999")
	require.NoError(t, os.MkdirAll(prDir, 0o755))

	// Create a fake binary so CheckPRCacheStatus finds something.
	binaryPath := filepath.Join(prDir, "atmos")
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake"), 0o755))

	// Save cache metadata.
	now := time.Now()
	meta := &PRCacheMetadata{
		HeadSHA:   "abc123def456",
		CheckedAt: now,
		RunID:     12345,
	}
	err := savePRCacheMetadata(999, meta)
	require.NoError(t, err)

	// Verify file was created.
	cacheFile := filepath.Join(prDir, cacheMetadataFile)
	assert.FileExists(t, cacheFile)

	// Load and verify.
	loaded, err := loadPRCacheMetadata(999)
	require.NoError(t, err)
	assert.Equal(t, meta.HeadSHA, loaded.HeadSHA)
	assert.Equal(t, meta.RunID, loaded.RunID)
	assert.WithinDuration(t, now, loaded.CheckedAt, time.Second)
}

func TestCheckPRCacheStatus_NoBinary(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	status, path := CheckPRCacheStatus(999)
	assert.Equal(t, PRCacheNeedsInstall, status)
	assert.Empty(t, path)
}

func TestCheckPRCacheStatus_BinaryWithValidCache(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create the PR directory structure with binary.
	prDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "pr-888")
	require.NoError(t, os.MkdirAll(prDir, 0o755))
	binaryPath := filepath.Join(prDir, "atmos")
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake"), 0o755))

	// Save recent cache metadata (within TTL).
	meta := &PRCacheMetadata{
		HeadSHA:   "abc123",
		CheckedAt: time.Now(), // Just now, definitely within TTL.
		RunID:     12345,
	}
	require.NoError(t, savePRCacheMetadata(888, meta))

	status, path := CheckPRCacheStatus(888)
	assert.Equal(t, PRCacheValid, status)
	assert.Equal(t, binaryPath, path)
}

func TestCheckPRCacheStatus_BinaryWithExpiredCache(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create the PR directory structure with binary.
	prDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "pr-777")
	require.NoError(t, os.MkdirAll(prDir, 0o755))
	binaryPath := filepath.Join(prDir, "atmos")
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake"), 0o755))

	// Save old cache metadata (outside TTL).
	meta := &PRCacheMetadata{
		HeadSHA:   "abc123",
		CheckedAt: time.Now().Add(-2 * time.Minute), // 2 minutes ago, outside 1 minute TTL.
		RunID:     12345,
	}
	require.NoError(t, savePRCacheMetadata(777, meta))

	status, path := CheckPRCacheStatus(777)
	assert.Equal(t, PRCacheNeedsCheck, status)
	assert.Equal(t, binaryPath, path)
}

func TestCheckPRCacheStatus_BinaryWithNoCache(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create the PR directory structure with binary only.
	prDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "pr-666")
	require.NoError(t, os.MkdirAll(prDir, 0o755))
	binaryPath := filepath.Join(prDir, "atmos")
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake"), 0o755))

	// No cache metadata saved.

	status, path := CheckPRCacheStatus(666)
	assert.Equal(t, PRCacheNeedsCheck, status) // Should check API since no metadata.
	assert.Equal(t, binaryPath, path)
}

func TestLoadPRCacheMetadata_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	meta, err := loadPRCacheMetadata(999)
	assert.Error(t, err)
	assert.Nil(t, meta)
}

func TestLoadPRCacheMetadata_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create the PR directory structure.
	prDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "pr-555")
	require.NoError(t, os.MkdirAll(prDir, 0o755))

	// Write invalid JSON.
	cacheFile := filepath.Join(prDir, cacheMetadataFile)
	require.NoError(t, os.WriteFile(cacheFile, []byte("not json"), 0o644))

	meta, err := loadPRCacheMetadata(555)
	assert.Error(t, err)
	assert.Nil(t, meta)
}

func TestPRCacheTTL_Value(t *testing.T) {
	// Verify the TTL constant is 1 minute as documented.
	assert.Equal(t, 1*time.Minute, PRCacheTTL)
}
