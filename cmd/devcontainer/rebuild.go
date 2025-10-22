package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	rebuildInstance string
	rebuildAttach   bool
	rebuildNoPull   bool
)

var rebuildCmd = &cobra.Command{
	Use:   "rebuild <name>",
	Short: "Rebuild a devcontainer",
	Long: `Rebuild a devcontainer from scratch.

This command stops and removes the existing container, pulls the latest image
(unless --no-pull is specified), and creates a new container with the current
configuration. This is useful when you've updated the devcontainer.json or
need to start fresh.`,
	Example: markdown.DevcontainerRebuildUsageMarkdown,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.rebuild.RunE")()

		name := args[0]
		if err := e.ExecuteDevcontainerRebuild(atmosConfigPtr, name, rebuildInstance, rebuildNoPull); err != nil {
			return err
		}

		// If --attach flag is set, attach to the container after rebuilding.
		if rebuildAttach {
			return e.ExecuteDevcontainerAttach(atmosConfigPtr, name, rebuildInstance)
		}

		return nil
	},
}

func init() {
	rebuildCmd.Flags().StringVar(&rebuildInstance, "instance", "default", "Instance name for this devcontainer")
	rebuildCmd.Flags().BoolVar(&rebuildAttach, "attach", false, "Attach to the container after rebuilding")
	rebuildCmd.Flags().BoolVar(&rebuildNoPull, "no-pull", false, "Don't pull the latest image before rebuilding")
	devcontainerCmd.AddCommand(rebuildCmd)
}
