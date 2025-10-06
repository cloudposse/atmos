package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInvalidFlagShowsUsage tests that invalid flags show usage, not config errors.
func TestInvalidFlagShowsUsage(t *testing.T) {
	// Create a temporary directory for the test.
	tmpDir := t.TempDir()

	// Create an atmos.yaml that would normally cause an error
	// (references a stacks directory that doesn't exist).
	atmosYaml := `
base_path: ""
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  excluded_paths: []
  name_pattern: "{stage}"
`
	atmosYamlPath := filepath.Join(tmpDir, "atmos.yaml")
	err := os.WriteFile(atmosYamlPath, []byte(atmosYaml), 0o644)
	require.NoError(t, err)

	// Build the atmos binary for testing.
	atmosBinary := filepath.Join(tmpDir, "atmos-test")
	buildCmd := exec.Command("go", "build", "-o", atmosBinary, ".")
	buildCmd.Dir = filepath.Join("..", ".") // Go up to project root
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "Failed to build atmos: %s", string(output))

	// Test 1: Invalid flag should show usage/error about the flag, NOT config error.
	t.Run("invalid_flag_shows_flag_error", func(t *testing.T) {
		cmd := exec.Command(atmosBinary, "--invalid-flag")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()

		// Command should fail.
		assert.Error(t, err)

		outputStr := string(output)

		// Should mention the invalid flag.
		assert.Contains(t, outputStr, "unknown flag", "Should mention unknown flag")
		assert.Contains(t, outputStr, "--invalid-flag", "Should mention the specific flag")

		// Should NOT mention stacks directory config error.
		assert.NotContains(t, outputStr, "stacks directory does not exist",
			"Should not show config error when flag is invalid")
		assert.NotContains(t, outputStr, "but the directory does not exist",
			"Should not show config error when flag is invalid")
	})

	// Test 2: Invalid command should show command error, NOT config error.
	t.Run("invalid_command_shows_command_error", func(t *testing.T) {
		cmd := exec.Command(atmosBinary, "invalid-command")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()

		// Command should fail.
		assert.Error(t, err)

		outputStr := string(output)

		// Should mention the invalid command (case insensitive check).
		assert.True(t,
			strings.Contains(outputStr, "unknown command") || strings.Contains(outputStr, "Unknown command"),
			"Should mention unknown command")

		// Should NOT mention stacks directory config error.
		assert.NotContains(t, outputStr, "stacks directory does not exist",
			"Should not show config error when command is invalid")
	})

	// Test 3: Valid command should show config error (if applicable).
	t.Run("valid_command_shows_config_error", func(t *testing.T) {
		cmd := exec.Command(atmosBinary, "list", "stacks")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()

		// Command should fail due to config error.
		assert.Error(t, err)

		outputStr := string(output)

		// NOW it should show the config error because the command/flags are valid.
		assert.True(t,
			strings.Contains(outputStr, "stacks") || strings.Contains(outputStr, "directory"),
			"Valid command should eventually show config errors if config is invalid")
	})

	// Test 4: Help flag should work even with bad config.
	t.Run("help_flag_works_with_bad_config", func(t *testing.T) {
		cmd := exec.Command(atmosBinary, "--help")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()

		// Help should succeed.
		assert.NoError(t, err, "Help should work even with bad config")

		outputStr := string(output)

		// Should show help content.
		assert.Contains(t, outputStr, "Usage:", "Should show usage")
		assert.Contains(t, outputStr, "atmos", "Should show atmos in help")

		// Should NOT show config error.
		assert.NotContains(t, outputStr, "stacks directory does not exist",
			"Help should not show config errors")
	})

	// Test 5: Version flag should work even with bad config.
	t.Run("version_flag_works_with_bad_config", func(t *testing.T) {
		cmd := exec.Command(atmosBinary, "version")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()

		// Version should succeed.
		assert.NoError(t, err, "Version should work even with bad config")

		outputStr := string(output)

		// Should NOT show config error.
		assert.NotContains(t, outputStr, "stacks directory does not exist",
			"Version should not show config errors")
	})
}
