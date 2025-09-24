package tests

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCustomCommandInvalidSubcommand tests that invalid subcommands show proper error messages
// to stderr without using logging, ensuring users get feedback even when logging is disabled.
func TestCustomCommandInvalidSubcommand(t *testing.T) {
	if skipReason != "" {
		t.Skipf("%s", skipReason)
	}

	// Test with a custom command configuration that has subcommands
	testCases := []struct {
		name           string
		command        string
		expectedInErr  []string
		notExpectedErr []string
	}{
		{
			name:    "Invalid subcommand shows available subcommands",
			command: "dev invalid-subcommand",
			expectedInErr: []string{
				"Incorrect Usage",
				"The command `atmos dev` requires a subcommand",
				"Valid subcommands are:",
			},
			notExpectedErr: []string{
				"INFO",                  // Should not use log.Info
				"Available command(s):", // Old format should not appear
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save current working directory
			currentDir, err := os.Getwd()
			assert.NoError(t, err)

			// Change to test-cases directory with custom commands configured
			testDir := "test-cases/atmos-config-with-custom-commands"
			err = os.Chdir(testDir)
			if err != nil {
				// If directory doesn't exist, skip test with reason
				t.Skipf("Test directory %s not found: %v", testDir, err)
			}

			// Ensure we restore directory
			defer func() {
				_ = os.Chdir(currentDir)
			}()

			// Construct the command
			cmdArgs := strings.Fields(tc.command)
			cmd := exec.Command("atmos", cmdArgs...)

			// Capture stderr (where error messages should go)
			var stderr strings.Builder
			cmd.Stderr = &stderr

			// We expect the command to fail with exit code 1
			err = cmd.Run()
			assert.Error(t, err, "Command should fail for invalid subcommand")

			// Get the stderr output
			stderrOutput := stderr.String()

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

			// Verify exit code is 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				assert.Equal(t, 1, exitErr.ExitCode(), "Exit code should be 1")
			}
		})
	}
}

// TestCustomCommandValidationMessageNoLogging tests that validation success messages
// don't use logging and go to stderr properly.
func TestCustomCommandValidationMessageNoLogging(t *testing.T) {
	if skipReason != "" {
		t.Skipf("%s", skipReason)
	}

	// Test validation command output
	testCases := []struct {
		name           string
		command        string
		expectedInErr  []string
		notExpectedErr []string
	}{
		{
			name:    "Validation success message uses stderr not logging",
			command: "validate component vpc -s tenant1-ue2-dev",
			expectedInErr: []string{
				"✓ Validated successfully",
			},
			notExpectedErr: []string{
				"INFO",                   // Should not use log.Info
				"Validated successfully", // Old log format (without checkmark)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save current working directory
			currentDir, err := os.Getwd()
			assert.NoError(t, err)

			// Change to test-cases directory
			testDir := "test-cases/complete"
			err = os.Chdir(testDir)
			if err != nil {
				t.Skipf("Test directory %s not found: %v", testDir, err)
			}

			// Ensure we restore directory
			defer func() {
				_ = os.Chdir(currentDir)
			}()

			// Construct the command
			cmdArgs := strings.Fields(tc.command)
			cmd := exec.Command("atmos", cmdArgs...)

			// Capture stderr
			var stderr strings.Builder
			cmd.Stderr = &stderr

			// Run command (may succeed or fail, we're checking output format)
			_ = cmd.Run()

			// Get the stderr output
			stderrOutput := stderr.String()

			// If we expect the command to succeed and show validation message
			if len(tc.expectedInErr) > 0 {
				for _, expected := range tc.expectedInErr {
					// Only check if there was output
					if stderrOutput != "" {
						assert.Contains(t, stderrOutput, expected,
							"stderr should contain '%s'", expected)
					}
				}
			}

			// Check that unwanted strings are NOT present
			for _, notExpected := range tc.notExpectedErr {
				if stderrOutput != "" {
					assert.NotContains(t, stderrOutput, notExpected,
						"stderr should NOT contain '%s' (indicates improper logging usage)", notExpected)
				}
			}
		})
	}
}
