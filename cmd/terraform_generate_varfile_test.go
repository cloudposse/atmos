package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/tests"
)

func TestTerraformGenerateVarfileCmd(t *testing.T) {
	tests.RequireTerraform(t)

	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	// Use t.Setenv() for automatic cleanup by test framework.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	assert.NoError(t, err, "Creating pipe should not error")

	os.Stderr = w

	// Ensure stderr is always restored, even if test fails.
	defer func() {
		os.Stderr = oldStderr
	}()

	// Execute the command.
	RootCmd.SetArgs([]string{"terraform", "generate", "varfile", "component-1", "-s", "nonprod"})
	err = Execute()
	assert.NoError(t, err, "'TestTerraformGenerateVarfileCmd' should execute without error")

	// Close the writer to signal end of output.
	err = w.Close()
	assert.NoError(t, err, "Closing pipe writer should not error")

	// Read captured output from the pipe.
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "Reading from pipe should not error")

	// Close the reader.
	_ = r.Close()

	outputStr := output.String()

	// Check if the output contains the expected output.
	assert.Contains(t, outputStr, "Generating varfile for variables component=component-1 stack=nonprod", "'TestTerraformGenerateVarfileCmd' output should contain 'Generating varfile for variables component=component-1 stack=nonprod'")
	assert.Contains(t, outputStr, "nonprod-component-1.terraform.tfvars.json", "'TestTerraformGenerateVarfileCmd' output should contain 'nonprod-component-1.terraform.tfvars.json'")
}
