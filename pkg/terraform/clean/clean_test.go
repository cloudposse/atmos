package clean

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockStackProcessor implements StackProcessor for testing.
type mockStackProcessor struct {
	processStacksFn                     func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error)
	executeDescribeStacksFn             func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error)
	getGenerateFilenamesForComponentFn  func(componentSection map[string]any) []string
	collectComponentsDirectoryObjectsFn func(basePath string, componentPaths []string, patterns []string) ([]Directory, error)
	constructVarfileNameFn              func(info *schema.ConfigAndStacksInfo) string
	constructPlanfileNameFn             func(info *schema.ConfigAndStacksInfo) string
	getAllStacksComponentsPathsFn       func(stacksMap map[string]any) []string
}

//nolint:gocritic // hugeParam: interface requires value type for schema.ConfigAndStacksInfo.
func (m *mockStackProcessor) ProcessStacks(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error) {
	if m.processStacksFn != nil {
		return m.processStacksFn(atmosConfig, info)
	}
	return info, nil
}

func (m *mockStackProcessor) ExecuteDescribeStacks(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error) {
	if m.executeDescribeStacksFn != nil {
		return m.executeDescribeStacksFn(atmosConfig, filterByStack, components)
	}
	return map[string]any{}, nil
}

func (m *mockStackProcessor) GetGenerateFilenamesForComponent(componentSection map[string]any) []string {
	if m.getGenerateFilenamesForComponentFn != nil {
		return m.getGenerateFilenamesForComponentFn(componentSection)
	}
	return nil
}

func (m *mockStackProcessor) CollectComponentsDirectoryObjects(basePath string, componentPaths []string, patterns []string) ([]Directory, error) {
	if m.collectComponentsDirectoryObjectsFn != nil {
		return m.collectComponentsDirectoryObjectsFn(basePath, componentPaths, patterns)
	}
	return CollectComponentsDirectoryObjects(basePath, componentPaths, patterns)
}

func (m *mockStackProcessor) ConstructTerraformComponentVarfileName(info *schema.ConfigAndStacksInfo) string {
	if m.constructVarfileNameFn != nil {
		return m.constructVarfileNameFn(info)
	}
	return "test.tfvars.json"
}

func (m *mockStackProcessor) ConstructTerraformComponentPlanfileName(info *schema.ConfigAndStacksInfo) string {
	if m.constructPlanfileNameFn != nil {
		return m.constructPlanfileNameFn(info)
	}
	return "test.planfile"
}

func (m *mockStackProcessor) GetAllStacksComponentsPaths(stacksMap map[string]any) []string {
	if m.getAllStacksComponentsPathsFn != nil {
		return m.getAllStacksComponentsPathsFn(stacksMap)
	}
	return GetAllStacksComponentsPaths(stacksMap)
}

// TestNewService tests the service constructor.
func TestNewService(t *testing.T) {
	mock := &mockStackProcessor{}
	service := NewService(mock)
	assert.NotNil(t, service)
	assert.Equal(t, mock, service.processor)
}

// TestService_Execute_NilOptions tests Execute with nil options.
func TestService_Execute_NilOptions(t *testing.T) {
	mock := &mockStackProcessor{}
	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := service.Execute(nil, atmosConfig)
	require.Error(t, err)
}

// TestService_Execute_WithComponent tests Execute with a specific component.
func TestService_Execute_WithComponent(t *testing.T) {
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	mock := &mockStackProcessor{
		processStacksFn: func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error) {
			info.Context.BaseComponent = "vpc"
			info.FinalComponent = "vpc"
			return info, nil
		},
		collectComponentsDirectoryObjectsFn: func(basePath string, componentPaths []string, patterns []string) ([]Directory, error) {
			// Return empty to simulate nothing to delete.
			return []Directory{}, nil
		},
	}

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tempDir,
		TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
		Components: schema.Components{
			Terraform: schema.Terraform{},
		},
	}

	opts := &Options{
		Component: "vpc",
		Force:     true, // Skip confirmation
	}

	err := service.Execute(opts, atmosConfig)
	// Should succeed even with no files to delete.
	require.NoError(t, err)
}

// TestBuildCleanPath tests the buildCleanPath method.
func TestBuildCleanPath(t *testing.T) {
	mock := &mockStackProcessor{}
	service := NewService(mock)

	tests := []struct {
		name          string
		info          *schema.ConfigAndStacksInfo
		componentPath string
		expectedPath  string
		expectError   bool
	}{
		{
			name: "Component without stack - has base component",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				StackFromArg:     "",
				Context: schema.Context{
					BaseComponent: "base-vpc",
				},
			},
			componentPath: "/path/to/components",
			expectedPath:  "/path/to/components/base-vpc",
			expectError:   false,
		},
		{
			name: "Component without stack - no base component",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				StackFromArg:     "",
				Context:          schema.Context{},
			},
			componentPath: "/path/to/components",
			expectedPath:  "",
			expectError:   true,
		},
		{
			name: "With stack",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				StackFromArg:     "dev",
			},
			componentPath: "/path/to/components",
			expectedPath:  "/path/to/components",
			expectError:   false,
		},
		{
			name: "No component",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
				StackFromArg:     "",
			},
			componentPath: "/path/to/components",
			expectedPath:  "/path/to/components",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.buildCleanPath(tt.info, tt.componentPath)
			if tt.expectError {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrComponentNotFound)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPath, result)
			}
		})
	}
}

// TestBuildRelativePath tests the buildRelativePath function.
func TestBuildRelativePath(t *testing.T) {
	tests := []struct {
		name          string
		basePath      string
		componentPath string
		baseComponent string
		expectError   bool
	}{
		{
			name:          "Simple relative path",
			basePath:      "/base",
			componentPath: "/base/components/terraform/vpc",
			baseComponent: "",
			expectError:   false,
		},
		{
			name:          "With base component",
			basePath:      "/base",
			componentPath: "/base/components/terraform/vpc",
			baseComponent: "vpc",
			expectError:   false,
		},
		{
			name:          "Same path",
			basePath:      "/base",
			componentPath: "/base",
			baseComponent: "",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildRelativePath(tt.basePath, tt.componentPath, tt.baseComponent)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, result)
			}
		})
	}
}

// TestInitializeFilesToClear tests the initializeFilesToClear method.
func TestInitializeFilesToClear(t *testing.T) {
	mock := &mockStackProcessor{
		constructVarfileNameFn: func(info *schema.ConfigAndStacksInfo) string {
			return "test-stack.tfvars.json"
		},
		constructPlanfileNameFn: func(info *schema.ConfigAndStacksInfo) string {
			return "test-stack.planfile"
		},
		getGenerateFilenamesForComponentFn: func(componentSection map[string]any) []string {
			return []string{"generated.tf"}
		},
	}
	service := NewService(mock)

	tests := []struct {
		name                string
		info                schema.ConfigAndStacksInfo
		atmosConfig         *schema.AtmosConfiguration
		expectedContains    []string
		expectedNotContains []string
	}{
		{
			name: "No component - default patterns",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
			},
			atmosConfig:      &schema.AtmosConfiguration{},
			expectedContains: []string{".terraform", ".terraform.lock.hcl", "*.tfvar.json", "terraform.tfstate.d"},
		},
		{
			name: "With component - includes varfile and planfile",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
			},
			atmosConfig:      &schema.AtmosConfiguration{},
			expectedContains: []string{".terraform", "test-stack.tfvars.json", "test-stack.planfile", ".terraform.lock.hcl"},
		},
		{
			name: "With component - skip lock file",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg:       "vpc",
				AdditionalArgsAndFlags: []string{SkipTerraformLockFileFlag},
			},
			atmosConfig:         &schema.AtmosConfiguration{},
			expectedContains:    []string{".terraform", "test-stack.tfvars.json", "test-stack.planfile"},
			expectedNotContains: []string{".terraform.lock.hcl"},
		},
		{
			name: "With auto generate backend file",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						AutoGenerateBackendFile: true,
					},
				},
			},
			expectedContains: []string{".terraform", "backend.tf.json"},
		},
		{
			name: "With auto generate files enabled",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						AutoGenerateFiles: true,
					},
				},
			},
			expectedContains: []string{".terraform", "generated.tf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := service.initializeFilesToClear(tt.info, tt.atmosConfig)

			for _, expected := range tt.expectedContains {
				assert.Contains(t, files, expected)
			}
			for _, notExpected := range tt.expectedNotContains {
				assert.NotContains(t, files, notExpected)
			}
		})
	}
}

// TestCountFilesToDelete tests the countFilesToDelete function.
func TestCountFilesToDelete(t *testing.T) {
	tests := []struct {
		name             string
		folders          []Directory
		tfDataDirFolders []Directory
		expected         int
	}{
		{
			name:             "Empty folders",
			folders:          []Directory{},
			tfDataDirFolders: []Directory{},
			expected:         0,
		},
		{
			name: "Single folder with files",
			folders: []Directory{
				{
					Files: []ObjectInfo{
						{Name: "file1"},
						{Name: "file2"},
					},
				},
			},
			tfDataDirFolders: []Directory{},
			expected:         2,
		},
		{
			name: "Multiple folders",
			folders: []Directory{
				{Files: []ObjectInfo{{Name: "file1"}, {Name: "file2"}}},
				{Files: []ObjectInfo{{Name: "file3"}}},
			},
			tfDataDirFolders: []Directory{
				{Files: []ObjectInfo{{Name: "file4"}, {Name: "file5"}}},
			},
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countFilesToDelete(tt.folders, tt.tfDataDirFolders)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildConfirmationMessage tests the buildConfirmationMessage function.
func TestBuildConfirmationMessage(t *testing.T) {
	tests := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		total    int
		expected string
	}{
		{
			name: "No component - affects all",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
			},
			total:    10,
			expected: "This will delete 10 local terraform state files affecting all components",
		},
		{
			name: "Component and stack specified",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Component:        "vpc",
				Stack:            "dev",
			},
			total:    5,
			expected: "This will delete 5 local terraform state files for component 'vpc' in stack 'dev'",
		},
		{
			name: "Only component from arg",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Component:        "",
				Stack:            "",
			},
			total:    3,
			expected: "This will delete 3 local terraform state files for component 'vpc'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildConfirmationMessage(tt.info, tt.total)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestService_getComponentsToClean tests the getComponentsToClean method.
func TestService_getComponentsToClean(t *testing.T) {
	t.Run("With component from arg - uses FinalComponent", func(t *testing.T) {
		mock := &mockStackProcessor{}
		service := NewService(mock)

		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			FinalComponent:   "vpc-final",
		}
		atmosConfig := &schema.AtmosConfiguration{}

		paths, err := service.getComponentsToClean(info, atmosConfig)
		require.NoError(t, err)
		assert.Equal(t, []string{"vpc-final"}, paths)
	})

	t.Run("With component from arg - falls back to BaseComponent", func(t *testing.T) {
		mock := &mockStackProcessor{}
		service := NewService(mock)

		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			FinalComponent:   "",
			Context: schema.Context{
				BaseComponent: "vpc-base",
			},
		}
		atmosConfig := &schema.AtmosConfiguration{}

		paths, err := service.getComponentsToClean(info, atmosConfig)
		require.NoError(t, err)
		assert.Equal(t, []string{"vpc-base"}, paths)
	})

	t.Run("With component from arg - falls back to ComponentFromArg", func(t *testing.T) {
		mock := &mockStackProcessor{}
		service := NewService(mock)

		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			FinalComponent:   "",
			Context:          schema.Context{},
		}
		atmosConfig := &schema.AtmosConfiguration{}

		paths, err := service.getComponentsToClean(info, atmosConfig)
		require.NoError(t, err)
		assert.Equal(t, []string{"vpc"}, paths)
	})

	t.Run("No component - calls ExecuteDescribeStacks", func(t *testing.T) {
		executeDescribeStacksCalled := false
		mock := &mockStackProcessor{
			executeDescribeStacksFn: func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error) {
				executeDescribeStacksCalled = true
				return map[string]any{
					"dev": map[string]any{
						"components": map[string]any{
							"terraform": map[string]any{
								"vpc": map[string]any{
									"component": "vpc",
								},
								"rds": map[string]any{
									"component": "rds",
								},
							},
						},
					},
				}, nil
			},
		}
		service := NewService(mock)

		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "",
		}
		atmosConfig := &schema.AtmosConfiguration{}

		paths, err := service.getComponentsToClean(info, atmosConfig)
		require.NoError(t, err)
		assert.True(t, executeDescribeStacksCalled)
		assert.Contains(t, paths, "vpc")
		assert.Contains(t, paths, "rds")
	})

	t.Run("ExecuteDescribeStacks error", func(t *testing.T) {
		mock := &mockStackProcessor{
			executeDescribeStacksFn: func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error) {
				return nil, ErrDescribeStack
			},
		}
		service := NewService(mock)

		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "",
		}
		atmosConfig := &schema.AtmosConfiguration{}

		_, err := service.getComponentsToClean(info, atmosConfig)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrDescribeStack)
	})
}

// TestService_HandleSubCommand_NotClean tests HandleSubCommand with non-clean subcommand.
func TestService_HandleSubCommand_NotClean(t *testing.T) {
	mock := &mockStackProcessor{}
	service := NewService(mock)

	info := &schema.ConfigAndStacksInfo{
		SubCommand: "plan", // Not "clean"
	}
	atmosConfig := &schema.AtmosConfiguration{}

	err := service.HandleSubCommand(info, "/path", atmosConfig)
	require.NoError(t, err)
}

// TestService_HandleSubCommand_NothingToDelete tests HandleSubCommand when there's nothing to delete.
func TestService_HandleSubCommand_NothingToDelete(t *testing.T) {
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	mock := &mockStackProcessor{
		executeDescribeStacksFn: func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error) {
			return map[string]any{}, nil
		},
		getAllStacksComponentsPathsFn: func(stacksMap map[string]any) []string {
			return []string{}
		},
		collectComponentsDirectoryObjectsFn: func(basePath string, componentPaths []string, patterns []string) ([]Directory, error) {
			return []Directory{}, nil
		},
	}
	service := NewService(mock)

	info := &schema.ConfigAndStacksInfo{
		SubCommand: "clean",
	}
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tempDir,
		TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
	}

	err := service.HandleSubCommand(info, componentDir, atmosConfig)
	require.NoError(t, err)
}

// TestService_HandleSubCommand_DryRun tests HandleSubCommand in dry-run mode.
func TestService_HandleSubCommand_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Create a file to be found.
	lockFile := filepath.Join(componentDir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile, []byte("lock"), 0o644))

	mock := &mockStackProcessor{
		executeDescribeStacksFn: func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error) {
			return map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
			}, nil
		},
	}
	service := NewService(mock)

	info := &schema.ConfigAndStacksInfo{
		SubCommand: "clean",
		DryRun:     true,
	}
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tempDir,
		TerraformDirAbsolutePath: tempDir,
	}

	err := service.HandleSubCommand(info, tempDir, atmosConfig)
	require.NoError(t, err)

	// File should still exist in dry-run mode.
	assert.FileExists(t, lockFile)
}

// TestService_HandleSubCommand_Force tests HandleSubCommand with force flag.
func TestService_HandleSubCommand_Force(t *testing.T) {
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Create a file to be deleted.
	lockFile := filepath.Join(componentDir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile, []byte("lock"), 0o644))

	mock := &mockStackProcessor{
		executeDescribeStacksFn: func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error) {
			return map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
							},
						},
					},
				},
			}, nil
		},
	}
	service := NewService(mock)

	info := &schema.ConfigAndStacksInfo{
		SubCommand:             "clean",
		AdditionalArgsAndFlags: []string{ForceFlag},
	}
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tempDir,
		TerraformDirAbsolutePath: tempDir,
	}

	err := service.HandleSubCommand(info, tempDir, atmosConfig)
	require.NoError(t, err)

	// File should be deleted with force flag.
	assert.NoFileExists(t, lockFile)
}
