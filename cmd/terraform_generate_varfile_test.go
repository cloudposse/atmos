package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformGenerateVarfileCmd(t *testing.T) {
	skipIfTerraformNotInstalled(t)
	
	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_LOGS_LEVEL", "Debug")
	assert.NoError(t, err, "Setting 'ATMOS_LOGS_LEVEL' environment variable should execute without error")

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Execute the command
	RootCmd.SetArgs([]string{"terraform", "generate", "varfile", "component-1", "-s", "nonprod"})
	err = Execute()
	assert.NoError(t, err, "'TestTerraformGenerateVarfileCmd' should execute without error")

	// Close the writer and restore stderr
	err = w.Close()
	assert.NoError(t, err, "'TestTerraformGenerateVarfileCmd' should execute without error")

	os.Stderr = oldStderr

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'TestTerraformGenerateVarfileCmd' should execute without error")

	outputStr := output.String()

	// Check if the output contains the expected output
	assert.Contains(t, outputStr, "Generating varfile for variables component=component-1 stack=nonprod", "'TestTerraformGenerateVarfileCmd' output should contain 'Generating varfile for variables component=component-1 stack=nonprod'")
	assert.Contains(t, outputStr, "nonprod-component-1.terraform.tfvars.json", "'TestTerraformGenerateVarfileCmd' output should contain 'nonprod-component-1.terraform.tfvars.json'")
}
