package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	tfclean "github.com/cloudposse/atmos/pkg/terraform/clean"
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

// TestCLITerraformClean is an integration test that verifies the clean command works
// correctly with ExecuteTerraform. This test must remain in internal/exec because it
// depends on ExecuteTerraform and other internal/exec functions.
func TestCLITerraformClean(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	// Define the work directory and change to it.
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
	// Verify that expected files exist after apply.
	exists, missingFile := verifyFileExists(t, files)
	if !exists {
		t.Fatalf("Expected file does not exist: %s", missingFile)
	}

	// Initialize atmosConfig for ExecuteClean.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Call clean service directly with typed parameters (no component, no stack, force=true, dryRun=false).
	// This cleans ALL components since component="" and stack="".
	opts := &tfclean.Options{
		Component:    "",
		Stack:        "",
		Force:        true,
		Everything:   false,
		SkipLockFile: false,
		DryRun:       false,
	}
	// Create adapter and service.
	adapter := tfclean.NewExecAdapter(
		ProcessStacksForClean,
		ExecuteDescribeStacksForClean,
		GetGenerateFilenamesForComponent,
		CollectComponentsDirectoryObjectsForClean,
		ConstructTerraformComponentVarfileNameForClean,
		ConstructTerraformComponentPlanfileNameForClean,
		GetAllStacksComponentsPathsForClean,
	)
	service := tfclean.NewService(adapter)
	err = service.Execute(opts, &atmosConfig)
	require.NoError(t, err)
	// Verify that files were deleted after clean.
	deleted, existingFile := verifyFileDeleted(t, files)
	if !deleted {
		t.Fatalf("File should have been deleted but still exists: %s", existingFile)
	}
}
