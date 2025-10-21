package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupTestCommand creates a test command with the necessary flags.
func TestVendorCommands_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := vendorPullCmd.RunE(vendorPullCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "vendor pull command should return an error when called with invalid flags")
}
