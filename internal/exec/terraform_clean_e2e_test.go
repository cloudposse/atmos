package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerraformClean_EndToEnd_NoDuplicateDeletions(t *testing.T) {
	// This test simulates the full flow from getAllStacksComponentsPaths through deletion.
	// It verifies that when multiple stacks reference the same component,
	// files are only deleted once.

	tempDir := t.TempDir()

	// Create component directories.
	componentsDir := filepath.Join(tempDir, "components", "terraform")
	vnetDir := filepath.Join(componentsDir, "vnet-elements")
	dbDir := filepath.Join(componentsDir, "database")

	require.NoError(t, os.MkdirAll(vnetDir, 0o755))
	require.NoError(t, os.MkdirAll(dbDir, 0o755))

	// Create files in vnet-elements (the component that multiple stacks will reference).
	vnetTerraformDir := filepath.Join(vnetDir, ".terraform")
	require.NoError(t, os.MkdirAll(vnetTerraformDir, 0o755))

	vnetLockFile := filepath.Join(vnetDir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(vnetLockFile, []byte("lock content"), 0o644))

	vnetVarFile := filepath.Join(vnetDir, "stack.tfvars.json")
	require.NoError(t, os.WriteFile(vnetVarFile, []byte("{}"), 0o644))

	vnetBackendFile := filepath.Join(vnetDir, "backend.tf.json")
	require.NoError(t, os.WriteFile(vnetBackendFile, []byte("{}"), 0o644))

	// Create files in database component.
	dbTerraformDir := filepath.Join(dbDir, ".terraform")
	require.NoError(t, os.MkdirAll(dbTerraformDir, 0o755))

	dbLockFile := filepath.Join(dbDir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(dbLockFile, []byte("lock content"), 0o644))

	// Simulate multiple stacks referencing the same vnet-elements component.
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vnet-elements-dev": map[string]any{
						"component": "vnet-elements", // References vnet-elements.
					},
					"database-dev": map[string]any{
						"component": "database",
					},
				},
			},
		},
		"staging": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vnet-elements-staging": map[string]any{
						"component": "vnet-elements", // Also references vnet-elements.
					},
				},
			},
		},
		"prod": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vnet-elements-prod": map[string]any{
						"component": "vnet-elements", // Also references vnet-elements.
					},
					"database-prod": map[string]any{
						"component": "database",
					},
				},
			},
		},
	}

	// Get all component paths (with our fix, should be deduplicated).
	paths := getAllStacksComponentsPaths(stacksMap)

	// Verify no duplicate paths.
	pathCounts := make(map[string]int)
	for _, path := range paths {
		pathCounts[path]++
	}

	for path, count := range pathCounts {
		assert.Equal(t, 1, count, "Path %s should appear only once but appears %d times", path, count)
	}

	// Verify we have the expected unique paths.
	assert.Contains(t, paths, "vnet-elements")
	assert.Contains(t, paths, "database")
	assert.Len(t, paths, 2, "Should have exactly 2 unique component paths")

	// Now test that file collection with these deduplicated paths works correctly.
	patterns := []string{".terraform", ".terraform.lock.hcl", "*.tfvars.json", "backend.tf.json"}
	folders, err := CollectComponentsDirectoryObjects(componentsDir, paths, patterns)
	require.NoError(t, err)

	// Count total files to be deleted.
	totalFiles := 0
	filesSeen := make(map[string]bool)
	for _, folder := range folders {
		for _, file := range folder.Files {
			if filesSeen[file.FullPath] {
				t.Errorf("File %s appears multiple times in deletion list", file.FullPath)
			}
			filesSeen[file.FullPath] = true
			totalFiles++
		}
	}

	// We should have:
	// vnet-elements: .terraform/, .terraform.lock.hcl, stack.tfvars.json, backend.tf.json = 4 files
	// database: .terraform/, .terraform.lock.hcl = 2 files
	// Total = 6 files
	assert.Equal(t, 6, totalFiles, "Should have exactly 6 files to delete")

	// Before deletion, verify all files exist.
	assert.DirExists(t, vnetTerraformDir)
	assert.FileExists(t, vnetLockFile)
	assert.FileExists(t, vnetVarFile)
	assert.FileExists(t, vnetBackendFile)
	assert.DirExists(t, dbTerraformDir)
	assert.FileExists(t, dbLockFile)

	// Execute deletions (this simulates what deleteFolders does).
	deletedCount := 0
	errorCount := 0
	for _, folder := range folders {
		for _, file := range folder.Files {
			err := DeletePathTerraform(file.FullPath, file.Name)
			if err != nil {
				if !os.IsNotExist(err) {
					t.Logf("Error deleting %s: %v", file.Name, err)
					errorCount++
				} else {
					// This would be the duplicate deletion attempt error.
					t.Errorf("Attempted to delete already-deleted file %s", file.Name)
				}
			} else {
				deletedCount++
			}
		}
	}

	// All files should be deleted successfully with no errors.
	assert.Equal(t, 6, deletedCount, "Should have deleted exactly 6 files")
	assert.Equal(t, 0, errorCount, "Should have no deletion errors")

	// Verify all files are actually gone.
	assert.NoDirExists(t, vnetTerraformDir)
	assert.NoFileExists(t, vnetLockFile)
	assert.NoFileExists(t, vnetVarFile)
	assert.NoFileExists(t, vnetBackendFile)
	assert.NoDirExists(t, dbTerraformDir)
	assert.NoFileExists(t, dbLockFile)
}
