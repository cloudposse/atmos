package devcontainer

import (
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
	Example: `  # Run a command in the default devcontainer
  atmos devcontainer exec default -- terraform version

  # Run a command in a specific instance
  atmos devcontainer exec terraform --instance my-instance -- make build

  # Run a shell command
  atmos devcontainer exec default -- sh -c "echo Hello from container"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.exec.RunE")()

		name := args[0]
		var command []string
		if len(args) > 1 {
			command = args[1:]
		}
		return e.ExecuteDevcontainerExec(atmosConfigPtr, name, execInstance, command)
	},
}

func init() {
	execCmd.Flags().StringVar(&execInstance, "instance", "default", "Instance name for this devcontainer")
	devcontainerCmd.AddCommand(execCmd)
}
