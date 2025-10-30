package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var execInstance string

var execCmd = &cobra.Command{
	Use:   "exec <name> -- <command> [args...]",
	Short: "Execute a command in a running devcontainer",
	Long: `Execute a command in a running devcontainer without attaching interactively.

The container must already be running. Use '--' to separate devcontainer arguments
from the command to execute.`,
	Example: markdown.DevcontainerExecUsageMarkdown,
	Args:    cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.exec.RunE")()

		name := args[0]
		command := args[1:]
		return e.ExecuteDevcontainerExec(atmosConfigPtr, name, execInstance, command)
	},
}

func init() {
	execCmd.Flags().StringVar(&execInstance, "instance", "default", "Instance name for this devcontainer")
	devcontainerCmd.AddCommand(execCmd)
}
