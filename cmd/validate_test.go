package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupTestCommand creates a test command with the necessary flags.
func TestValidateCommands_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Unset ENV variables after testing
	defer func() {
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		os.Unsetenv("ATMOS_BASE_PATH")
	}()

	err = ValidateStacksCmd.RunE(ValidateStacksCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "validate stacks command should return an error when called with invalid flags")

	err = validateComponentCmd.RunE(validateComponentCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "validate component command should return an error when called with invalid flags")
}
