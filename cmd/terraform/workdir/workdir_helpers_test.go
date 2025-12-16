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
