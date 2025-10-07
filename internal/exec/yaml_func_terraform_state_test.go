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

func TestYamlFuncTerraformState(t *testing.T) {
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
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-state-yaml-function"
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

	d, err := processTagTerraformState(&atmosConfig, "!terraform.state component-1 foo", stack)
	assert.Equal(t, "component-1-a", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 bar", stack)
	assert.Equal(t, "component-1-b", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 nonprod baz", "")
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

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-2 foo", stack)
	assert.Equal(t, "component-1-a", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod bar", stack)
	assert.Equal(t, "component-1-b", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod baz", "")
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

func TestProcessTagTerraformState_ErrorPaths(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/tmp/test",
	}

	tests := []struct {
		name          string
		input         string
		currentStack  string
		expectError   bool
		errorContains string
	}{
		{
			name:          "empty after tag",
			input:         "!terraform.state",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid Atmos YAML function",
		},
		{
			name:          "insufficient parameters - 1 param",
			input:         "!terraform.state component-1",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid number of arguments",
		},
		{
			name:          "insufficient parameters - 0 params",
			input:         "!terraform.state ",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid Atmos YAML function",
		},
		{
			name:          "too many parameters - 4 params",
			input:         "!terraform.state component-1 dev attr extra",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid number of arguments",
		},
		{
			name:          "invalid parameter format - quoted string not closed",
			input:         "!terraform.state component-1 \"unclosed",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "", // SplitStringByDelimiter error varies
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processTagTerraformState(atmosConfig, tt.input, tt.currentStack)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				// For error cases, result should be nil
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
