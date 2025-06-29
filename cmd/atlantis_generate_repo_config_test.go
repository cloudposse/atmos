package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtlantisGenerateRepoConfigCmd_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/atlantis-generate-repo-config"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Unset ENV variables after testing
	defer func() {
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		os.Unsetenv("ATMOS_BASE_PATH")
	}()

	err = atlantisGenerateRepoConfigCmd.RunE(atlantisGenerateRepoConfigCmd, []string{})
	assert.Error(t, err, "atlantis generate repo-config command should return an error when called with no parameters")
}
