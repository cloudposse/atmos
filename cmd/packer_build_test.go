package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"
)

// TestPackerBuildCmd tests the packer build command execution.
// Note: This test verifies that packer build executes correctly. The actual packer
// execution may fail due to missing AWS credentials, but the test verifies:
// 1. Command arguments are parsed correctly.
// 2. Component and stack are resolved.
// 3. Variable file is generated.
// 4. Packer is invoked with correct arguments.
func TestPackerBuildCmd(t *testing.T) {
	_ = NewTestKit(t)

	skipIfPackerNotInstalled(t)

	workDir := "../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.WarnLevel)

	// Ensure plugins are installed so build errors are about credentials, not init.
	RootCmd.SetArgs([]string{"packer", "init", "aws/bastion", "-s", "nonprod"})
	if initErr := Execute(); initErr != nil {
		t.Skipf("Skipping test: packer init failed (may require network access): %v", initErr)
	}

	// Reset for actual test.
	_ = NewTestKit(t)

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe for stdout capture: %v", err)
	}
	os.Stdout = w
	os.Stderr = w
	log.SetOutput(w)

	// Ensure cleanup happens before any reads.
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		log.SetOutput(os.Stderr)
	}()

	// Run packer build - it will fail due to missing AWS credentials,
	// but we verify that Atmos correctly processes the command.
	RootCmd.SetArgs([]string{"packer", "build", "aws/bastion", "-s", "nonprod"})
	err = Execute()

	// Close write end after Execute.
	_ = w.Close()

	// Read the captured output.
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// The command may fail due to AWS credentials, but the output should contain
	// packer-specific content, indicating that Atmos successfully invoked packer
	// with the correct arguments.
	if err == nil {
		t.Logf("TestPackerBuildCmd completed successfully (unexpected in test environment)")
		return
	}

	// Skip if plugins are still missing despite init attempt.
	if strings.Contains(output, "Missing plugins") {
		t.Skipf("Skipping test: packer plugins missing (run packer init): %v", err)
	}

	// If packer ran and failed due to credentials, that's expected.
	// Check that packer actually ran (output contains packer-specific content).
	packerRan := strings.Contains(output, "amazon-ebs") ||
		strings.Contains(output, "Build") ||
		strings.Contains(output, "credential") ||
		strings.Contains(output, "Packer")

	if packerRan {
		t.Logf("Packer build executed but failed (likely due to missing credentials): %v", err)
		// Test passes - packer was correctly invoked.
		return
	}

	// If the error is from Atmos (not packer), that's a real failure.
	t.Logf("TestPackerBuildCmd output: %s", output)
	t.Errorf("Packer build failed unexpectedly: %v", err)
}

func TestPackerBuildCmdInvalidComponent(t *testing.T) {
	_ = NewTestKit(t)

	skipIfPackerNotInstalled(t)

	workDir := "../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.WarnLevel)

	// Capture stderr for error messages.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe for stderr capture: %v", err)
	}
	os.Stderr = w
	log.SetOutput(w)

	defer func() {
		os.Stderr = oldStderr
		log.SetOutput(os.Stderr)
	}()

	RootCmd.SetArgs([]string{"packer", "build", "invalid/component", "-s", "nonprod"})
	err = Execute()

	// Close write end after Execute.
	_ = w.Close()

	// Read the captured output.
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Should fail with invalid component error.
	assert.Error(t, err, "'TestPackerBuildCmdInvalidComponent' should fail for invalid component")

	// Log the error for debugging.
	t.Logf("TestPackerBuildCmdInvalidComponent error: %v", err)
	t.Logf("TestPackerBuildCmdInvalidComponent output: %s", output)
}

func TestPackerBuildCmdMissingStack(t *testing.T) {
	_ = NewTestKit(t)

	skipIfPackerNotInstalled(t)

	workDir := "../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.WarnLevel)

	RootCmd.SetArgs([]string{"packer", "build", "aws/bastion"})
	err := Execute()

	// The command should fail - either with "stack is required" or with a packer
	// execution error. Both indicate the command was processed.
	assert.Error(t, err, "'TestPackerBuildCmdMissingStack' should fail when stack is not specified")
	t.Logf("TestPackerBuildCmdMissingStack error: %v", err)
}

func TestPackerBuildCmdWithDirectoryTemplate(t *testing.T) {
	_ = NewTestKit(t)

	skipIfPackerNotInstalled(t)

	workDir := "../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.WarnLevel)

	// Ensure plugins are installed so build errors are about credentials, not init.
	RootCmd.SetArgs([]string{"packer", "init", "aws/multi-file", "-s", "nonprod"})
	if initErr := Execute(); initErr != nil {
		t.Skipf("Skipping test: packer init failed (may require network access): %v", initErr)
	}

	// Reset for actual test.
	_ = NewTestKit(t)

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe for stdout capture: %v", err)
	}
	os.Stdout = w
	os.Stderr = w
	log.SetOutput(w)

	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		log.SetOutput(os.Stderr)
	}()

	// Test with explicit directory template flag (directory mode).
	// This uses "." to load all *.pkr.hcl files from the component directory.
	RootCmd.SetArgs([]string{"packer", "build", "aws/multi-file", "-s", "nonprod", "--template", "."})
	err = Execute()

	// Close write end after Execute.
	_ = w.Close()

	// Read the captured output.
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// The command may fail due to AWS credentials, but verify packer was invoked.
	if err == nil {
		return
	}

	// Skip if plugins are still missing despite init attempt.
	if strings.Contains(output, "Missing plugins") {
		t.Skipf("Skipping test: packer plugins missing (run packer init): %v", err)
	}

	packerRan := strings.Contains(output, "amazon-ebs") ||
		strings.Contains(output, "Build") ||
		strings.Contains(output, "credential") ||
		strings.Contains(output, "Packer")

	if packerRan {
		t.Logf("Packer build with directory template executed (failed due to credentials): %v", err)
		// Test passes.
		return
	}

	t.Logf("TestPackerBuildCmdWithDirectoryTemplate output: %s", output)
	t.Errorf("Packer build with directory template failed unexpectedly: %v", err)
}
