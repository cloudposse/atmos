package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkflowCmd(t *testing.T) {
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
