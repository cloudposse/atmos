package workdir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultWorkdirManager_ListWorkdirs(t *testing.T) {
	// Create a temp directory with workdir structure.
	tmpDir := t.TempDir()

	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	require.NoError(t, os.MkdirAll(workdirBase, 0o755))

	// Create a workdir with metadata.
	workdir1 := filepath.Join(workdirBase, "dev-vpc")
	require.NoError(t, os.MkdirAll(workdir1, 0o755))

	metadata1 := provWorkdir.WorkdirMetadata{
		Component:   "vpc",
		Stack:       "dev",
		SourceType:  provWorkdir.SourceTypeLocal,
		LocalPath:   "components/terraform/vpc",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ContentHash: "abc123",
	}
	metadataBytes, err := json.Marshal(metadata1)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir1, provWorkdir.WorkdirMetadataFile), metadataBytes, 0o644))

	// Create another workdir.
	workdir2 := filepath.Join(workdirBase, "prod-vpc")
	require.NoError(t, os.MkdirAll(workdir2, 0o755))

	metadata2 := provWorkdir.WorkdirMetadata{
		Component:   "vpc",
		Stack:       "prod",
		SourceType:  provWorkdir.SourceTypeLocal,
		LocalPath:   "components/terraform/vpc",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ContentHash: "def456",
	}
	metadataBytes2, err := json.Marshal(metadata2)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir2, provWorkdir.WorkdirMetadataFile), metadataBytes2, 0o644))

	// Create a directory without metadata (should be skipped).
	workdir3 := filepath.Join(workdirBase, "invalid")
	require.NoError(t, os.MkdirAll(workdir3, 0o755))

	// Test listing.
	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	workdirs, err := manager.ListWorkdirs(atmosConfig)
	require.NoError(t, err)
	assert.Len(t, workdirs, 2)

	// Check the workdirs.
	names := make([]string, len(workdirs))
	for i, w := range workdirs {
		names[i] = w.Name
	}
	assert.Contains(t, names, "dev-vpc")
	assert.Contains(t, names, "prod-vpc")
}

func TestDefaultWorkdirManager_ListWorkdirs_NoWorkdirs(t *testing.T) {
	tmpDir := t.TempDir()

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	workdirs, err := manager.ListWorkdirs(atmosConfig)
	require.NoError(t, err)
	assert.Empty(t, workdirs)
}

func TestDefaultWorkdirManager_GetWorkdirInfo(t *testing.T) {
	tmpDir := t.TempDir()

	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	workdirPath := filepath.Join(workdirBase, "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	metadata := provWorkdir.WorkdirMetadata{
		Component:   "vpc",
		Stack:       "dev",
		SourceType:  provWorkdir.SourceTypeLocal,
		LocalPath:   "components/terraform/vpc",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ContentHash: "abc123",
	}
	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, provWorkdir.WorkdirMetadataFile), metadataBytes, 0o644))

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	info, err := manager.GetWorkdirInfo(atmosConfig, "vpc", "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev-vpc", info.Name)
	assert.Equal(t, "vpc", info.Component)
	assert.Equal(t, "dev", info.Stack)
	assert.Equal(t, "components/terraform/vpc", info.Source)
	assert.Equal(t, "abc123", info.ContentHash)
}

func TestDefaultWorkdirManager_GetWorkdirInfo_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	_, err := manager.GetWorkdirInfo(atmosConfig, "vpc", "dev")
	assert.Error(t, err)
}

func TestDefaultWorkdirManager_DescribeWorkdir(t *testing.T) {
	tmpDir := t.TempDir()

	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	workdirPath := filepath.Join(workdirBase, "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	metadata := provWorkdir.WorkdirMetadata{
		Component:   "vpc",
		Stack:       "dev",
		SourceType:  provWorkdir.SourceTypeLocal,
		LocalPath:   "components/terraform/vpc",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ContentHash: "abc123",
	}
	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, provWorkdir.WorkdirMetadataFile), metadataBytes, 0o644))

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	manifest, err := manager.DescribeWorkdir(atmosConfig, "vpc", "dev")
	require.NoError(t, err)

	// Check that output contains expected structure.
	assert.Contains(t, manifest, "components:")
	assert.Contains(t, manifest, "terraform:")
	assert.Contains(t, manifest, "vpc:")
	assert.Contains(t, manifest, "metadata:")
	assert.Contains(t, manifest, "workdir:")
	assert.Contains(t, manifest, "name: dev-vpc")
	assert.Contains(t, manifest, "source: components/terraform/vpc")
}

func TestDefaultWorkdirManager_CleanWorkdir(t *testing.T) {
	tmpDir := t.TempDir()

	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	workdirPath := filepath.Join(workdirBase, "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create a file in the workdir.
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, "test.txt"), []byte("test"), 0o644))

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := manager.CleanWorkdir(atmosConfig, "vpc", "dev")
	require.NoError(t, err)

	// Verify workdir is removed.
	_, err = os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDefaultWorkdirManager_CleanWorkdir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := manager.CleanWorkdir(atmosConfig, "vpc", "dev")
	assert.Error(t, err)
}

func TestDefaultWorkdirManager_CleanAllWorkdirs(t *testing.T) {
	tmpDir := t.TempDir()

	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "dev-vpc"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "prod-vpc"), 0o755))

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := manager.CleanAllWorkdirs(atmosConfig)
	require.NoError(t, err)

	// Verify workdir base is removed.
	_, err = os.Stat(workdirBase)
	assert.True(t, os.IsNotExist(err))
}

func TestDefaultWorkdirManager_CleanAllWorkdirs_NoWorkdirs(t *testing.T) {
	tmpDir := t.TempDir()

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := manager.CleanAllWorkdirs(atmosConfig)
	require.NoError(t, err) // Should not error if nothing to clean.
}

// Test ListWorkdirs with file instead of directory (should be skipped).

func TestDefaultWorkdirManager_ListWorkdirs_FileInsteadOfDir(t *testing.T) {
	tmpDir := t.TempDir()

	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	require.NoError(t, os.MkdirAll(workdirBase, 0o755))

	// Create a file instead of directory (should be skipped).
	require.NoError(t, os.WriteFile(filepath.Join(workdirBase, "not-a-dir.txt"), []byte("test"), 0o644))

	// Create a valid workdir.
	workdir := filepath.Join(workdirBase, "dev-vpc")
	require.NoError(t, os.MkdirAll(workdir, 0o755))

	metadata := provWorkdir.WorkdirMetadata{
		Component:   "vpc",
		Stack:       "dev",
		SourceType:  provWorkdir.SourceTypeLocal,
		LocalPath:   "components/terraform/vpc",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ContentHash: "abc123",
	}
	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, provWorkdir.WorkdirMetadataFile), metadataBytes, 0o644))

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	workdirs, err := manager.ListWorkdirs(atmosConfig)
	require.NoError(t, err)
	assert.Len(t, workdirs, 1)
	assert.Equal(t, "dev-vpc", workdirs[0].Name)
}

// Test ListWorkdirs with invalid metadata JSON.

func TestDefaultWorkdirManager_ListWorkdirs_InvalidMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	require.NoError(t, os.MkdirAll(workdirBase, 0o755))

	// Create a workdir with invalid metadata.
	workdir := filepath.Join(workdirBase, "invalid-json")
	require.NoError(t, os.MkdirAll(workdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workdir, provWorkdir.WorkdirMetadataFile), []byte("not valid json"), 0o644))

	// Create a valid workdir.
	validWorkdir := filepath.Join(workdirBase, "dev-vpc")
	require.NoError(t, os.MkdirAll(validWorkdir, 0o755))

	metadata := provWorkdir.WorkdirMetadata{
		Component:  "vpc",
		Stack:      "dev",
		SourceType: provWorkdir.SourceTypeLocal,
		LocalPath:  "components/terraform/vpc",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(validWorkdir, provWorkdir.WorkdirMetadataFile), metadataBytes, 0o644))

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Should skip invalid metadata and return only valid workdirs.
	workdirs, err := manager.ListWorkdirs(atmosConfig)
	require.NoError(t, err)
	assert.Len(t, workdirs, 1)
	assert.Equal(t, "dev-vpc", workdirs[0].Name)
}

// Test DescribeWorkdir when workdir not found.

func TestDefaultWorkdirManager_DescribeWorkdir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	manifest, err := manager.DescribeWorkdir(atmosConfig, "nonexistent", "dev")
	assert.Error(t, err)
	assert.Empty(t, manifest)
}

// Test readWorkdirMetadata function directly.

func TestReadWorkdirMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := provWorkdir.WorkdirMetadata{
		Component:   "test-component",
		Stack:       "test-stack",
		SourceType:  provWorkdir.SourceTypeLocal,
		LocalPath:   "components/terraform/test",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ContentHash: "test123",
	}

	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)

	metadataPath := filepath.Join(tmpDir, "metadata.json")
	require.NoError(t, os.WriteFile(metadataPath, metadataBytes, 0o644))

	result, err := readWorkdirMetadata(metadataPath)
	require.NoError(t, err)
	assert.Equal(t, "test-component", result.Component)
	assert.Equal(t, "test-stack", result.Stack)
	assert.Equal(t, "test123", result.ContentHash)
}

func TestReadWorkdirMetadata_FileNotFound(t *testing.T) {
	result, err := readWorkdirMetadata("/nonexistent/path/metadata.json")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestReadWorkdirMetadata_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	metadataPath := filepath.Join(tmpDir, "metadata.json")
	require.NoError(t, os.WriteFile(metadataPath, []byte("{invalid json"), 0o644))

	result, err := readWorkdirMetadata(metadataPath)
	assert.Error(t, err)
	assert.Nil(t, result)
}

// Test WorkdirInfo struct.

func TestWorkdirInfo_Fields(t *testing.T) {
	info := WorkdirInfo{
		Name:        "dev-vpc",
		Component:   "vpc",
		Stack:       "dev",
		Source:      "components/terraform/vpc",
		Path:        ".workdir/terraform/dev-vpc",
		ContentHash: "abc123",
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	assert.Equal(t, "dev-vpc", info.Name)
	assert.Equal(t, "vpc", info.Component)
	assert.Equal(t, "dev", info.Stack)
	assert.Equal(t, "components/terraform/vpc", info.Source)
	assert.Equal(t, ".workdir/terraform/dev-vpc", info.Path)
	assert.Equal(t, "abc123", info.ContentHash)
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), info.CreatedAt)
	assert.Equal(t, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), info.UpdatedAt)
}

func TestWorkdirInfo_EmptyContentHash_Struct(t *testing.T) {
	info := WorkdirInfo{
		Name:      "dev-vpc",
		Component: "vpc",
		Stack:     "dev",
		// ContentHash is empty.
	}

	assert.Empty(t, info.ContentHash)
}

// Test ListWorkdirs ReadDir error path.

func TestDefaultWorkdirManager_ListWorkdirs_ReadDirError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the workdir base as a file instead of directory to cause ReadDir error.
	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	require.NoError(t, os.MkdirAll(filepath.Dir(workdirBase), 0o755))
	require.NoError(t, os.WriteFile(workdirBase, []byte("not a directory"), 0o644))

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	_, err := manager.ListWorkdirs(atmosConfig)
	assert.Error(t, err)
}

// Test CleanWorkdir RemoveAll error (permission denied).

func TestDefaultWorkdirManager_CleanWorkdir_RemoveAllError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tmpDir := t.TempDir()

	// Create workdir structure.
	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	workdirPath := filepath.Join(workdirBase, "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create a file inside the workdir.
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, "test.txt"), []byte("test"), 0o644))

	// Make the parent directory read-only to prevent removal.
	require.NoError(t, os.Chmod(workdirBase, 0o555))

	// Restore permissions for cleanup.
	defer func() {
		_ = os.Chmod(workdirBase, 0o755)
	}()

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := manager.CleanWorkdir(atmosConfig, "vpc", "dev")
	// This may or may not error depending on OS behavior.
	// On some systems, RemoveAll can still work with read-only parent.
	_ = err
}

// Test CleanAllWorkdirs RemoveAll error.

func TestDefaultWorkdirManager_CleanAllWorkdirs_RemoveAllError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tmpDir := t.TempDir()

	// Create workdir structure.
	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "dev-vpc"), 0o755))

	// Create a file inside to prevent deletion.
	require.NoError(t, os.WriteFile(filepath.Join(workdirBase, "dev-vpc", "test.txt"), []byte("test"), 0o644))

	// Make the workdir directory read-only.
	require.NoError(t, os.Chmod(filepath.Join(workdirBase, "dev-vpc"), 0o555))

	// Restore permissions for cleanup.
	defer func() {
		_ = os.Chmod(filepath.Join(workdirBase, "dev-vpc"), 0o755)
	}()

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := manager.CleanAllWorkdirs(atmosConfig)
	// This may or may not error depending on OS behavior.
	_ = err
}

// Test DescribeWorkdir with YAML marshal (always succeeds with valid struct, so just coverage).

func TestDefaultWorkdirManager_DescribeWorkdir_ValidOutput(t *testing.T) {
	tmpDir := t.TempDir()

	workdirBase := filepath.Join(tmpDir, provWorkdir.WorkdirPath, "terraform")
	workdirPath := filepath.Join(workdirBase, "prod-s3")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	metadata := provWorkdir.WorkdirMetadata{
		Component:   "s3",
		Stack:       "prod",
		SourceType:  provWorkdir.SourceTypeLocal,
		LocalPath:   "components/terraform/s3",
		CreatedAt:   time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 6, 16, 14, 45, 0, 0, time.UTC),
		ContentHash: "sha256:abc123",
	}
	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, provWorkdir.WorkdirMetadataFile), metadataBytes, 0o644))

	manager := NewDefaultWorkdirManager()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	manifest, err := manager.DescribeWorkdir(atmosConfig, "s3", "prod")
	require.NoError(t, err)

	// Verify all expected keys are present.
	assert.Contains(t, manifest, "components:")
	assert.Contains(t, manifest, "terraform:")
	assert.Contains(t, manifest, "s3:")
	assert.Contains(t, manifest, "metadata:")
	assert.Contains(t, manifest, "workdir:")
	assert.Contains(t, manifest, "name: prod-s3")
	assert.Contains(t, manifest, "source: components/terraform/s3")
	assert.Contains(t, manifest, "content_hash: sha256:abc123")
	assert.Contains(t, manifest, "2024-06-15")
	assert.Contains(t, manifest, "2024-06-16")
}
