package toolchain

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddToolToVersionsDuplicateCheck(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// First, add the full name version
	err := AddToolToVersions(filePath, "opentofu/opentofu", "1.10.3")
	require.NoError(t, err)

	// Load the file to verify it was added
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu/opentofu")
	assert.Equal(t, []string{"1.10.3"}, toolVersions.Tools["opentofu/opentofu"])

	// Now try to add the alias version - this should be skipped due to duplicate check
	err = AddToolToVersions(filePath, "opentofu", "1.10.3")
	require.NoError(t, err) // Should not error, but should skip adding

	// Load the file again to verify the alias was NOT added
	toolVersions, err = LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu/opentofu")
	assert.NotContains(t, toolVersions.Tools, "opentofu") // Should not have the alias
	assert.Equal(t, []string{"1.10.3"}, toolVersions.Tools["opentofu/opentofu"])
}

func TestAddToolToVersionsReverseDuplicateCheck(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// First, add the alias version
	err := AddToolToVersions(filePath, "opentofu", "1.10.3")
	require.NoError(t, err)

	// Load the file to verify it was added
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.Equal(t, []string{"1.10.3"}, toolVersions.Tools["opentofu"])

	// Now try to add the full name version - this should be skipped due to duplicate check
	err = AddToolToVersions(filePath, "opentofu/opentofu", "1.10.3")
	require.NoError(t, err) // Should not error, but should skip adding

	// Load the file again to verify the full name was NOT added
	toolVersions, err = LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.NotContains(t, toolVersions.Tools, "opentofu/opentofu") // Should not have the full name
	assert.Equal(t, []string{"1.10.3"}, toolVersions.Tools["opentofu"])
}

func TestAddToolToVersionsDuplicateCheckWithExistingAlias(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// First, add the alias version (this simulates what happens when someone manually adds "opentofu 1.10.2")
	err := AddToolToVersions(filePath, "opentofu", "1.10.2")
	require.NoError(t, err)

	// Load the file to verify it was added
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.Equal(t, []string{"1.10.2"}, toolVersions.Tools["opentofu"])

	// Now try to add the full name version - this should be skipped due to duplicate check
	// This simulates what happens when InstallSingleTool calls AddToolToVersions with "opentofu/opentofu"
	err = AddToolToVersions(filePath, "opentofu/opentofu", "1.10.2")
	require.NoError(t, err) // Should not error, but should skip adding

	// Load the file again to verify the full name was NOT added
	toolVersions, err = LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.NotContains(t, toolVersions.Tools, "opentofu/opentofu") // Should not have the full name
	assert.Equal(t, []string{"1.10.2"}, toolVersions.Tools["opentofu"])
}

func TestAddToolToVersionsDuplicateCheckWithMultipleVersions(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// First, add the alias version with multiple versions (like in your example)
	err := AddToolToVersions(filePath, "opentofu", "1.10.3")
	require.NoError(t, err)
	err = AddToolToVersions(filePath, "opentofu", "1.10.2")
	require.NoError(t, err)

	// Load the file to verify it was added
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.Equal(t, []string{"1.10.3", "1.10.2"}, toolVersions.Tools["opentofu"])

	// Now try to add the full name version for 1.10.2 - this should be skipped due to duplicate check
	err = AddToolToVersions(filePath, "opentofu/opentofu", "1.10.2")
	require.NoError(t, err) // Should not error, but should skip adding

	// Load the file again to verify the full name was NOT added
	toolVersions, err = LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.NotContains(t, toolVersions.Tools, "opentofu/opentofu") // Should not have the full name
	assert.Equal(t, []string{"1.10.3", "1.10.2"}, toolVersions.Tools["opentofu"])
}
