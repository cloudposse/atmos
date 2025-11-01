package exec

import (
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetAllStacksComponentsPaths tests extracting component paths from stacks map.
func TestGetAllStacksComponentsPaths(t *testing.T) {
	tests := []struct {
		name           string
		stacksMap      map[string]any
		expectedPaths  []string
		expectedUnique bool
	}{
		{
			name: "Valid stacks with components",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
				"stack2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"ec2": map[string]any{
								"component": "ec2",
							},
						},
					},
				},
			},
			expectedPaths:  []string{"vpc", "ec2"},
			expectedUnique: true,
		},
		{
			name: "Duplicate paths across stacks",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
				"stack2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
			},
			expectedPaths:  []string{"vpc"},
			expectedUnique: true,
		},
		{
			name: "Invalid stack data - not a map",
			stacksMap: map[string]any{
				"stack1": "invalid",
			},
			expectedPaths: []string{},
		},
		{
			name: "Missing components section",
			stacksMap: map[string]any{
				"stack1": map[string]any{
					"other": "data",
				},
			},
			expectedPaths: []string{},
		},
		{
			name:          "Empty stacks map",
			stacksMap:     map[string]any{},
			expectedPaths: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := getAllStacksComponentsPaths(tt.stacksMap)

			// Check length matches
			assert.Len(t, paths, len(tt.expectedPaths), "Expected %d paths, got %d", len(tt.expectedPaths), len(paths))

			// If we expect unique paths, verify no duplicates
			if tt.expectedUnique && len(paths) > 0 {
				uniqueMap := make(map[string]bool)
				for _, path := range paths {
					assert.False(t, uniqueMap[path], "Found duplicate path: %s", path)
					uniqueMap[path] = true
				}
			}

			// Check all expected paths are present
			for _, expectedPath := range tt.expectedPaths {
				found := false
				for _, path := range paths {
					if path == expectedPath {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected path '%s' not found in result", expectedPath)
			}
		})
	}
}

// TestGetComponentsPaths tests parsing component paths from stack data.
func TestGetComponentsPaths(t *testing.T) {
	tests := []struct {
		name          string
		stackData     any
		expectedPaths []string
		expectedError error
	}{
		{
			name: "Valid terraform components",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"component": "vpc",
						},
						"ec2": map[string]any{
							"component": "ec2",
						},
					},
				},
			},
			expectedPaths: []string{"vpc", "ec2"},
		},
		{
			name:          "Invalid stack data - not a map",
			stackData:     "invalid",
			expectedError: errUtils.ErrParseStacks,
		},
		{
			name: "Missing components section",
			stackData: map[string]any{
				"other": "data",
			},
			expectedError: errUtils.ErrParseComponents,
		},
		{
			name: "Missing terraform section",
			stackData: map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{},
				},
			},
			expectedError: ErrParseTerraformComponents,
		},
		{
			name: "Invalid terraform components - not a map",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": "invalid",
				},
			},
			expectedError: ErrParseTerraformComponents,
		},
		{
			name: "Invalid component data - not a map",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": "invalid",
					},
				},
			},
			expectedError: ErrParseTerraformComponents,
		},
		{
			name: "Missing component attribute",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"other": "data",
						},
					},
				},
			},
			expectedError: ErrParseComponentsAttributes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths, err := getComponentsPaths(tt.stackData)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Len(t, paths, len(tt.expectedPaths))
				// Check all expected paths are present (order may vary)
				for _, expectedPath := range tt.expectedPaths {
					found := false
					for _, path := range paths {
						if path == expectedPath {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected path '%s' not found", expectedPath)
				}
			}
		})
	}
}

// TestValidateInputPath tests path validation.
func TestValidateInputPath(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		path          string
		expectedError error
	}{
		{
			name:          "Empty path",
			path:          "",
			expectedError: ErrEmptyPath,
		},
		{
			name:          "Non-existent path",
			path:          "/non/existent/path/12345",
			expectedError: ErrPathNotExist,
		},
		{
			name: "Valid path",
			path: tmpDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInputPath(tt.path)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateFolder tests Directory struct creation.
func TestCreateFolder(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(subDir, 0o755)
	require.NoError(t, err)

	tests := []struct {
		name       string
		rootPath   string
		folderPath string
		folderName string
		expectErr  bool
	}{
		{
			name:       "Valid folder creation",
			rootPath:   tmpDir,
			folderPath: subDir,
			folderName: "subdir",
			expectErr:  false,
		},
		{
			name:       "Root and folder are the same",
			rootPath:   tmpDir,
			folderPath: tmpDir,
			folderName: filepath.Base(tmpDir),
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder, err := createFolder(tt.rootPath, tt.folderPath, tt.folderName)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), errUtils.ErrRelPath.Error())
			} else {
				require.NoError(t, err)
				assert.NotNil(t, folder)
				assert.Equal(t, tt.folderName, folder.Name)
				assert.Equal(t, tt.folderPath, folder.FullPath)
				assert.NotEmpty(t, folder.RelativePath)
				assert.NotNil(t, folder.Files)
			}
		})
	}
}

// TestCollectFilesInFolder tests file collection with glob patterns.
func TestCollectFilesInFolder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	testFiles := []string{
		"test.tfstate",
		"test.tfstate.backup",
		"other.txt",
	}
	for _, file := range testFiles {
		filePath := filepath.Join(tmpDir, file)
		err := os.WriteFile(filePath, []byte("test"), 0o644)
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		patterns      []string
		expectedCount int
		expectErr     bool
	}{
		{
			name:          "Match tfstate files",
			patterns:      []string{"*.tfstate"},
			expectedCount: 1,
		},
		{
			name:          "Match multiple patterns",
			patterns:      []string{"*.tfstate", "*.backup"},
			expectedCount: 2,
		},
		{
			name:          "No matches",
			patterns:      []string{"*.json"},
			expectedCount: 0,
		},
		{
			name:          "Match all files",
			patterns:      []string{"*"},
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder := &Directory{
				Name:         "test",
				FullPath:     tmpDir,
				RelativePath: ".",
				Files:        []ObjectInfo{},
			}

			err := collectFilesInFolder(folder, tmpDir, tt.patterns)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, folder.Files, tt.expectedCount, "Expected %d files, got %d", tt.expectedCount, len(folder.Files))
			}
		})
	}
}

// TestCreateFileInfo tests ObjectInfo struct creation.
func TestCreateFileInfo(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// Create a test directory
	testDir := filepath.Join(tmpDir, "testdir")
	err = os.MkdirAll(testDir, 0o755)
	require.NoError(t, err)

	tests := []struct {
		name      string
		rootPath  string
		filePath  string
		expectErr bool
		expectNil bool
	}{
		{
			name:     "Regular file",
			rootPath: tmpDir,
			filePath: testFile,
		},
		{
			name:     "Directory",
			rootPath: tmpDir,
			filePath: testDir,
		},
		{
			name:      "Non-existent file",
			rootPath:  tmpDir,
			filePath:  filepath.Join(tmpDir, "nonexistent.txt"),
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := createFileInfo(tt.rootPath, tt.filePath)

			switch {
			case tt.expectErr:
				require.Error(t, err)
			case tt.expectNil:
				require.NoError(t, err)
				assert.Nil(t, info)
			default:
				require.NoError(t, err)
				assert.NotNil(t, info)
				assert.Equal(t, tt.filePath, info.FullPath)
				assert.NotEmpty(t, info.RelativePath)
				assert.Equal(t, filepath.Base(tt.filePath), info.Name)
			}
		})
	}
}

// TestCollectComponentObjects tests the full component object collection.
func TestCollectComponentObjects(t *testing.T) {
	tmpDir := t.TempDir()
	componentDir := filepath.Join(tmpDir, "vpc")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Create test files in component directory
	testFile := filepath.Join(componentDir, ".terraform")
	err = os.MkdirAll(testFile, 0o755)
	require.NoError(t, err)

	tests := []struct {
		name          string
		terraformDir  string
		componentPath string
		patterns      []string
		expectedError error
		expectedFiles int
	}{
		{
			name:          "Valid component with matching files",
			terraformDir:  tmpDir,
			componentPath: componentDir,
			patterns:      []string{".terraform"},
			expectedFiles: 1,
		},
		{
			name:          "Empty terraform directory path",
			terraformDir:  "",
			componentPath: componentDir,
			patterns:      []string{".terraform"},
			expectedError: ErrEmptyPath,
		},
		{
			name:          "Non-existent terraform directory",
			terraformDir:  "/non/existent/path",
			componentPath: componentDir,
			patterns:      []string{".terraform"},
			expectedError: ErrPathNotExist,
		},
		{
			name:          "No matching files",
			terraformDir:  tmpDir,
			componentPath: componentDir,
			patterns:      []string{"*.tfstate"},
			expectedFiles: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs, err := CollectComponentObjects(tt.terraformDir, tt.componentPath, tt.patterns)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
				if tt.expectedFiles > 0 {
					assert.Len(t, dirs, 1)
					assert.Len(t, dirs[0].Files, tt.expectedFiles)
				} else {
					assert.Empty(t, dirs)
				}
			}
		})
	}
}

// TestCollectComponentsDirectoryObjectsUtil tests collecting objects from multiple components.
func TestCollectComponentsDirectoryObjectsUtil(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test component directories
	component1 := filepath.Join(tmpDir, "vpc")
	component2 := filepath.Join(tmpDir, "ec2")
	err := os.MkdirAll(component1, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(component2, 0o755)
	require.NoError(t, err)

	// Create test files
	testFile1 := filepath.Join(component1, ".terraform")
	err = os.MkdirAll(testFile1, 0o755)
	require.NoError(t, err)
	testFile2 := filepath.Join(component2, ".terraform")
	err = os.MkdirAll(testFile2, 0o755)
	require.NoError(t, err)

	tests := []struct {
		name           string
		terraformDir   string
		componentPaths []string
		patterns       []string
		expectedCount  int
		expectedError  error
	}{
		{
			name:           "Multiple components with files",
			terraformDir:   tmpDir,
			componentPaths: []string{"vpc", "ec2"},
			patterns:       []string{".terraform"},
			expectedCount:  2,
		},
		{
			name:           "Empty component paths",
			terraformDir:   tmpDir,
			componentPaths: []string{},
			patterns:       []string{".terraform"},
			expectedCount:  0,
		},
		{
			name:           "Invalid terraform directory",
			terraformDir:   "/non/existent",
			componentPaths: []string{"vpc"},
			patterns:       []string{".terraform"},
			expectedError:  ErrPathNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs, err := CollectComponentsDirectoryObjects(tt.terraformDir, tt.componentPaths, tt.patterns)

			if tt.expectedError != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, dirs, tt.expectedCount)
			}
		})
	}
}
