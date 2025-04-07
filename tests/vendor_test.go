package tests

import (
	"errors"
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteVendorPullCommand(t *testing.T) {
	stacksPath := "./fixtures/scenarios/vendor2"
	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")
	// Unset env values after testing
	defer func() {
		os.Unsetenv("ATMOS_BASE_PATH")
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	}()

	os.Args = []string{
		"atmos",
		"vendor",
		"pull",
	}
	// Execute the command
	err = cmd.Execute()
	assert.NoError(t, err, "'atmos vendor pull' command should execute without error")
}

// TestExecuteVendorPull tests the ExecuteVendorPullCommand function.
// It checks that the function executes the `atmos vendor pull`
// and that the vendor components are correctly pulled.
// The function also verifies that the state files are existing and deleted after the vendor pull command is executed.
func TestExecuteVendorPull(t *testing.T) {
	if _, exist := os.LookupEnv("ATMOS_CLI_CONFIG_PATH"); exist {
		err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		if err != nil {
			t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
		}
	}
	if _, exist := os.LookupEnv("ATMOS_BASE_PATH"); exist {
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
	workDir := "./fixtures/scenarios/vendor"
	if err := os.Chdir(workDir); err != nil {
		dir, _ := os.Getwd()
		t.Fatalf("Failed to change directory to currentDir %q to %q: %v", dir, workDir, err)
	}
	os.Args = []string{
		"atmos",
		"vendor",
		"pull",
	}
	err = cmd.Execute()
	require.NoError(t, err)
	if err != nil {
		t.Errorf("Failed to execute vendor pull command: %v", err)
	}
	files := []string{
		"./components/terraform/github/stargazers/main/main.tf",
		"./components/terraform/github/stargazers/main/outputs.tf",
		"./components/terraform/github/stargazers/main/providers.tf",
		"./components/terraform/github/stargazers/main/variables.tf",
		"./components/terraform/github/stargazers/main/versions.tf",
		"./components/terraform/github/stargazers/main/README.md",
		"./components/terraform/test-components/main/main.tf",
		"./components/terraform/test-components/main/outputs.tf",
		"./components/terraform/test-components/main/README.md",
		"./components/terraform/test-components/main/variables.tf",
		"./components/terraform/test-components/main/versions.tf",
		"./components/terraform/weather/main/main.tf",
		"./components/terraform/weather/main/outputs.tf",
		"./components/terraform/weather/main/providers.tf",
		"./components/terraform/weather/main/variables.tf",
		"./components/terraform/weather/main/versions.tf",
		"./components/terraform/weather/main/README.md",
		"./components/terraform/myapp2/outputs.tf",
		"./components/terraform/myapp2/main.tf",
		"./components/terraform/myapp2/variables.tf",
		"./components/terraform/myapp2/versions.tf",
		"./components/terraform/myapp2/README.md",
		"./components/terraform/myapp1/outputs.tf",
		"./components/terraform/myapp1/main.tf",
		"./components/terraform/myapp1/variables.tf",
		"./components/terraform/myapp1/versions.tf",
		"./components/terraform/myapp1/README.md",
	}
	success, file := verifyVendorFileExists(t, files)
	if !success {
		t.Errorf("Files do not exist: %v", file)
	}
	deleteStateFiles(t, files)

	os.Args = []string{
		"atmos",
		"vendor",
		"pull",
		"--dry-run",
	}
	err = cmd.Execute()
	require.NoError(t, err)
	if err != nil {
		t.Errorf("Dry run failed: %v", err)
	}
	os.Args = []string{
		"atmos",
		"vendor",
		"pull",
		"--dry-run",
		"--tags=demo",
	}
	err = cmd.Execute()
	require.NoError(t, err)
	if err != nil {
		t.Errorf("pull tag demo failed: %v", err)
	}
}

func verifyVendorFileExists(t *testing.T, files []string) (bool, string) {
	success := true
	for _, file := range files {
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			t.Errorf("Reason: Expected file does not exist: %q", file)
			return false, file
		}
	}
	return success, ""
}

func deleteStateFiles(t *testing.T, files []string) {
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			t.Errorf("Failed to delete state file: %q", file)
		}
	}
}
