package exec

import (
	"fmt"
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
			expectedError: fmt.Errorf("root path cannot be empty"),
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
			_, err := findFoldersNamesWithPrefix(tt.root, tt.prefix, schema.AtmosConfiguration{})
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
			expectedError: fmt.Errorf("path cannot be empty"),
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
			_, err := getStackTerraformStateFolder(tt.componentPath, tt.stack, schema.AtmosConfiguration{})
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
