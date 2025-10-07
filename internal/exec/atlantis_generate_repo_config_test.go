package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestExecuteAtlantisGenerateRepoConfigWithStackNameTemplate(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/atlantis-generate-repo-config"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Unset ENV variables after testing
	defer func() {
		os.Unsetenv("ATMOS_BASE_PATH")
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	}()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	err = ExecuteAtlantisGenerateRepoConfig(
		&atmosConfig,
		"/dev/stdout",
		"config-1",
		"project-1",
		nil,
		nil,
	)

	assert.Nil(t, err)
}

func TestExecuteAtlantisGenerateRepoConfigAffectedOnly(t *testing.T) {
	// Skip long tests in short mode (this test takes ~21 seconds due to Git operations)
	tests.SkipIfShort(t)

	// Check for Git repository with valid remotes precondition
	tests.RequireGitRemoteWithValidURL(t)

	stacksPath := "../../tests/fixtures/scenarios/atlantis-generate-repo-config"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Unset ENV variables after testing
	defer func() {
		os.Unsetenv("ATMOS_BASE_PATH")
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	}()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	err = ExecuteAtlantisGenerateRepoConfigAffectedOnly(
		&atmosConfig,
		"/dev/stdout",
		"config-1",
		"project-1",
		"",
		"",
		"",
		"",
		"",
		true,
		"",
	)

	assert.Nil(t, err)
}
