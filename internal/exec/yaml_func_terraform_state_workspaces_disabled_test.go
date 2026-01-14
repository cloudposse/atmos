package exec

import (
	"os"
	"path/filepath"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestYamlFuncTerraformStateWorkspacesDisabled tests that the !terraform.state YAML function
// works correctly when Terraform workspaces are disabled (workspaces_enabled: false).
//
// When workspaces are disabled:
// - The workspace name is "default"
// - For local backend: state is stored at terraform.tfstate (not terraform.tfstate.d/default/terraform.tfstate)
// - For S3 backend: state is stored at <key> (not <workspace_key_prefix>/default/<key>)
//
// See: https://github.com/cloudposse/atmos/issues/1920
func TestYamlFuncTerraformStateWorkspacesDisabled(t *testing.T) {
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

	stack := "test"

	defer func() {
		// Delete the generated files and folders after the test.
		mockComponentPath := filepath.Join("..", "..", "tests", "fixtures", "components", "terraform", "mock")
		// Clean up terraform state files.
		err := os.RemoveAll(filepath.Join(mockComponentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(mockComponentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		// When workspaces are disabled, state is stored at terraform.tfstate (not in terraform.tfstate.d/).
		err = os.Remove(filepath.Join(mockComponentPath, "terraform.tfstate"))
		// Ignore error if file doesn't exist.
		if err != nil && !os.IsNotExist(err) {
			assert.NoError(t, err)
		}

		err = os.Remove(filepath.Join(mockComponentPath, "terraform.tfstate.backup"))
		// Ignore error if file doesn't exist.
		if err != nil && !os.IsNotExist(err) {
			assert.NoError(t, err)
		}
	}()

	// Define the working directory (workspaces-disabled fixture).
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-state-yaml-function-workspaces-disabled"
	t.Chdir(workDir)

	// Deploy component-1 first to create terraform state.
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
	require.NoError(t, err, "Failed to execute 'ExecuteTerraform' for component-1")

	// Initialize CLI config.
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)

	// Verify that workspaces are disabled.
	require.NotNil(t, atmosConfig.Components.Terraform.WorkspacesEnabled)
	assert.False(t, *atmosConfig.Components.Terraform.WorkspacesEnabled,
		"Expected workspaces to be disabled in this test fixture")

	// Test !terraform.state can read outputs from component-1.
	// When workspaces are disabled, the state should be at terraform.tfstate (not terraform.tfstate.d/default/terraform.tfstate).
	d, err := processTagTerraformState(&atmosConfig, "!terraform.state component-1 foo", stack, nil)
	require.NoError(t, err)
	assert.Equal(t, "component-1-a", d, "Expected to read 'foo' output from component-1 state")

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 bar", stack, nil)
	require.NoError(t, err)
	assert.Equal(t, "component-1-b", d, "Expected to read 'bar' output from component-1 state")

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 test baz", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "component-1-c", d, "Expected to read 'baz' output from component-1 state")

	// Verify component-2 can use !terraform.state to reference component-1's outputs.
	res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "component-2",
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          nil,
	})
	require.NoError(t, err)

	y, err := u.ConvertToYAML(res)
	require.NoError(t, err)
	assert.Contains(t, y, "foo: component-1-a", "component-2 should have foo from component-1 state")
	assert.Contains(t, y, "bar: component-1-b", "component-2 should have bar from component-1 state")
	assert.Contains(t, y, "baz: component-1-c", "component-2 should have baz from component-1 state")
}

// TestWorkspacesDisabledStateLocation verifies that when workspaces are disabled,
// the terraform state is stored at the correct location (terraform.tfstate, not terraform.tfstate.d/default/terraform.tfstate).
func TestWorkspacesDisabledStateLocation(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err)

	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err)

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	stack := "test"

	// Get the absolute path to the mock component before changing directories.
	// The path is relative to the current working directory (internal/exec).
	mockComponentPath, err := filepath.Abs("../../tests/fixtures/components/terraform/mock")
	require.NoError(t, err)

	defer func() {
		// Clean up terraform state files.
		err := os.RemoveAll(filepath.Join(mockComponentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(mockComponentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		err = os.Remove(filepath.Join(mockComponentPath, "terraform.tfstate"))
		if err != nil && !os.IsNotExist(err) {
			assert.NoError(t, err)
		}

		err = os.Remove(filepath.Join(mockComponentPath, "terraform.tfstate.backup"))
		if err != nil && !os.IsNotExist(err) {
			assert.NoError(t, err)
		}
	}()

	// Define the working directory (workspaces-disabled fixture).
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-state-yaml-function-workspaces-disabled"
	t.Chdir(workDir)

	// Deploy component-1.
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
	require.NoError(t, err, "Failed to deploy component-1")

	// Verify that the state file is at terraform.tfstate (not terraform.tfstate.d/default/terraform.tfstate).
	stateFilePath := filepath.Join(mockComponentPath, "terraform.tfstate")
	wrongStatePath := filepath.Join(mockComponentPath, "terraform.tfstate.d", "default", "terraform.tfstate")

	// State should exist at the correct location.
	_, err = os.Stat(stateFilePath)
	assert.NoError(t, err, "State file should exist at %s when workspaces are disabled", stateFilePath)

	// State should NOT exist at the wrong location.
	_, err = os.Stat(wrongStatePath)
	assert.True(t, os.IsNotExist(err), "State file should NOT exist at %s when workspaces are disabled", wrongStatePath)
}
