package tests

import (
	"os"
	"path/filepath"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestYamlFuncTerraformOutput(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	stack := "nonprod"
	oldArgs := os.Args
	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", "terraform.tfstate.d"))
		assert.NoError(t, err)

		// Change back to the original working directory after the test
		if err = os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
		os.Args = oldArgs
	}()

	// Define the working directory
	workDir := "./fixtures/scenarios/atmos-terraform-output-yaml-function"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	os.Args = []string{"atmos", "terraform", "deploy", "component-1", "-s", stack, "--process-templates", "--process-functions"}
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}
	os.Args = []string{"atmos", "terraform", "deploy", "component-2", "-s", stack, "--process-templates", "--process-functions"}
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}
	res, err := exec.ExecuteDescribeComponent(
		"component-3",
		stack,
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err := u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: component-1-c")
}
