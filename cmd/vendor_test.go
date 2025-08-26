package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupTestCommand creates a test command with the necessary flags.
func TestVendorCommands_Error(t *testing.T) {
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

	err = vendorPullCmd.RunE(vendorPullCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "vendor pull command should return an error when called with invalid flags")

	err = vendorDiffCmd.RunE(vendorDiffCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "vendor diff command should return an error when called with invalid flags")
}
