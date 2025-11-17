package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkflowCmd(t *testing.T) {
	_ = NewTestKit(t)

	stacksPath := "../tests/fixtures/scenarios/atmos-overrides-section"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

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

	// Execute the command
	RootCmd.SetArgs([]string{"workflow", "--file", "workflows", "show-all-describe-component-commands"})
	err := RootCmd.Execute()
	assert.NoError(t, err, "'atmos workflow' command should execute without error")

	// Close the writer and restore stdout
	err = w.Close()
	assert.NoError(t, err, "'atmos workflow' command should execute without error")

	os.Stdout = oldStdout

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'atmos workflow' command should execute without error")

	// Check if the output contains expected markdown content
	assert.Contains(t, output.String(), expectedOutput, "'atmos workflow' output should contain information about workflows")
}

// TestWorkflowCmd_WithFileFlag tests that workflow execution with --file flag does not show usage error.
func TestWorkflowCmd_WithFileFlag(t *testing.T) {
	_ = NewTestKit(t)

	stacksPath := "../tests/fixtures/scenarios/complete"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Execute workflow with --file flag (test-1 has simple shell commands that will succeed)
	// Note: workflows.base_path in atmos.yaml is "stacks/workflows", so we just need the filename
	RootCmd.SetArgs([]string{"workflow", "test-1", "--file", "workflow1.yaml"})
	err := RootCmd.Execute()

	// Workflow should execute successfully
	// The main fix is that when TUI is used (len(args)==0), it should return after execution
	// and NOT show "Incorrect Usage" message
	assert.NoError(t, err, "workflow should execute successfully with --file flag")
}
