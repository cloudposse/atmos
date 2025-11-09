package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/perf"
)

var configCmd = &cobra.Command{
	Use:   "config <name>",
	Short: "Show devcontainer configuration",
	Long: `Display the resolved configuration for a devcontainer.

This shows the final configuration after merging all sources including
imported devcontainer.json files.`,
	Example:           markdown.DevcontainerConfigUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.config.RunE")()

		name := args[0]
		return devcontainer.ShowConfig(atmosConfigPtr, name)
	},
}

func init() {
	devcontainerCmd.AddCommand(configCmd)
}
