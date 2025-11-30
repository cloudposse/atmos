package exec

import (
	"os"
	"path/filepath"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
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

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", "terraform.tfstate.d"))
		assert.NoError(t, err)
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-output-yaml-function"
	t.Chdir(workDir)

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

	d := processTagTerraformOutput(&atmosConfig, "!terraform.output component-1 foo", stack, nil)
	assert.Equal(t, "component-1-a", d)

	d = processTagTerraformOutput(&atmosConfig, "!terraform.output component-1 bar", stack, nil)
	assert.Equal(t, "component-1-b", d)

	d = processTagTerraformOutput(&atmosConfig, "!terraform.output component-1 nonprod baz", "", nil)
	assert.Equal(t, "component-1-c", d)

	res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "component-2",
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          nil,
	})
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

	d = processTagTerraformOutput(&atmosConfig, "!terraform.output component-2 foo", stack, nil)
	assert.Equal(t, "component-1-a", d)

	d = processTagTerraformOutput(&atmosConfig, "!terraform.output component-2 nonprod bar", stack, nil)
	assert.Equal(t, "component-1-b", d)

	d = processTagTerraformOutput(&atmosConfig, "!terraform.output component-2 nonprod baz", "", nil)
	assert.Equal(t, "component-1-c", d)

	res, err = ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "component-3",
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          nil,
	})
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
    key1: value1
    key2: value2`)
}
