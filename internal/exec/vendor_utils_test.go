package exec

import (
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestExecuteVendorPullCommand tests the ExecuteVendorPullCommand function.
// It checks that the function executes the `atmos vendor pull`
// and that the vendor components are correctly pulled.
// The function also verifies that the state files are existing and deleted after the vendor pull command is executed.
func TestExecuteVendorPullCommand(t *testing.T) {
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
	workDir := "../../tests/fixtures/scenarios/vendor"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}
	// set vendor pull command
	cmd := cobra.Command{}
	flags := cmd.Flags()
	flags.String("component", "", "")
	flags.String("stack", "", "")
	flags.String("tags", "", "")
	flags.Bool("dry-run", false, "")
	flags.Bool("everything", false, "")
	err = flags.Set("component", "")
	require.NoError(t, err)
	err = ExecuteVendorPullCommand(&cmd, []string{})
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
	success, file := verifyFileExists(t, files)
	if !success {
		t.Errorf("Files do not exist: %v", file)
	}
	deleteStateFiles(t, files)
	// test dry run
	err = flags.Set("dry-run", "true")
	require.NoError(t, err)
	if err != nil {
		t.Errorf("set dry-run failed: %v", err)
	}
	err = ExecuteVendorPullCommand(&cmd, []string{})
	require.NoError(t, err)
	if err != nil {
		t.Errorf("Dry run failed: %v", err)
	}
	err = flags.Set("tags", "demo")
	require.NoError(t, err)
	if err != nil {
		t.Errorf("set tags failed: %v", err)
	}
	err = ExecuteVendorPullCommand(&cmd, []string{})
	require.NoError(t, err)
	if err != nil {
		t.Errorf("pull tag demo failed: %v", err)
	}
}

func verifyFileExists(t *testing.T, files []string) (bool, string) {
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
