package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupTestCommand creates a test command with the necessary flags.
func TestValidateCommands_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := ValidateStacksCmd.RunE(ValidateStacksCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "validate stacks command should return an error when called with invalid flags")

	err = validateComponentCmd.RunE(validateComponentCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "validate component command should return an error when called with invalid flags")
}
