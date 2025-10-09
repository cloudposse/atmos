package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/tests"
)

func TestTerraformGenerateBackendCmd(t *testing.T) {
	tests.RequireTerraform(t)

	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_LOGS_LEVEL", "Debug")
	assert.NoError(t, err, "Setting 'ATMOS_LOGS_LEVEL' environment variable should execute without error")

	// Reset flag states to prevent pollution from other tests.
	// Only reset flags that were actually changed to avoid issues with complex flag types.
	RootCmd.PersistentFlags().Visit(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Execute the command
	RootCmd.SetArgs([]string{"terraform", "generate", "backend", "component-1", "-s", "nonprod"})
	err = Execute()
	assert.NoError(t, err, "'TestTerraformGenerateBackendCmd' should execute without error")

	// Close the writer and restore stderr
	err = w.Close()
	assert.NoError(t, err, "'TestTerraformGenerateBackendCmd' should execute without error")

	os.Stderr = oldStderr

	// Read captured output
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'TestTerraformGenerateBackendCmd' should execute without error")

	// Expected output after processing the templates in the component's `backend` section
	expectedOutput := "nonprod-tfstate-lock"
	outputStr := output.String()

	// Check if the output contains the expected output
	assert.Contains(t, outputStr, expectedOutput, "'TestTerraformGenerateBackendCmd' output should contain information about the generated backend")
}
