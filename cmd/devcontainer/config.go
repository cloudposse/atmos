package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var configParser *flags.StandardFlagParser

var configCmd = &cobra.Command{
	Use:   "config <name>",
	Short: "Show devcontainer configuration",
	Long: `Display the resolved configuration for a devcontainer.

This shows the final configuration after merging all sources including
imported devcontainer.json files.`,
	Example:           markdown.DevcontainerConfigUsageMarkdown,
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.config.RunE")()

		parsed, err := configParser.Parse(cmd.Context(), args)
		if err != nil {
			return err
		}

		name := parsed.PositionalArgs[0]
		mgr := devcontainer.NewManager()
		return mgr.ShowConfig(atmosConfigPtr, name)
	},
}

// init registers the config command as a subcommand of the devcontainer command.
func init() {
	var usage string
	configParser, usage = newDevcontainerParser(true)
	configCmd.Use = "config " + usage

	initCommandWithFlags(configCmd, configParser)
	devcontainerCmd.AddCommand(configCmd)
}
