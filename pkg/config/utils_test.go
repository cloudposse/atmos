package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFindAllStackConfigsInPathsForStack(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/stack-templates-2"

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
	atmosConfig, err := InitCliConfig(configAndStacksInfo, true)
	assert.NoError(t, err)

	_, relativePaths, _, err := FindAllStackConfigsInPathsForStack(
		atmosConfig,
		"nonprod",
		atmosConfig.IncludeStackAbsolutePaths,
		atmosConfig.ExcludeStackAbsolutePaths,
	)
	assert.NoError(t, err)
	assert.Equal(t, "deploy/nonprod.yaml", relativePaths[0])
}

func TestFindAllStackConfigsInPaths(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/atmos-overrides-section"

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
	atmosConfig, err := InitCliConfig(configAndStacksInfo, true)
	assert.NoError(t, err)

	_, relativePaths, err := FindAllStackConfigsInPaths(
		atmosConfig,
		atmosConfig.IncludeStackAbsolutePaths,
		atmosConfig.ExcludeStackAbsolutePaths,
	)
	assert.NoError(t, err)
	assert.Equal(t, "deploy/dev.yaml", relativePaths[0])
	assert.Equal(t, "deploy/prod.yaml", relativePaths[1])
	assert.Equal(t, "deploy/sandbox.yaml", relativePaths[2])
	assert.Equal(t, "deploy/staging.yaml", relativePaths[3])
}
