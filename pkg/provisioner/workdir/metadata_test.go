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

// Test MetadataPath function.

func TestMetadataPath(t *testing.T) {
	tests := []struct {
		name        string
		workdirPath string
		expected    string
	}{
		{
			name:        "simple path",
			workdirPath: filepath.Join("base", "workdir"),
			expected:    filepath.Join("base", "workdir", AtmosDir, MetadataFile),
		},
		{
			name:        "single directory",
			workdirPath: "workdir",
			expected:    filepath.Join("workdir", AtmosDir, MetadataFile),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := MetadataPath(tc.workdirPath)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test WriteMetadata with all fields populated.

func TestWriteMetadata_AllFields(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	now := time.Now().Truncate(time.Second)
	metadata := &WorkdirMetadata{
		Component:     "vpc",
		Stack:         "dev-us-east-1",
		SourceType:    SourceTypeRemote,
		Source:        "github.com/cloudposse/terraform-aws-vpc//src",
		SourceURI:     "github.com/cloudposse/terraform-aws-vpc//src",
		SourceVersion: "1.2.3",
		ContentHash:   "abc123",
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now,
		LastAccessed:  now,
	}

	err := WriteMetadata(workdirPath, metadata)
	require.NoError(t, err)

	// Read it back and verify all fields.
	result, err := ReadMetadata(workdirPath)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, metadata.Component, result.Component)
	assert.Equal(t, metadata.Stack, result.Stack)
	assert.Equal(t, metadata.SourceType, result.SourceType)
	assert.Equal(t, metadata.Source, result.Source)
	assert.Equal(t, metadata.SourceURI, result.SourceURI)
	assert.Equal(t, metadata.SourceVersion, result.SourceVersion)
	assert.Equal(t, metadata.ContentHash, result.ContentHash)
}

// Test ReadMetadata with priority (new location over legacy).

func TestReadMetadata_NewLocationPriority(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create metadata in new location.
	newMetadata := &WorkdirMetadata{
		Component:  "new",
		Stack:      "dev",
		SourceType: SourceTypeLocal,
	}
	require.NoError(t, WriteMetadata(workdirPath, newMetadata))

	// Also create metadata in legacy location.
	legacyMetadata := &WorkdirMetadata{
		Component:  "legacy",
		Stack:      "prod",
		SourceType: SourceTypeRemote,
	}
	data, err := json.MarshalIndent(legacyMetadata, "", "  ")
	require.NoError(t, err)
	legacyPath := filepath.Join(workdirPath, WorkdirMetadataFile)
	require.NoError(t, os.WriteFile(legacyPath, data, 0o644))

	// Should read from new location.
	result, err := ReadMetadata(workdirPath)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "new", result.Component, "should read from new location, not legacy")
}

// Test UpdateLastAccessed preserves all other fields.

func TestUpdateLastAccessed_PreservesAllFields(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create initial metadata with all fields populated.
	initialTime := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	metadata := &WorkdirMetadata{
		Component:     "vpc",
		Stack:         "dev-us-east-1",
		SourceType:    SourceTypeRemote,
		Source:        "github.com/test/repo",
		SourceURI:     "github.com/test/repo//src",
		SourceVersion: "1.0.0",
		ContentHash:   "abc123def456",
		CreatedAt:     initialTime,
		UpdatedAt:     initialTime.Add(12 * time.Hour),
		LastAccessed:  initialTime.Add(24 * time.Hour),
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	// Update last accessed.
	err := UpdateLastAccessed(workdirPath)
	require.NoError(t, err)

	// Read and verify all other fields are preserved.
	result, err := ReadMetadata(workdirPath)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, metadata.Component, result.Component)
	assert.Equal(t, metadata.Stack, result.Stack)
	assert.Equal(t, metadata.SourceType, result.SourceType)
	assert.Equal(t, metadata.Source, result.Source)
	assert.Equal(t, metadata.SourceURI, result.SourceURI)
	assert.Equal(t, metadata.SourceVersion, result.SourceVersion)
	assert.Equal(t, metadata.ContentHash, result.ContentHash)
	assert.True(t, initialTime.Equal(result.CreatedAt))
	// LastAccessed should be updated (more recent than original).
	assert.True(t, result.LastAccessed.After(metadata.LastAccessed))
}
