package workdir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Test UpdateLastAccessed function.

func TestUpdateLastAccessed_Success(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create initial metadata.
	initialTime := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	metadata := &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "dev",
		SourceType:   SourceTypeLocal,
		Source:       "components/terraform/vpc",
		CreatedAt:    initialTime,
		UpdatedAt:    initialTime,
		LastAccessed: initialTime,
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	// Update last accessed.
	beforeUpdate := time.Now()
	err := UpdateLastAccessed(workdirPath)
	afterUpdate := time.Now()
	require.NoError(t, err)

	// Read metadata and verify LastAccessed was updated.
	updated, err := ReadMetadata(workdirPath)
	require.NoError(t, err)
	require.NotNil(t, updated)

	// LastAccessed should be between beforeUpdate and afterUpdate.
	assert.True(t, updated.LastAccessed.After(beforeUpdate) || updated.LastAccessed.Equal(beforeUpdate))
	assert.True(t, updated.LastAccessed.Before(afterUpdate) || updated.LastAccessed.Equal(afterUpdate))

	// Other fields should remain unchanged.
	assert.Equal(t, "vpc", updated.Component)
	assert.Equal(t, "dev", updated.Stack)
	assert.True(t, initialTime.Equal(updated.CreatedAt))
}

func TestUpdateLastAccessed_NoMetadataFile(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "no-metadata")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// No metadata file exists - should return nil (not an error).
	err := UpdateLastAccessed(workdirPath)
	assert.NoError(t, err)
}

func TestUpdateLastAccessed_LegacyLocation(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "legacy-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create metadata in legacy location (directly in workdir, not in .atmos/).
	initialTime := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	metadata := &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "dev",
		SourceType:   SourceTypeLocal,
		Source:       "components/terraform/vpc",
		CreatedAt:    initialTime,
		UpdatedAt:    initialTime,
		LastAccessed: initialTime,
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	require.NoError(t, err)

	legacyPath := filepath.Join(workdirPath, WorkdirMetadataFile)
	require.NoError(t, os.WriteFile(legacyPath, data, 0o644))

	// Update last accessed should work with legacy location.
	err = UpdateLastAccessed(workdirPath)
	require.NoError(t, err)

	// Verify the update happened on the legacy file.
	updatedData, err := os.ReadFile(legacyPath)
	require.NoError(t, err)

	var updated WorkdirMetadata
	require.NoError(t, json.Unmarshal(updatedData, &updated))

	assert.True(t, updated.LastAccessed.After(initialTime))
}

// Test readMetadataUnlocked function.

func TestReadMetadataUnlocked_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create metadata file directly.
	metadata := &WorkdirMetadata{
		Component:  "vpc",
		Stack:      "dev",
		SourceType: SourceTypeLocal,
		Source:     "components/terraform/vpc",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	require.NoError(t, err)

	metadataPath := filepath.Join(tmpDir, "metadata.json")
	require.NoError(t, os.WriteFile(metadataPath, data, 0o644))

	// Read using readMetadataUnlocked.
	result, err := readMetadataUnlocked(metadataPath)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "vpc", result.Component)
	assert.Equal(t, "dev", result.Stack)
	assert.Equal(t, SourceTypeLocal, result.SourceType)
}

func TestReadMetadataUnlocked_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "nonexistent.json")

	result, err := readMetadataUnlocked(metadataPath)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
}

func TestReadMetadataUnlocked_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "invalid.json")

	require.NoError(t, os.WriteFile(metadataPath, []byte("not valid json"), 0o644))

	result, err := readMetadataUnlocked(metadataPath)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
}

// Test ReadMetadata with various scenarios.

func TestReadMetadata_Success(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Write metadata using the public API.
	metadata := &WorkdirMetadata{
		Component:  "vpc",
		Stack:      "dev",
		SourceType: SourceTypeLocal,
		Source:     "components/terraform/vpc",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	// Read it back.
	result, err := ReadMetadata(workdirPath)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "vpc", result.Component)
}

func TestReadMetadata_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "no-metadata")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// No metadata file should return nil, nil.
	result, err := ReadMetadata(workdirPath)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestReadMetadata_LegacyLocation(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "legacy-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create metadata in legacy location.
	metadata := &WorkdirMetadata{
		Component:  "legacy",
		Stack:      "test",
		SourceType: SourceTypeLocal,
		Source:     "components/terraform/legacy",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	require.NoError(t, err)

	legacyPath := filepath.Join(workdirPath, WorkdirMetadataFile)
	require.NoError(t, os.WriteFile(legacyPath, data, 0o644))

	// Should find it in the legacy location.
	result, err := ReadMetadata(workdirPath)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "legacy", result.Component)
}

// Test WriteMetadata error paths.

func TestWriteMetadata_Success(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	metadata := &WorkdirMetadata{
		Component:  "vpc",
		Stack:      "dev",
		SourceType: SourceTypeLocal,
		Source:     "components/terraform/vpc",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := WriteMetadata(workdirPath, metadata)
	require.NoError(t, err)

	// Verify the file was created in .atmos/ directory.
	expectedPath := filepath.Join(workdirPath, AtmosDir, MetadataFile)
	_, err = os.Stat(expectedPath)
	assert.NoError(t, err)
}

func TestWriteMetadata_CreatesAtmosDir(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// .atmos dir doesn't exist yet.
	atmosDir := filepath.Join(workdirPath, AtmosDir)
	_, err := os.Stat(atmosDir)
	assert.True(t, os.IsNotExist(err))

	metadata := &WorkdirMetadata{
		Component:  "vpc",
		Stack:      "dev",
		SourceType: SourceTypeLocal,
		Source:     "components/terraform/vpc",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err = WriteMetadata(workdirPath, metadata)
	require.NoError(t, err)

	// .atmos dir should now exist.
	info, err := os.Stat(atmosDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}
