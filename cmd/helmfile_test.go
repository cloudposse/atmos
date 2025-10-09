package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHelmfileCommands_Error(t *testing.T) {
	skipIfHelmfileNotInstalled(t)
	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Unset ENV variables after testing
	defer func() {
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		os.Unsetenv("ATMOS_BASE_PATH")
	}()

	err = helmfileApplyCmd.RunE(helmfileApplyCmd, []string{})
	assert.Error(t, err, "helmfile apply command should return an error when called with no parameters")

	err = helmfileDestroyCmd.RunE(helmfileDestroyCmd, []string{})
	assert.Error(t, err, "helmfile destroy command should return an error when called with no parameters")

	err = helmfileDiffCmd.RunE(helmfileDiffCmd, []string{})
	assert.Error(t, err, "helmfile diff command should return an error when called with no parameters")

	err = helmfileSyncCmd.RunE(helmfileSyncCmd, []string{})
	assert.Error(t, err, "helmfile sync command should return an error when called with no parameters")
}
