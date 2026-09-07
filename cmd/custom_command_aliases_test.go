package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommand_NativeAliasesAndInternal verifies schema.Command.Aliases/Internal wire
// onto the constructed *cobra.Command as native, in-process Cobra aliases/Hidden -- distinct
// from the subprocess-based CommandAliases mechanism.
func TestCustomCommand_NativeAliasesAndInternal(t *testing.T) {
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	atmosConfig.Commands = []schema.Command{
		{
			Name:     "na-deploy",
			Aliases:  []string{"na-dep", "na-d"},
			Internal: true,
			Steps:    schema.Tasks{{Type: "shell", Command: "true"}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var deployCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "na-deploy" {
			deployCmd = c
			break
		}
	}
	require.NotNil(t, deployCmd, "na-deploy command should be registered")

	assert.True(t, deployCmd.Hidden, "internal: true must set cobra Hidden")
	assert.ElementsMatch(t, []string{"na-dep", "na-d"}, deployCmd.Aliases)
	assert.True(t, deployCmd.HasAlias("na-dep"))
	assert.True(t, deployCmd.HasAlias("na-d"))
}
