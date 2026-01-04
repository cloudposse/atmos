package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// verifyFileExists checks that all files in the list exist.
// Returns true if all files exist, false otherwise with the first missing file path.
func verifyFileExists(t *testing.T, files []string) (bool, string) {
	t.Helper()
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			return false, file
		}
	}
	return true, ""
}

// verifyFileDeleted checks that all files in the list have been deleted.
// Returns true if all files are deleted, false otherwise with the first existing file path.
func verifyFileDeleted(t *testing.T, files []string) (bool, string) {
	t.Helper()
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			return false, file
		}
	}
	return true, ""
}

// test ExecuteTerraform clean command.
func TestCLITerraformClean(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	// Define the work directory and change to it
	workDir := "../../tests/fixtures/scenarios/terraform-sub-components"
	t.Chdir(workDir)

	var infoApply schema.ConfigAndStacksInfo
	infoApply.SubCommand = "apply"
	infoApply.ComponentType = "terraform"
	infoApply.Stack = "staging"
	infoApply.Component = "component-1"
	infoApply.ComponentFromArg = "component-1"
	err = ExecuteTerraform(infoApply)
	require.NoError(t, err)
	infoApply.Component = "component-2"
	infoApply.ComponentFromArg = "component-2"
	err = ExecuteTerraform(infoApply)
	require.NoError(t, err)
	files := []string{
		"../../components/terraform/mock-subcomponents/component-1/.terraform",
		"../../components/terraform/mock-subcomponents/component-1/terraform.tfstate.d/staging-component-1/terraform.tfstate",
		"../../components/terraform/mock-subcomponents/component-1/component-2/.terraform",
		"../../components/terraform/mock-subcomponents/component-1/component-2/terraform.tfstate.d/staging-component-2/terraform.tfstate",
	}
	// Verify that expected files exist after apply
	exists, missingFile := verifyFileExists(t, files)
	if !exists {
		t.Fatalf("Expected file does not exist: %s", missingFile)
	}

	// Initialize atmosConfig for ExecuteClean
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Call ExecuteClean directly with typed parameters (no component, no stack, force=true, dryRun=false)
	// This cleans ALL components since component="" and stack=""
	opts := &CleanOptions{
		Component:    "",
		Stack:        "",
		Force:        true,
		Everything:   false,
		SkipLockFile: false,
		DryRun:       false,
	}
	err = ExecuteClean(opts, &atmosConfig)
	require.NoError(t, err)
	// Verify that files were deleted after clean
	deleted, existingFile := verifyFileDeleted(t, files)
	if !deleted {
		t.Fatalf("File should have been deleted but still exists: %s", existingFile)
	}
}

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
			_, err := findFoldersNamesWithPrefix(tt.root, tt.prefix)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
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
				require.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
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
			_, err := getStackTerraformStateFolder(tt.componentPath, tt.stack)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIsValidDataDir(t *testing.T) {
	tests := []struct {
		name          string
		tfDataDir     string
		expectedError error
	}{
		{
			name:          "Empty TF_DATA_DIR",
			tfDataDir:     "",
			expectedError: ErrEmptyEnvDir,
		},
		{
			name:          "Root TF_DATA_DIR",
			tfDataDir:     "/",
			expectedError: ErrRefusingToDeleteDir,
		},
		{
			name:          "Valid TF_DATA_DIR",
			tfDataDir:     "/valid/path",
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidDataDir(tt.tfDataDir)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCollectComponentsDirectoryObjects(t *testing.T) {
	// Define the test cases
	tests := []struct {
		name                 string
		terraformPath        string
		componentPaths       []string
		patterns             []string
		expectedObjectsCount int
		expectedError        error
		setup                func() error
		cleanup              func() error
	}{
		{
			name:                 "Components with nested subfolders",
			terraformPath:        "../../tests/fixtures/scenarios/terraform-sub-components/components/terraform",
			componentPaths:       []string{"mock-subcomponents/component-1", "mock-subcomponents/component-1/component-2"},
			patterns:             []string{".terraform", "terraform.tfstate.d"},
			expectedObjectsCount: 2, // One for each component path with matching patterns
			setup: func() error {
				// Create test directories and files for nested components
				dirs := []string{
					"../../tests/fixtures/scenarios/terraform-sub-components/components/terraform/mock-subcomponents/component-1/.terraform",
					"../../tests/fixtures/scenarios/terraform-sub-components/components/terraform/mock-subcomponents/component-1/terraform.tfstate.d",
					"../../tests/fixtures/scenarios/terraform-sub-components/components/terraform/mock-subcomponents/component-1/component-2/.terraform",
					"../../tests/fixtures/scenarios/terraform-sub-components/components/terraform/mock-subcomponents/component-1/component-2/terraform.tfstate.d",
				}
				for _, dir := range dirs {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						return err
					}
				}
				return nil
			},
			cleanup: func() error {
				// Remove the test directories
				return os.RemoveAll("../../tests/fixtures/scenarios/terraform-sub-components/components")
			},
		},
		{
			name:           "Empty terraform path",
			terraformPath:  "",
			componentPaths: []string{"component-1"},
			patterns:       []string{".terraform"},
			expectedError:  ErrEmptyPath,
		},
		{
			name:           "Terraform path does not exist",
			terraformPath:  "nonexistent/path",
			componentPaths: []string{"component-1"},
			patterns:       []string{".terraform"},
			expectedError:  ErrPathNotExist,
		},
		{
			name:           "Empty component paths",
			terraformPath:  "../../tests/fixtures/scenarios/terraform-sub-components/components/terraform",
			componentPaths: []string{},
			patterns:       []string{".terraform"},
			expectedError:  nil, // Should return empty result, not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment if needed
			if tt.setup != nil {
				if err := tt.setup(); err != nil {
					t.Fatalf("Failed to setup test: %v", err)
				}
			}

			// Cleanup after the test
			defer func() {
				if tt.cleanup != nil {
					if err := tt.cleanup(); err != nil {
						t.Fatalf("Failed to cleanup test: %v", err)
					}
				}
			}()

			// Run the test
			dirs, err := CollectComponentsDirectoryObjects(tt.terraformPath, tt.componentPaths, tt.patterns)

			// Verify error expectation
			if tt.expectedError != nil {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
				// Verify expected number of objects
				if tt.expectedObjectsCount > 0 {
					require.Len(t, dirs, tt.expectedObjectsCount, "Expected %d directories, got %d", tt.expectedObjectsCount, len(dirs))
				}
			}
		})
	}
}

func TestCleanPluginCache_Force(t *testing.T) {
	// Create a temporary cache directory to simulate the XDG cache.
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Create the plugin cache directory with some content.
	cacheDir, err := xdg.GetXDGCacheDir("terraform/plugins", 0o755)
	require.NoError(t, err)

	// Create a fake provider file.
	testFile := filepath.Join(cacheDir, "registry.terraform.io", "hashicorp", "null", "test-provider")
	err = os.MkdirAll(filepath.Dir(testFile), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(testFile, []byte("fake provider"), 0o644)
	require.NoError(t, err)

	// Verify the file exists.
	_, err = os.Stat(testFile)
	require.NoError(t, err)

	// Run cleanPluginCache with force=true.
	err = cleanPluginCache(true, false)
	require.NoError(t, err)

	// Verify the cache directory was deleted.
	_, err = os.Stat(cacheDir)
	require.True(t, os.IsNotExist(err), "Cache directory should be deleted")
}

func TestCleanPluginCache_DryRun(t *testing.T) {
	// Create a temporary cache directory to simulate the XDG cache.
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Create the plugin cache directory with some content.
	cacheDir, err := xdg.GetXDGCacheDir("terraform/plugins", 0o755)
	require.NoError(t, err)

	// Create a fake provider file.
	testFile := filepath.Join(cacheDir, "registry.terraform.io", "hashicorp", "null", "test-provider")
	err = os.MkdirAll(filepath.Dir(testFile), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(testFile, []byte("fake provider"), 0o644)
	require.NoError(t, err)

	// Run cleanPluginCache with dryRun=true.
	err = cleanPluginCache(true, true)
	require.NoError(t, err)

	// Verify the file still exists (dry run should not delete).
	_, err = os.Stat(testFile)
	require.NoError(t, err, "File should still exist after dry run")
}

func TestCleanPluginCache_NonExistent(t *testing.T) {
	// Create a temporary cache directory without the terraform/plugins folder.
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Remove any existing cache dir.
	cacheDir := filepath.Join(tmpDir, "atmos", "terraform", "plugins")
	os.RemoveAll(cacheDir)

	// Run cleanPluginCache - should not error even if directory doesn't exist.
	err := cleanPluginCache(true, false)
	require.NoError(t, err)
}

func TestBuildConfirmationMessage(t *testing.T) {
	tests := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		total    int
		expected string
	}{
		{
			name: "No component - all components message",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
			},
			total:    5,
			expected: "This will delete 5 local terraform state files affecting all components",
		},
		{
			name: "Component and stack specified",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Component:        "vpc",
				Stack:            "dev-us-east-1",
			},
			total:    3,
			expected: "This will delete 3 local terraform state files for component 'vpc' in stack 'dev-us-east-1'",
		},
		{
			name: "Only component from arg",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Component:        "",
				Stack:            "",
			},
			total:    2,
			expected: "This will delete 2 local terraform state files for component 'vpc'",
		},
		{
			name: "Component only without stack",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Component:        "vpc",
				Stack:            "",
			},
			total:    1,
			expected: "This will delete 1 local terraform state files for component 'vpc'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildConfirmationMessage(tt.info, tt.total)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildCleanPath(t *testing.T) {
	tests := []struct {
		name          string
		info          *schema.ConfigAndStacksInfo
		componentPath string
		expectedPath  string
		expectedError error
	}{
		{
			name: "Component without stack - uses base component",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				StackFromArg:     "",
				Context: schema.Context{
					BaseComponent: "base-vpc",
				},
			},
			componentPath: "/terraform/components",
			expectedPath:  "/terraform/components/base-vpc",
			expectedError: nil,
		},
		{
			name: "Component without stack - no base component",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				StackFromArg:     "",
				Context: schema.Context{
					BaseComponent: "",
				},
			},
			componentPath: "/terraform/components",
			expectedPath:  "",
			expectedError: ErrComponentNotFound,
		},
		{
			name: "No component from arg - returns componentPath as-is",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
				StackFromArg:     "",
			},
			componentPath: "/terraform/components",
			expectedPath:  "/terraform/components",
			expectedError: nil,
		},
		{
			name: "Component with stack - returns componentPath as-is",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				StackFromArg:     "dev",
			},
			componentPath: "/terraform/components",
			expectedPath:  "/terraform/components",
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildCleanPath(tt.info, tt.componentPath)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
				// Normalize path separators for cross-platform comparison.
				require.Equal(t, tt.expectedPath, filepath.ToSlash(result))
			}
		})
	}
}

func TestBuildRelativePath(t *testing.T) {
	tests := []struct {
		name          string
		basePath      string
		componentPath string
		baseComponent string
		expectedPath  string
		expectError   bool
	}{
		{
			name:          "Simple relative path",
			basePath:      "/app",
			componentPath: "/app/components/terraform/vpc",
			baseComponent: "",
			expectedPath:  "app/components/terraform/vpc",
			expectError:   false,
		},
		{
			name:          "With base component - removes it from path",
			basePath:      "/app",
			componentPath: "/app/components/terraform/vpc",
			baseComponent: "vpc",
			expectedPath:  "app/components/terraform/",
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
				// Normalize path separators for cross-platform comparison.
				require.Equal(t, tt.expectedPath, filepath.ToSlash(result))
			}
		})
	}
}

func TestInitializeFilesToClear(t *testing.T) {
	tests := []struct {
		name                    string
		info                    schema.ConfigAndStacksInfo
		autoGenerateBackendFile bool
		expectedFiles           []string
	}{
		{
			name: "No component - returns default patterns",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
			},
			autoGenerateBackendFile: false,
			expectedFiles:           []string{".terraform", ".terraform.lock.hcl", "*.tfvar.json", "terraform.tfstate.d"},
		},
		{
			name: "With component - returns component-specific files",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Component:        "vpc",
				ContextPrefix:    "dev",
				Context: schema.Context{
					BaseComponent: "vpc",
				},
			},
			autoGenerateBackendFile: false,
			expectedFiles:           []string{".terraform", "dev-vpc.terraform.tfvars.json", "dev-vpc.planfile", ".terraform.lock.hcl"},
		},
		{
			name: "With component and skip-lock-file flag",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg:       "vpc",
				Component:              "vpc",
				ContextPrefix:          "dev",
				Context:                schema.Context{BaseComponent: "vpc"},
				AdditionalArgsAndFlags: []string{"--skip-lock-file"},
			},
			autoGenerateBackendFile: false,
			expectedFiles:           []string{".terraform", "dev-vpc.terraform.tfvars.json", "dev-vpc.planfile"},
		},
		{
			name: "With auto-generate backend file",
			info: schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Component:        "vpc",
				ContextPrefix:    "dev",
				Context:          schema.Context{BaseComponent: "vpc"},
			},
			autoGenerateBackendFile: true,
			expectedFiles:           []string{".terraform", "dev-vpc.terraform.tfvars.json", "dev-vpc.planfile", ".terraform.lock.hcl", "backend.tf.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						AutoGenerateBackendFile: tt.autoGenerateBackendFile,
					},
				},
			}
			result := initializeFilesToClear(tt.info, atmosConfig)
			require.Equal(t, tt.expectedFiles, result)
		})
	}
}

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
			name: "Multiple folders with files",
			folders: []Directory{
				{
					Files: []ObjectInfo{{Name: "file1"}, {Name: "file2"}},
				},
				{
					Files: []ObjectInfo{{Name: "file3"}},
				},
			},
			tfDataDirFolders: []Directory{
				{
					Files: []ObjectInfo{{Name: "datafile1"}},
				},
			},
			expected: 4,
		},
		{
			name:    "Only tfDataDir folders",
			folders: []Directory{},
			tfDataDirFolders: []Directory{
				{
					Files: []ObjectInfo{{Name: "datafile1"}, {Name: "datafile2"}},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countFilesToDelete(tt.folders, tt.tfDataDirFolders)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPrintFolderFiles(t *testing.T) {
	// Create a temp directory structure for testing.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tf")
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	folders := []Directory{
		{
			Name:     "folder1",
			FullPath: tmpDir,
			Files: []ObjectInfo{
				{
					Name:     "test.tf",
					FullPath: testFile,
				},
			},
		},
	}

	// printFolderFiles should not panic or error.
	require.NotPanics(t, func() {
		printFolderFiles(folders, tmpDir)
	})
}

func TestPrintDryRunOutput(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tf")
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	folders := []Directory{
		{
			Name:     "folder1",
			FullPath: tmpDir,
			Files: []ObjectInfo{
				{
					Name:     "test.tf",
					FullPath: testFile,
				},
			},
		},
	}

	// printDryRunOutput should not panic or error.
	require.NotPanics(t, func() {
		printDryRunOutput(folders, nil, tmpDir, 1)
	})
}

func TestHandleTFDataDir(t *testing.T) {
	// Create a temp directory structure for testing.
	tmpDir := t.TempDir()
	componentPath := filepath.Join(tmpDir, "component")
	tfDataDir := ".terraform-custom"
	tfDataDirPath := filepath.Join(componentPath, tfDataDir)

	err := os.MkdirAll(tfDataDirPath, 0o755)
	require.NoError(t, err)

	// Create a test file in the TF_DATA_DIR.
	testFile := filepath.Join(tfDataDirPath, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// Set TF_DATA_DIR environment variable.
	t.Setenv(EnvTFDataDir, tfDataDir)

	// handleTFDataDir should delete the directory.
	handleTFDataDir(componentPath, "component")

	// Verify the directory was deleted.
	_, err = os.Stat(tfDataDirPath)
	require.True(t, os.IsNotExist(err), "TF_DATA_DIR should be deleted")
}

func TestHandleTFDataDir_Empty(t *testing.T) {
	// When TF_DATA_DIR is not set, handleTFDataDir should do nothing.
	t.Setenv(EnvTFDataDir, "")

	// Should not panic.
	require.NotPanics(t, func() {
		handleTFDataDir("/some/path", "component")
	})
}

func TestHandleTFDataDir_InvalidPath(t *testing.T) {
	// When TF_DATA_DIR is set to invalid path, handleTFDataDir should do nothing.
	t.Setenv(EnvTFDataDir, "/")

	// Should not panic.
	require.NotPanics(t, func() {
		handleTFDataDir("/some/path", "component")
	})
}

func TestCollectTFDataDirFolders(t *testing.T) {
	tests := []struct {
		name           string
		tfDataDir      string
		setupDir       bool
		expectedLength int
	}{
		{
			name:           "Empty TF_DATA_DIR",
			tfDataDir:      "",
			setupDir:       false,
			expectedLength: 0,
		},
		{
			name:           "Invalid TF_DATA_DIR (root)",
			tfDataDir:      "/",
			setupDir:       false,
			expectedLength: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tfDataDir != "" {
				t.Setenv(EnvTFDataDir, tt.tfDataDir)
			}
			// If tt.tfDataDir is empty, don't set it - t.Setenv provides isolation.

			folders, _ := collectTFDataDirFolders("/tmp/test")
			require.Len(t, folders, tt.expectedLength)
		})
	}
}

func TestCollectTFDataDirFolders_ValidDir(t *testing.T) {
	// Create a temp directory structure for testing.
	tmpDir := t.TempDir()
	tfDataDir := ".terraform-custom"
	tfDataDirPath := filepath.Join(tmpDir, tfDataDir)

	err := os.MkdirAll(tfDataDirPath, 0o755)
	require.NoError(t, err)

	// Create a test file in the TF_DATA_DIR.
	testFile := filepath.Join(tfDataDirPath, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// Set TF_DATA_DIR environment variable.
	t.Setenv(EnvTFDataDir, tfDataDir)

	folders, returnedDir := collectTFDataDirFolders(tmpDir)
	require.Equal(t, tfDataDir, returnedDir)
	require.NotEmpty(t, folders)
}

func TestConfirmDeletion_NonTTY(t *testing.T) {
	// In test environment (non-TTY), confirmDeletion should return an error.
	confirmed, err := confirmDeletion()
	require.False(t, confirmed)
	require.Error(t, err)
}

func TestDeletePathTerraform_NonExistent(t *testing.T) {
	// Trying to delete a non-existent path should return an error.
	err := DeletePathTerraform("/nonexistent/path/that/does/not/exist", "test-file")
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestDeletePathTerraform_Symlink(t *testing.T) {
	// Create a temp directory and a symlink.
	tmpDir := t.TempDir()
	realFile := filepath.Join(tmpDir, "real.txt")
	symlink := filepath.Join(tmpDir, "link.txt")

	err := os.WriteFile(realFile, []byte("test"), 0o644)
	require.NoError(t, err)

	err = os.Symlink(realFile, symlink)
	require.NoError(t, err)

	// Trying to delete a symlink should return an error.
	err = DeletePathTerraform(symlink, "link.txt")
	require.Error(t, err)
}

func TestDeletePathTerraform_Success(t *testing.T) {
	// Create a temp file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// Delete the file.
	err = DeletePathTerraform(testFile, "test.txt")
	require.NoError(t, err)

	// Verify the file was deleted.
	_, err = os.Stat(testFile)
	require.True(t, os.IsNotExist(err))
}

func TestDeleteFolders(t *testing.T) {
	// Create a temp directory structure.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	folders := []Directory{
		{
			Name:     "folder1",
			FullPath: tmpDir,
			Files: []ObjectInfo{
				{
					Name:     "test.txt",
					FullPath: testFile,
					IsDir:    false,
				},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	// deleteFolders should not panic.
	require.NotPanics(t, func() {
		deleteFolders(folders, "folder1", atmosConfig)
	})

	// Verify the file was deleted.
	_, err = os.Stat(testFile)
	require.True(t, os.IsNotExist(err))
}

func TestGetRelativePath(t *testing.T) {
	tests := []struct {
		name          string
		basePath      string
		componentPath string
		expectError   bool
	}{
		{
			name:          "Valid paths",
			basePath:      "/app",
			componentPath: "/app/components/terraform/vpc",
			expectError:   false,
		},
		{
			name:          "Same path",
			basePath:      "/app",
			componentPath: "/app",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getRelativePath(tt.basePath, tt.componentPath)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, result)
			}
		})
	}
}

func TestFindFoldersNamesWithPrefix_Valid(t *testing.T) {
	// Create a temp directory structure.
	tmpDir := t.TempDir()

	// Create level 1 directories.
	dir1 := filepath.Join(tmpDir, "test-prefix-1")
	dir2 := filepath.Join(tmpDir, "test-prefix-2")
	dir3 := filepath.Join(tmpDir, "other-dir")

	err := os.MkdirAll(dir1, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(dir2, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(dir3, 0o755)
	require.NoError(t, err)

	// Create level 2 directories - these must also match the prefix to be included.
	subDir1 := filepath.Join(dir1, "test-prefix-sub-1")
	subDir2 := filepath.Join(dir3, "test-prefix-sub-2")
	err = os.MkdirAll(subDir1, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(subDir2, 0o755)
	require.NoError(t, err)

	// Test with prefix - matches level 1 dirs and level 2 dirs that start with prefix.
	// Level 1: test-prefix-1, test-prefix-2 match
	// Level 2: test-prefix-1/test-prefix-sub-1 and other-dir/test-prefix-sub-2 match
	folders, err := findFoldersNamesWithPrefix(tmpDir, "test-prefix")
	require.NoError(t, err)
	require.Len(t, folders, 4)

	// Test with empty prefix (all folders).
	allFolders, err := findFoldersNamesWithPrefix(tmpDir, "")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(allFolders), 5) // All level 1 and level 2 dirs.
}
