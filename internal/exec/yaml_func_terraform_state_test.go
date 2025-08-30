package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestYamlFuncTerraformState(t *testing.T) {
	// Skip this test on Windows due to path handling and terraform execution differences
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows due to path handling differences with Terraform execution")
	}

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
	}()

	// Define the working directory
	workDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-terraform-state-yaml-function")
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err = ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	d := processTagTerraformState(&atmosConfig, "!terraform.state component-1 foo", stack)
	assert.Equal(t, "component-1-a", d)

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-1 bar", stack)
	assert.Equal(t, "component-1-b", d)

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-1 nonprod baz", "")
	assert.Equal(t, "component-1-c", d)

	res, err := ExecuteDescribeComponent(
		"component-2",
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

	info = schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-2",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err = ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-2 foo", stack)
	assert.Equal(t, "component-1-a", d)

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod bar", stack)
	assert.Equal(t, "component-1-b", d)

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod baz", "")
	assert.Equal(t, "component-1-c", d)

	res, err = ExecuteDescribeComponent(
		"component-3",
		stack,
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: default-value")
	assert.Contains(t, y, `test_list:
    - fallback1
    - fallback2`)
	assert.Contains(t, y, `test_map:
    key1: fallback1
    key2: fallback2`)
}
