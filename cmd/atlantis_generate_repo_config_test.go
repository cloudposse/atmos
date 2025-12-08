package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtlantisGenerateRepoConfigCmd_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/atlantis-generate-repo-config"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := atlantisGenerateRepoConfigCmd.RunE(atlantisGenerateRepoConfigCmd, []string{})
	assert.Error(t, err, "atlantis generate repo-config command should return an error when called with no parameters")
}
