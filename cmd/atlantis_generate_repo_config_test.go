package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtlantisGenerateRepoConfigCmd(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/atlantis-generate-repo-config"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_LOGS_LEVEL", "Info")
	assert.NoError(t, err, "Setting 'ATMOS_LOGS_LEVEL' environment variable should execute without error")

	// Unset ENV variables after testing
	defer func() {
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		os.Unsetenv("ATMOS_BASE_PATH")
		os.Unsetenv("ATMOS_LOGS_LEVEL")
	}()

	// Execute the command
	RootCmd.SetArgs([]string{"atlantis", "generate", "repo-config", "--output-path", "/dev/stdout", "--config-template", "config-1", "--project-template", "project-1"})
	err = Execute()
	assert.NoError(t, err, "'TestTerraformGenerateBackendCmd' should execute without error")
}
