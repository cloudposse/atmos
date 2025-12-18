package clean

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindFoldersNamesWithPrefix(t *testing.T) {
	tests := []struct {
		name          string
		root          string
		prefix        string
		expectedError error
	}{
		{
			name:          "Empty root path",
			root:          "",
			prefix:        "test",
			expectedError: ErrRootPath,
		},
		{
			name:          "Non-existent root path",
			root:          "nonexistent/path",
			prefix:        "test",
			expectedError: ErrReadDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FindFoldersNamesWithPrefix(tt.root, tt.prefix)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestFindFoldersNamesWithPrefix_Success tests successful folder discovery with prefix matching.
func TestFindFoldersNamesWithPrefix_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Create level 1 directories.
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "dev-stack"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "staging-stack"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "prod-stack"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "other"), 0o755))

	// Create level 2 directories.
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "dev-stack", "dev-west"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "dev-stack", "dev-east"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "dev-stack", "other-region"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "staging-stack", "staging-us"), 0o755))

	// Create a file (should be ignored).
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "dev-file.txt"), []byte("content"), 0o644))

	tests := []struct {
		name           string
		prefix         string
		expectedCount  int
		mustContain    []string
		mustNotContain []string
	}{
		{
			name:           "Match dev prefix",
			prefix:         "dev",
			expectedCount:  3, // dev-stack, dev-stack/dev-west, dev-stack/dev-east
			mustContain:    []string{"dev-stack", "dev-stack/dev-west", "dev-stack/dev-east"},
			mustNotContain: []string{"staging-stack", "prod-stack", "other"},
		},
		{
			name:          "Match staging prefix",
			prefix:        "staging",
			expectedCount: 2, // staging-stack, staging-stack/staging-us
			mustContain:   []string{"staging-stack", "staging-stack/staging-us"},
		},
		{
			name:           "Empty prefix matches all directories",
			prefix:         "",
			expectedCount:  8, // All dirs at level 1 and level 2.
			mustContain:    []string{"dev-stack", "staging-stack", "prod-stack", "other"},
			mustNotContain: []string{},
		},
		{
			name:          "No matching prefix",
			prefix:        "nonexistent",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folders, err := FindFoldersNamesWithPrefix(tempDir, tt.prefix)
			require.NoError(t, err)
			assert.Len(t, folders, tt.expectedCount)

			for _, expected := range tt.mustContain {
				assert.Contains(t, folders, expected)
			}
			for _, notExpected := range tt.mustNotContain {
				assert.NotContains(t, folders, notExpected)
			}
		})
	}
}

func TestCollectDirectoryObjects(t *testing.T) {
	tests := []struct {
		name          string
		basePath      string
		patterns      []string
		expectedError error
	}{
		{
			name:          "Empty base path",
			basePath:      "",
			patterns:      []string{"*.tfstate"},
			expectedError: ErrEmptyPath,
		},
		{
			name:          "Non-existent base path",
			basePath:      "nonexistent/path",
			patterns:      []string{"*.tfstate"},
			expectedError: ErrPathNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CollectDirectoryObjects(tt.basePath, tt.patterns)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCollectDirectoryObjects_Success tests successful file collection with various patterns.
func TestCollectDirectoryObjects_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Create base directory files.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.tfstate"), []byte("state"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "backup.tfstate.backup"), []byte("backup"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".terraform.lock.hcl"), []byte("lock"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "vars.tfvars.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "unrelated.txt"), []byte("other"), 0o644))

	// Create .terraform directory.
	terraformDir := filepath.Join(tempDir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(terraformDir, "terraform.tfstate"), []byte("state"), 0o644))

	// Create subdirectory with files.
	subDir := filepath.Join(tempDir, "workspace1")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "workspace.tfstate"), []byte("state"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "workspace.tfstate.backup"), []byte("backup"), 0o644))

	tests := []struct {
		name               string
		patterns           []string
		expectedFolders    int
		expectedTotalFiles int
		checkFiles         map[string][]string // folder -> file names
	}{
		{
			name:               "Match tfstate files",
			patterns:           []string{"*.tfstate"},
			expectedFolders:    3, // base, .terraform subdir, and workspace1
			expectedTotalFiles: 3, // main.tfstate, .terraform/terraform.tfstate, workspace.tfstate
			checkFiles: map[string][]string{
				filepath.Base(tempDir): {"main.tfstate"},
				"workspace1":           {"workspace.tfstate"},
				".terraform":           {"terraform.tfstate"},
			},
		},
		{
			name:               "Match multiple patterns",
			patterns:           []string{"*.tfstate", "*.tfstate.backup"},
			expectedFolders:    3, // base, .terraform, and workspace1
			expectedTotalFiles: 5, // main.tfstate, backup.tfstate.backup, .terraform/terraform.tfstate, workspace.tfstate, workspace.tfstate.backup
		},
		{
			name:               "Match .terraform directory",
			patterns:           []string{".terraform"},
			expectedFolders:    1,
			expectedTotalFiles: 1, // The .terraform directory itself
		},
		{
			name:               "Match lock file",
			patterns:           []string{".terraform.lock.hcl"},
			expectedFolders:    1,
			expectedTotalFiles: 1,
		},
		{
			name:               "No matches",
			patterns:           []string{"*.nonexistent"},
			expectedFolders:    0,
			expectedTotalFiles: 0,
		},
		{
			name:               "Match tfvars.json",
			patterns:           []string{"*.tfvars.json"},
			expectedFolders:    1,
			expectedTotalFiles: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folders, err := CollectDirectoryObjects(tempDir, tt.patterns)
			require.NoError(t, err)
			assert.Len(t, folders, tt.expectedFolders)

			totalFiles := 0
			for _, folder := range folders {
				totalFiles += len(folder.Files)

				// Verify folder structure.
				assert.NotEmpty(t, folder.Name)
				assert.NotEmpty(t, folder.FullPath)
				assert.NotEmpty(t, folder.RelativePath)

				// Verify file structure.
				for _, file := range folder.Files {
					assert.NotEmpty(t, file.Name)
					assert.NotEmpty(t, file.FullPath)
					assert.NotEmpty(t, file.RelativePath)
					// Verify file exists.
					_, err := os.Stat(file.FullPath)
					assert.NoError(t, err)
				}

				// Check expected files if specified.
				if expectedFiles, ok := tt.checkFiles[folder.Name]; ok {
					var fileNames []string
					for _, f := range folder.Files {
						fileNames = append(fileNames, f.Name)
					}
					for _, expected := range expectedFiles {
						assert.Contains(t, fileNames, expected)
					}
				}
			}
			assert.Equal(t, tt.expectedTotalFiles, totalFiles)
		})
	}
}

// TestCollectDirectoryObjects_DirectoryAsFile tests collecting directories as file entries.
func TestCollectDirectoryObjects_DirectoryAsFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create a .terraform directory.
	terraformDir := filepath.Join(tempDir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir, 0o755))

	folders, err := CollectDirectoryObjects(tempDir, []string{".terraform"})
	require.NoError(t, err)
	require.Len(t, folders, 1)
	require.Len(t, folders[0].Files, 1)

	// Verify the directory is marked as such.
	assert.True(t, folders[0].Files[0].IsDir)
	assert.Equal(t, ".terraform", folders[0].Files[0].Name)
}

func TestGetStackTerraformStateFolder(t *testing.T) {
	tests := []struct {
		name          string
		componentPath string
		stack         string
		expectedError error
	}{
		{
			name:          "Non-existent component path",
			componentPath: "nonexistent/path",
			stack:         "test",
			expectedError: ErrFailedFoundStack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetStackTerraformStateFolder(tt.componentPath, tt.stack)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGetStackTerraformStateFolder_Success tests successful terraform state folder discovery.
func TestGetStackTerraformStateFolder_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Create terraform.tfstate.d directory structure.
	tfStateDDir := filepath.Join(tempDir, "terraform.tfstate.d")
	require.NoError(t, os.MkdirAll(tfStateDDir, 0o755))

	// Create stack-specific state folders.
	devStack := filepath.Join(tfStateDDir, "dev-us-east-1")
	stagingStack := filepath.Join(tfStateDDir, "staging-us-west-2")
	prodStack := filepath.Join(tfStateDDir, "prod-eu-west-1")
	require.NoError(t, os.MkdirAll(devStack, 0o755))
	require.NoError(t, os.MkdirAll(stagingStack, 0o755))
	require.NoError(t, os.MkdirAll(prodStack, 0o755))

	// Create state files in each stack folder.
	require.NoError(t, os.WriteFile(filepath.Join(devStack, "terraform.tfstate"), []byte("state"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(devStack, "terraform.tfstate.backup"), []byte("backup"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(stagingStack, "terraform.tfstate"), []byte("state"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(prodStack, "terraform.tfstate"), []byte("state"), 0o644))

	tests := []struct {
		name               string
		stack              string
		expectedFolders    int
		expectedTotalFiles int
	}{
		{
			name:               "Find dev stack state folder",
			stack:              "dev",
			expectedFolders:    1,
			expectedTotalFiles: 2, // terraform.tfstate and terraform.tfstate.backup
		},
		{
			name:               "Find staging stack state folder",
			stack:              "staging",
			expectedFolders:    1,
			expectedTotalFiles: 1,
		},
		{
			name:               "Find prod stack state folder",
			stack:              "prod",
			expectedFolders:    1,
			expectedTotalFiles: 1,
		},
		{
			name:               "No matching stack",
			stack:              "nonexistent",
			expectedFolders:    0,
			expectedTotalFiles: 0,
		},
		{
			name:               "Empty stack matches all",
			stack:              "",
			expectedFolders:    3,
			expectedTotalFiles: 4, // 2 in dev + 1 in staging + 1 in prod
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folders, err := GetStackTerraformStateFolder(tempDir, tt.stack)
			require.NoError(t, err)
			assert.Len(t, folders, tt.expectedFolders)

			totalFiles := 0
			for _, folder := range folders {
				totalFiles += len(folder.Files)
			}
			assert.Equal(t, tt.expectedTotalFiles, totalFiles)
		})
	}
}

// TestGetStackTerraformStateFolder_NestedStacks tests state folder discovery with nested structure.
func TestGetStackTerraformStateFolder_NestedStacks(t *testing.T) {
	tempDir := t.TempDir()

	// Create terraform.tfstate.d directory structure with nested folders.
	tfStateDDir := filepath.Join(tempDir, "terraform.tfstate.d")
	require.NoError(t, os.MkdirAll(tfStateDDir, 0o755))

	// Create level 1 directory.
	regionDir := filepath.Join(tfStateDDir, "us-east-1")
	require.NoError(t, os.MkdirAll(regionDir, 0o755))

	// Create level 2 stack directory.
	devStack := filepath.Join(regionDir, "dev")
	require.NoError(t, os.MkdirAll(devStack, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(devStack, "terraform.tfstate"), []byte("state"), 0o644))

	// Search with empty prefix to find nested folders.
	folders, err := GetStackTerraformStateFolder(tempDir, "")
	require.NoError(t, err)

	// Should find both the region dir and the nested dev stack.
	assert.GreaterOrEqual(t, len(folders), 1)
}

// TestCollectComponentsDirectoryObjects_NoDuplicateFiles verifies that when multiple stacks
// reference the same component, files are collected only once.
func TestCollectComponentsDirectoryObjects_NoDuplicateFiles(t *testing.T) {
	// Create a temporary directory structure to simulate terraform components.
	tempDir := t.TempDir()

	// Create component directories that would be referenced by multiple stacks.
	component1Dir := filepath.Join(tempDir, "component1")
	component2Dir := filepath.Join(tempDir, "component2")

	require.NoError(t, os.MkdirAll(component1Dir, 0o755))
	require.NoError(t, os.MkdirAll(component2Dir, 0o755))

	// Create test files in component1.
	terraformDir1 := filepath.Join(component1Dir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir1, 0o755))

	lockFile1 := filepath.Join(component1Dir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile1, []byte("lock content"), 0o644))

	varFile1 := filepath.Join(component1Dir, "test.tfvars.json")
	require.NoError(t, os.WriteFile(varFile1, []byte("{}"), 0o644))

	// Create test files in component2.
	terraformDir2 := filepath.Join(component2Dir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir2, 0o755))

	lockFile2 := filepath.Join(component2Dir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile2, []byte("lock content"), 0o644))

	// Test patterns that would match our created files.
	patterns := []string{".terraform", ".terraform.lock.hcl", "*.tfvars.json"}

	// Simulate multiple stacks referencing the same components.
	// This would happen when getAllStacksComponentsPaths returns duplicate paths.
	componentPaths := []string{"component1", "component1", "component2", "component1"} // component1 appears 3 times.

	// Use CollectComponentsDirectoryObjects which should deduplicate.
	folders, err := CollectComponentsDirectoryObjects(tempDir, componentPaths, patterns)
	require.NoError(t, err)

	// Count how many times each file appears.
	fileOccurrences := make(map[string]int)
	for _, folder := range folders {
		for _, file := range folder.Files {
			fileOccurrences[file.FullPath]++
		}
	}

	// Check if any file appears more than once.
	for path, count := range fileOccurrences {
		if count > 1 {
			t.Errorf("File %s appears %d times (duplicate)", path, count)
		}
	}
}

// TestTerraformClean_EndToEnd_NoDuplicateDeletions is an end-to-end test that simulates the
// full flow from getAllStacksComponentsPaths through deletion. It verifies that when multiple
// stacks reference the same component, files are only deleted once.
func TestTerraformClean_EndToEnd_NoDuplicateDeletions(t *testing.T) {
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

	// Get all component paths (with deduplication).
	paths := GetAllStacksComponentsPaths(stacksMap)

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
	// Total = 6 files.
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
			err := DeletePath(file.FullPath, file.Name)
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

// TestGetAllStacksComponentsPaths tests the GetAllStacksComponentsPaths function.
func TestGetAllStacksComponentsPaths(t *testing.T) {
	tests := []struct {
		name          string
		stacksMap     map[string]any
		expectedPaths []string
		expectedLen   int
	}{
		{
			name:          "Empty stacks map",
			stacksMap:     map[string]any{},
			expectedPaths: nil,
			expectedLen:   0,
		},
		{
			name: "Single stack single component",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
			},
			expectedPaths: []string{"vpc"},
			expectedLen:   1,
		},
		{
			name: "Multiple stacks same component",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc-dev": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
				"staging": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc-staging": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
			},
			expectedPaths: []string{"vpc"},
			expectedLen:   1, // Deduplicated
		},
		{
			name: "Multiple stacks different components",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc-dev": map[string]any{
								"component": "vpc",
							},
							"rds-dev": map[string]any{
								"component": "rds",
							},
						},
					},
				},
				"staging": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"s3-staging": map[string]any{
								"component": "s3",
							},
						},
					},
				},
			},
			expectedPaths: []string{"vpc", "rds", "s3"},
			expectedLen:   3,
		},
		{
			name: "Invalid stack structure is skipped",
			stacksMap: map[string]any{
				"invalid": "not a map",
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
			},
			expectedPaths: []string{"vpc"},
			expectedLen:   1,
		},
		{
			name: "Missing components section is skipped",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"other": "data",
				},
				"staging": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
			},
			expectedPaths: []string{"vpc"},
			expectedLen:   1,
		},
		{
			name: "Missing terraform section is skipped",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"helmfile": map[string]any{
							"chart": map[string]any{
								"component": "chart",
							},
						},
					},
				},
			},
			expectedPaths: nil,
			expectedLen:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := GetAllStacksComponentsPaths(tt.stacksMap)
			assert.Len(t, paths, tt.expectedLen)

			for _, expected := range tt.expectedPaths {
				assert.Contains(t, paths, expected)
			}
		})
	}
}

// TestCollectComponentsDirectoryObjects tests the CollectComponentsDirectoryObjects function.
func TestCollectComponentsDirectoryObjects(t *testing.T) {
	t.Run("Empty base path returns error", func(t *testing.T) {
		_, err := CollectComponentsDirectoryObjects("", []string{"vpc"}, []string{"*.tf"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrEmptyPath)
	})

	t.Run("Non-existent base path returns error", func(t *testing.T) {
		_, err := CollectComponentsDirectoryObjects("/nonexistent/path", []string{"vpc"}, []string{"*.tf"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrPathNotExist)
	})

	t.Run("Empty component paths returns empty result", func(t *testing.T) {
		tempDir := t.TempDir()
		folders, err := CollectComponentsDirectoryObjects(tempDir, []string{}, []string{"*.tf"})
		require.NoError(t, err)
		assert.Len(t, folders, 0)
	})

	t.Run("Successful collection from multiple components", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create component directories.
		vpcDir := filepath.Join(tempDir, "vpc")
		rdsDir := filepath.Join(tempDir, "rds")
		require.NoError(t, os.MkdirAll(vpcDir, 0o755))
		require.NoError(t, os.MkdirAll(rdsDir, 0o755))

		// Create files in vpc.
		require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "main.tf"), []byte("resource"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(vpcDir, ".terraform.lock.hcl"), []byte("lock"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "vars.tfvars.json"), []byte("{}"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(vpcDir, ".terraform"), 0o755))

		// Create files in rds.
		require.NoError(t, os.WriteFile(filepath.Join(rdsDir, "main.tf"), []byte("resource"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(rdsDir, ".terraform.lock.hcl"), []byte("lock"), 0o644))

		componentPaths := []string{"vpc", "rds"}
		patterns := []string{".terraform", ".terraform.lock.hcl", "*.tfvars.json"}

		folders, err := CollectComponentsDirectoryObjects(tempDir, componentPaths, patterns)
		require.NoError(t, err)
		assert.Len(t, folders, 2)

		// Count total files.
		totalFiles := 0
		for _, folder := range folders {
			totalFiles += len(folder.Files)
		}
		assert.Equal(t, 4, totalFiles) // vpc: .terraform, .terraform.lock.hcl, vars.tfvars.json; rds: .terraform.lock.hcl
	})
}

// TestCollectComponentsDirectoryObjects_Deduplication tests that duplicate paths are handled.
func TestCollectComponentsDirectoryObjects_Deduplication(t *testing.T) {
	tempDir := t.TempDir()

	// Create component directory.
	vpcDir := filepath.Join(tempDir, "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, ".terraform.lock.hcl"), []byte("lock"), 0o644))

	// Pass duplicate paths.
	componentPaths := []string{"vpc", "vpc", "vpc"}
	patterns := []string{".terraform.lock.hcl"}

	folders, err := CollectComponentsDirectoryObjects(tempDir, componentPaths, patterns)
	require.NoError(t, err)

	// Should have only 1 folder despite 3 duplicate paths.
	assert.Len(t, folders, 1)
	assert.Len(t, folders[0].Files, 1)
}

// TestGetRelativePath tests the getRelativePath helper function through public APIs.
func TestGetRelativePath_ThroughPublicAPI(t *testing.T) {
	tempDir := t.TempDir()

	// Create a nested component structure.
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, ".terraform.lock.hcl"), []byte("lock"), 0o644))

	// Use CollectDirectoryObjects which internally uses relative path computation.
	folders, err := CollectDirectoryObjects(componentDir, []string{".terraform.lock.hcl"})
	require.NoError(t, err)
	require.Len(t, folders, 1)

	// Verify the relative path is computed correctly.
	assert.Equal(t, ".", folders[0].RelativePath)
	for _, file := range folders[0].Files {
		assert.Equal(t, ".terraform.lock.hcl", file.RelativePath)
	}
}
