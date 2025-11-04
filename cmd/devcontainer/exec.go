package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	execInstance    string
	execInteractive bool
)

var execCmd = &cobra.Command{
	Use:   "exec <name> -- <command> [args...]",
	Short: "Execute a command in a running devcontainer",
	Long: `Execute a command in a running devcontainer.

By default, runs in non-interactive mode where output is automatically masked.
Use --interactive for full TTY support (tab completion, colors, etc.) but note
that output masking will not be available in interactive mode.

The container must already be running. Use '--' to separate devcontainer arguments
from the command to execute.`,
	Example: markdown.DevcontainerExecUsageMarkdown,
	Args:    cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.exec.RunE")()

		name := args[0]
		command := args[1:]
		return e.ExecuteDevcontainerExec(atmosConfigPtr, name, execInstance, execInteractive, command)
	},
}

func init() {
	execCmd.Flags().StringVar(&execInstance, "instance", "default", "Instance name for this devcontainer")
	execCmd.Flags().BoolVarP(&execInteractive, "interactive", "i", false, "Enable interactive TTY mode (disables output masking)")
	devcontainerCmd.AddCommand(execCmd)
}
