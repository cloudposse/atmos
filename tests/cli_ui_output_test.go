package tests

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestUIOutputNotUsingLogging verifies that UI output goes to stderr and doesn't use logging.
// This ensures users get proper feedback even when logging is disabled.
func TestUIOutputNotUsingLogging(t *testing.T) {
	if skipReason != "" {
		t.Skipf("%s", skipReason)
	}

	testCases := []struct {
		name           string
		command        string
		envVars        map[string]string
		expectedInErr  []string
		notExpectedErr []string
	}{
		{
			name:    "Invalid command shows error without logging",
			command: "invalid-command",
			envVars: map[string]string{
				"ATMOS_LOGS_LEVEL": "Off", // Disable logging completely
			},
			expectedInErr: []string{
				"Incorrect Usage",
				"Unknown command",
				"Valid subcommands are:",
			},
			notExpectedErr: []string{
				"INFO", // Should not see log levels
				"DEBUG",
				"WARN",
				"ERROR",
			},
		},
		{
			name:    "Invalid subcommand shows proper error format",
			command: "terraform invalid-action",
			envVars: map[string]string{
				"ATMOS_LOGS_LEVEL": "Off",
			},
			expectedInErr: []string{
				"Unknown command invalid-action for atmos terraform",
				"Valid subcommands are:",
			},
			notExpectedErr: []string{
				"INFO Available",        // Old log format
				"Available command(s):", // Old message format
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Construct the command
			cmdArgs := strings.Fields(tc.command)
			cmd := exec.Command("atmos", cmdArgs...)

			// Set environment variables
			cmd.Env = os.Environ()
			for key, val := range tc.envVars {
				cmd.Env = append(cmd.Env, key+"="+val)
			}

			// Capture stderr (where error messages should go)
			var stderr strings.Builder
			cmd.Stderr = &stderr

			// We expect the command to fail
			err := cmd.Run()
			assert.Error(t, err, "Command should fail for invalid input")

			// Get the stderr output
			stderrOutput := stderr.String()

			// Verify we got output even with logging disabled
			assert.NotEmpty(t, stderrOutput,
				"Should have stderr output even with logging disabled")

			// Check that expected strings are present
			for _, expected := range tc.expectedInErr {
				assert.Contains(t, stderrOutput, expected,
					"stderr should contain '%s'", expected)
			}

			// Check that unwanted strings are NOT present
			for _, notExpected := range tc.notExpectedErr {
				assert.NotContains(t, stderrOutput, notExpected,
					"stderr should NOT contain '%s' (indicates improper logging usage)", notExpected)
			}
		})
	}
}

// TestValidationOutputNotUsingLogging verifies validation messages don't use log.Info.
func TestValidationOutputNotUsingLogging(t *testing.T) {
	if skipReason != "" {
		t.Skipf("%s", skipReason)
	}

	// Create a temporary test configuration
	tempDir := t.TempDir()

	// Create a minimal atmos.yaml
	atmosYaml := `
base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  name_pattern: "{stage}"
`
	err := os.WriteFile(tempDir+"/atmos.yaml", []byte(atmosYaml), 0o644)
	assert.NoError(t, err)

	// Create stacks directory
	err = os.MkdirAll(tempDir+"/stacks", 0o755)
	assert.NoError(t, err)

	// Create components directory
	err = os.MkdirAll(tempDir+"/components/terraform/test", 0o755)
	assert.NoError(t, err)

	// Create a simple stack config
	stackYaml := `
components:
  terraform:
    test:
      vars:
        enabled: true
`
	err = os.WriteFile(tempDir+"/stacks/dev.yaml", []byte(stackYaml), 0o644)
	assert.NoError(t, err)

	// Test validation command
	cmd := exec.Command("atmos", "validate", "stacks")
	cmd.Dir = tempDir
	cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Off")

	var stderr strings.Builder
	cmd.Stderr = &stderr

	// Run command (may succeed or fail depending on validation)
	_ = cmd.Run()

	stderrOutput := stderr.String()

	// Check that if there's output, it doesn't use INFO logging
	if stderrOutput != "" {
		assert.NotContains(t, stderrOutput, "INFO",
			"Validation output should not use log.Info")
		assert.NotContains(t, stderrOutput, "[1;38;5;86m",
			"Should not contain ANSI color codes from logging")
	}
}
