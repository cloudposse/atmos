package exec

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

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

	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the work directory and change to it
	workDir := "../../tests/fixtures/scenarios/terraform-sub-components"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

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
	success, file := verifyFileExists(t, files)
	if !success {
		t.Fatalf("File %s does not exist", file)
	}
	var cleanInfo schema.ConfigAndStacksInfo
	cleanInfo.SubCommand = "clean"
	cleanInfo.ComponentType = "terraform"
	cleanInfo.AdditionalArgsAndFlags = []string{"--force"}
	err = ExecuteTerraform(cleanInfo)
	require.NoError(t, err)
	success, file = verifyFileDeleted(t, files)
	if !success {
		t.Fatalf("File %s should not exist", file)
	}
}

func verifyFileDeleted(t *testing.T, files []string) (bool, string) {
	for _, file := range files {
		fileAbs, err := os.Stat(file)
		if err == nil {
			t.Errorf("Reason: File still exists: %q", file)
			return false, fileAbs.Name()
		}
	}
	return true, ""
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
	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

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
