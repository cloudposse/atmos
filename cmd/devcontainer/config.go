package devcontainer

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var configCmd = &cobra.Command{
	Use:   "config <name>",
	Short: "Show devcontainer configuration",
	Long: `Display the resolved configuration for a devcontainer.

This shows the final configuration after merging all sources including
imported devcontainer.json files.`,
	Example: `  # Show configuration for default devcontainer
  atmos devcontainer config default

  # Show configuration in JSON format
  atmos devcontainer config terraform --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.config.RunE")()

		name := args[0]
		return e.ExecuteDevcontainerConfig(atmosConfigPtr, name)
	},
}

func init() {
	devcontainerCmd.AddCommand(configCmd)
}
