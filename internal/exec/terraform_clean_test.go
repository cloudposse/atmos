package exec

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

// test ExecuteTerraform clean command.
func TestCLITerraformClean(t *testing.T) {
	if os.Getenv("ATMOS_CLI_CONFIG_PATH") != "" {
		err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		if err != nil {
			t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
		}
	}
	if os.Getenv("ATMOS_BASE_PATH") != "" {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		if err != nil {
			t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
		}
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
		"./components/terraform/component-1/.terraform",
		"./components/terraform/component-1/terraform.tfstate.d/staging/terraform.tfstate",
		"./components/terraform/component-1/component-2/.terraform",
		"./components/terraform/component-1/component-2/terraform.tfstate.d/staging-component-2/terraform.tfstate",
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
	if err != nil {
		t.Errorf("Failed to execute vendor pull command: %v", err)
	}
	success, file = verifyFileDeleted(t, files)
	if !success {
		t.Fatalf("File %s should not exist", file)
	}
}

func verifyFileDeleted(t *testing.T, files []string) (bool, string) {
	for _, file := range files {
		fileAbs, err := os.Stat(file)
		if err == nil {
			return false, fileAbs.Name()
		}
	}
	return true, ""
}
