package devcontainer

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available devcontainers",
	Long: `List all devcontainers defined in your Atmos configuration.

Shows the name, image, and configured ports for each devcontainer.`,
	Example: `  # List all devcontainers
  atmos devcontainer list

  # List with specific format
  atmos devcontainer list --format json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.list.RunE")()

		return e.ExecuteDevcontainerList(atmosConfigPtr)
	},
}

func init() {
	devcontainerCmd.AddCommand(listCmd)
}
