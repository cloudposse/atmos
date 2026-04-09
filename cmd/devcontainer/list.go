package devcontainer

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/perf"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available devcontainers",
	Long: `List all devcontainers defined in your Atmos configuration.

Shows the name, image, and configured ports for each devcontainer.`,
	Example: markdown.DevcontainerListUsageMarkdown,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.list.RunE")()

		mgr := devcontainer.NewManager()
		return mgr.List(atmosConfigPtr)
	},
}

// init registers the list subcommand under the devcontainer command.
func init() {
	devcontainerCmd.AddCommand(listCmd)
}
