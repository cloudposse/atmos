package cmd

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/tests"
)

func TestTerraformGenerateBackendCmd(t *testing.T) {
	CleanupRootCmd(t)

	tests.RequireTerraform(t)

	if runtime.GOOS == "windows" {
		t.Skipf("Skipping on Windows: test hangs due to pipe/stderr interaction with background goroutines")
	}

	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	// Use t.Setenv() for automatic cleanup by test framework.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")

	// Reset flag states to prevent pollution from other tests.
	// Only reset flags that were actually changed to avoid issues with complex flag types.
	defer func() {
		RootCmd.PersistentFlags().Visit(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}()

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	assert.NoError(t, err, "Creating pipe should not error")

	os.Stderr = w

	// Execute the command.
	RootCmd.SetArgs([]string{"terraform", "generate", "backend", "component-1", "-s", "nonprod"})
	err = Execute()
	assert.NoError(t, err, "'TestTerraformGenerateBackendCmd' should execute without error")

	// Restore stderr immediately after command execution to prevent any goroutines from blocking.
	os.Stderr = oldStderr

	// Close the writer to signal end of output.
	err = w.Close()
	assert.NoError(t, err, "Closing pipe writer should not error")

	// Read captured output from the pipe.
	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "Reading from pipe should not error")

	// Close the reader.
	err = r.Close()
	assert.NoError(t, err, "Closing pipe reader should not error")

	// Expected output after processing the templates in the component's `backend` section.
	expectedOutput := "nonprod-tfstate-lock"
	outputStr := output.String()

	// Check if the output contains the expected output.
	assert.Contains(t, outputStr, expectedOutput, "'TestTerraformGenerateBackendCmd' output should contain information about the generated backend")
}
