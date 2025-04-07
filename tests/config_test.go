package tests

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestFindAllStackConfigsInPathsForStack(t *testing.T) {
	stacksPath := "./fixtures/scenarios/stack-templates-2"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Unset env values after testing
	defer func() {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		assert.NoError(t, err)
		err = os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		assert.NoError(t, err)
	}()

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	assert.NoError(t, err)
	atmosConfig.BasePath = stacksPath
	_, relativePaths, _, err := config.FindAllStackConfigsInPathsForStack(
		atmosConfig,
		"nonprod",
		atmosConfig.IncludeStackAbsolutePaths,
		atmosConfig.ExcludeStackAbsolutePaths,
	)
	assert.NoError(t, err)
	assert.Equal(t, "deploy/nonprod.yaml", relativePaths[0])
}
