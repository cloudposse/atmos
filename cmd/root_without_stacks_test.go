package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootWithoutStacksRequestsHelp(t *testing.T) {
	_ = NewTestKit(t)
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	command := &cobra.Command{Use: "atmos"}
	var helpArgs []string
	command.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		assert.Same(t, command, cmd)
		helpArgs = args
	})
	require.NoError(t, RootCmd.RunE(command, nil))
	assert.Equal(t, []string{helpFlagLong}, helpArgs, "explicit help avoids the missing-subcommand error path")
}
