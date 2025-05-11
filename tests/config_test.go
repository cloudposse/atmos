package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/internal/exec"
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

func TestFindAllStackConfigsInPaths(t *testing.T) {
	stacksPath := "./fixtures/scenarios/atmos-overrides-section"

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

	_, relativePaths, err := config.FindAllStackConfigsInPaths(
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

func TestValidateStacks(t *testing.T) {
	basePath := "./fixtures/scenarios/atmos-overrides-section/stacks"

	basePathAbs, err := filepath.Abs(basePath)
	assert.Nil(t, err)

	stackConfigFilesAbsolutePaths := []string{
		filepath.Join(basePathAbs, "deploy", "dev.yaml"),
		filepath.Join(basePathAbs, "deploy", "prod.yaml"),
		filepath.Join(basePathAbs, "deploy", "sandbox.yaml"),
		filepath.Join(basePathAbs, "deploy", "staging.yaml"),
		filepath.Join(basePathAbs, "deploy", "test.yaml"),
	}

	atmosConfig := schema.AtmosConfiguration{
		BasePath:                      basePath,
		StacksBaseAbsolutePath:        basePathAbs,
		StackConfigFilesAbsolutePaths: stackConfigFilesAbsolutePaths,
		Stacks: schema.Stacks{
			NamePattern: "{stage}",
		},
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	err = exec.ValidateStacks(atmosConfig)
	assert.Nil(t, err)
}
