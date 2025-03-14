package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformGenerateBackendCmd(t *testing.T) {
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
	RootCmd.SetArgs([]string{"terraform", "generate", "backend", "component-1", "-s", "nonprod"})
	err = RootCmd.Execute()
	assert.NoError(t, err, "'TestTerraformGenerateBackendCmd' should execute without error")

	// Close the writer and restore stdout
	err = w.Close()
	assert.NoError(t, err, "'TestTerraformGenerateBackendCmd' should execute without error")

	os.Stderr = oldStderr

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'TestTerraformGenerateBackendCmd' should execute without error")

	// Expected output after processing the templates in the component's `backend` section
	expectedOutput := "nonprod-tfstate-lock"

	// Check if output contains the expected output
	assert.Contains(t, output.String(), expectedOutput, "'TestTerraformGenerateBackendCmd' output should contain information about the generated backend")
}
