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

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", "terraform.tfstate.d"))
		assert.NoError(t, err)
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-state-yaml-function"
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

	d, err := processTagTerraformState(&atmosConfig, "!terraform.state component-1 foo", stack, nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-a", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 bar", stack, nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-b", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 nonprod baz", "", nil)
	assert.NoError(t, err)
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

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-2 foo", stack, nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-a", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod bar", stack, nil)
	assert.NoError(t, err)
	assert.Equal(t, "component-1-b", d)

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod baz", "", nil)
	assert.NoError(t, err)
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
    key1: fallback1
    key2: fallback2`)

	// Test bracket notation with map keys containing slashes (user-reported issue)
	// https://atmos.tools/functions/yaml/terraform.state#handling-yq-expressions-with-bracket-notation-and-quotes
	t.Run("bracket notation with slashes in map keys", func(t *testing.T) {
		// Test with single quotes around the YQ expression (recommended syntax)
		d, err = processTagTerraformState(&atmosConfig, `!terraform.state component-1 '.secret_arns_map["auth0-event-stream/app/client-id"]'`, stack, nil)
		assert.NoError(t, err)
		assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123", d)

		// Test with bare brackets (also valid)
		d, err = processTagTerraformState(&atmosConfig, `!terraform.state component-1 .secret_arns_map["auth0-event-stream/app/client-id"]`, stack, nil)
		assert.NoError(t, err)
		assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123", d)

		// Test with stack parameter and single quotes
		d, err = processTagTerraformState(&atmosConfig, `!terraform.state component-1 nonprod '.secret_arns_map["auth0-event-stream/app/client-secret"]'`, stack, nil)
		assert.NoError(t, err)
		assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:client-secret-xyz789", d)
	})

	// Test the component-bracket-notation component resolution
	t.Run("component-bracket-notation describe", func(t *testing.T) {
		res, err = ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            "component-bracket-notation",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			AuthManager:          nil,
		})
		assert.NoError(t, err)

		y, err = u.ConvertToYAML(res)
		assert.Nil(t, err)
		assert.Contains(t, y, "client_id_arn: arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123")
		assert.Contains(t, y, "client_secret_arn: arn:aws:secretsmanager:us-east-1:123456789012:secret:client-secret-xyz789")
		assert.Contains(t, y, "client_id_arn_bare: arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123")
		assert.Contains(t, y, "client_id_arn_with_stack: arn:aws:secretsmanager:us-east-1:123456789012:secret:client-id-abc123")
	})
}
