package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformGenerateVarfileCmd(t *testing.T) {
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

	// Check if output contains the expected output
	assert.Contains(t, output.String(), "foo: component-1-a", "'TestTerraformGenerateVarfileCmd' output should contain 'foo: component-1-a'")
	assert.Contains(t, output.String(), "bar: component-1-b", "'TestTerraformGenerateVarfileCmd' output should contain 'bar: component-1-b'")
	assert.Contains(t, output.String(), "baz: component-1-c", "'TestTerraformGenerateVarfileCmd' output should contain 'baz: component-1-c'")
}
