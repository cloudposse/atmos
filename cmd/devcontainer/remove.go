package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	removeInstance string
	removeForce    bool
)

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a devcontainer",
	Long: `Remove a devcontainer and all its data.

This will stop the container if it's running and remove it completely.
Use --force to remove a running container without stopping it first.`,
	Example:           markdown.DevcontainerRemoveUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.remove.RunE")()

		name := args[0]
		return e.ExecuteDevcontainerRemove(atmosConfigPtr, name, removeInstance, removeForce)
	},
}

func init() {
	removeCmd.Flags().StringVar(&removeInstance, "instance", "default", "Instance name for this devcontainer")
	removeCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "Force remove even if running")
	devcontainerCmd.AddCommand(removeCmd)
}
