package tests

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestExecuteWorkflowCmd(t *testing.T) {
	stacksPath := "./fixtures/scenarios/atmos-overrides-section"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	utils.OsExit = func(code int) {}
	utils.PrintErrorMarkdownAndExitFn = func(title string, err error, suggestion string) {
	}
	expectedOutput := `atmos describe component c1 -s prod
atmos describe component c1 -s staging
atmos describe component c1 -s dev
atmos describe component c1 -s sandbox
atmos describe component c1 -s test
`

	// Execute the command
	os.Args = []string{"atmos", "workflow", "--file", "workflows", "show-all-describe-component-commands"}
	err = cmd.Execute()
	assert.NoError(t, err, "'atmos workflow' command should execute without error")

	// Close the writer and restore stdout
	err = w.Close()
	assert.NoError(t, err, "'atmos workflow' command should execute without error")

	os.Stdout = oldStdout

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'atmos workflow' command should execute without error")

	// Check if output contains expected markdown content
	assert.Contains(t, output.String(), expectedOutput, "'atmos workflow' output should contain information about workflows")
}

func TestExecuteWorkflow(t *testing.T) {
	stacksPath := "./fixtures/scenarios/atmos-overrides-section"
	workflowPath := "./fixtures/scenarios/atmos-overrides-section/stacks/workflows/workflows.yaml"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	expectedOutput := `atmos describe component c1 -s prod
atmos describe component c1 -s staging
atmos describe component c1 -s dev
atmos describe component c1 -s sandbox
atmos describe component c1 -s test
`

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	assert.NoError(t, err, "'InitCliConfig' should execute without error")

	workflowDefinition := schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Type:    "shell",
				Command: "echo atmos describe component c1 -s prod",
			},
			{
				Type:    "shell",
				Command: "echo atmos describe component c1 -s staging",
			},
			{
				Type:    "shell",
				Command: "echo atmos describe component c1 -s dev",
			},
			{
				Type:    "shell",
				Command: "echo atmos describe component c1 -s sandbox",
			},
			{
				Type:    "shell",
				Command: "echo atmos describe component c1 -s test",
			},
		},
	}

	err = exec.ExecuteWorkflow(atmosConfig, "show-all-describe-component-commands", workflowPath, &workflowDefinition, false, "", "")
	assert.NoError(t, err, "'ExecuteWorkflow' should execute without error")

	// Close the writer and restore stdout
	err = w.Close()
	assert.NoError(t, err, "'ExecuteWorkflow' command should execute without error")

	os.Stdout = oldStdout

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'ExecuteWorkflow' should execute without error")

	// Check if output contains expected markdown content
	assert.Contains(t, output.String(), expectedOutput, "'ExecuteWorkflow' output should contain information workflows")
}
