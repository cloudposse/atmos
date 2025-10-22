package devcontainer

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	startInstance string
	startAttach   bool
)

var startCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a devcontainer",
	Long: `Start a devcontainer by name.

If the container doesn't exist, it will be created. If it exists but is stopped,
it will be started. Use --instance to manage multiple instances of the same devcontainer.`,
	Example: `  # Start the default devcontainer
  atmos devcontainer start default

  # Start and attach to the container
  atmos devcontainer start default --attach

  # Start a specific instance
  atmos devcontainer start terraform --instance my-instance

  # Start with custom runtime
  export ATMOS_CONTAINER_RUNTIME=podman
  atmos devcontainer start default`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.start.RunE")()

		name := args[0]
		if err := e.ExecuteDevcontainerStart(atmosConfigPtr, name, startInstance); err != nil {
			return err
		}

		// If --attach flag is set, attach to the container after starting
		if startAttach {
			return e.ExecuteDevcontainerAttach(atmosConfigPtr, name, startInstance)
		}

		return nil
	},
}

func init() {
	startCmd.Flags().StringVar(&startInstance, "instance", "default", "Instance name for this devcontainer")
	startCmd.Flags().BoolVar(&startAttach, "attach", false, "Attach to the container after starting")
	devcontainerCmd.AddCommand(startCmd)
}
