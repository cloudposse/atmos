package cmd

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/tests"
)

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiRegex.ReplaceAllString(s, "")
}

func TestTerraformGenerateVarfileCmd(t *testing.T) {
	_ = NewTestKit(t)

	tests.RequireTerraform(t)

	if runtime.GOOS == "windows" {
		t.Skipf("Skipping on Windows: test hangs due to pipe/stderr interaction with background goroutines")
	}

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

	// Execute the command.
	RootCmd.SetArgs([]string{"terraform", "generate", "varfile", "component-1", "-s", "nonprod"})
	err = Execute()
	assert.NoError(t, err, "'TestTerraformGenerateVarfileCmd' should execute without error")

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

	outputStr := output.String()
	// Strip ANSI codes for comparison.
	cleanOutput := stripANSI(outputStr)

	// Check if the output contains the expected output.
	assert.Contains(t, cleanOutput, "Generating varfile for variables component=component-1 stack=nonprod", "'TestTerraformGenerateVarfileCmd' output should contain 'Generating varfile for variables component=component-1 stack=nonprod'")
	assert.Contains(t, cleanOutput, "nonprod-component-1.terraform.tfvars.json", "'TestTerraformGenerateVarfileCmd' output should contain 'nonprod-component-1.terraform.tfvars.json'")
}

func TestTerraformGenerateVarfileCmdNoColor(t *testing.T) {
	_ = NewTestKit(t)

	tests.RequireTerraform(t)

	if runtime.GOOS == "windows" {
		t.Skipf("Skipping on Windows: test hangs due to pipe/stderr interaction with background goroutines")
	}

	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	// Use t.Setenv() for automatic cleanup by test framework.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")
	t.Setenv("NO_COLOR", "1")

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	assert.NoError(t, err, "Creating pipe should not error")

	os.Stderr = w

	// Execute the command.
	RootCmd.SetArgs([]string{"terraform", "generate", "varfile", "component-1", "-s", "nonprod"})
	err = Execute()
	assert.NoError(t, err, "'TestTerraformGenerateVarfileCmdNoColor' should execute without error")

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

	outputStr := output.String()

	// Verify NO ANSI codes are present when NO_COLOR is set.
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	assert.False(t, ansiRegex.MatchString(outputStr), "Output should not contain ANSI codes when NO_COLOR=1")

	// Check if the output contains the expected output (plain text).
	assert.Contains(t, outputStr, "Generating varfile for variables component=component-1 stack=nonprod", "'TestTerraformGenerateVarfileCmdNoColor' output should contain plain text message")
	assert.Contains(t, outputStr, "nonprod-component-1.terraform.tfvars.json", "'TestTerraformGenerateVarfileCmdNoColor' output should contain filename")
}
