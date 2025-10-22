package devcontainer

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var attachInstance string

var attachCmd = &cobra.Command{
	Use:   "attach <name>",
	Short: "Attach to a running devcontainer",
	Long: `Attach to a running devcontainer and get an interactive shell.

If the container is not running, it will be started automatically.`,
	Example: `  # Attach to the default devcontainer
  atmos devcontainer attach default

  # Attach to a specific instance
  atmos devcontainer attach terraform --instance my-instance`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.attach.RunE")()

		name := args[0]
		return e.ExecuteDevcontainerAttach(atmosConfigPtr, name, attachInstance)
	},
}

func init() {
	attachCmd.Flags().StringVar(&attachInstance, "instance", "default", "Instance name for this devcontainer")
	devcontainerCmd.AddCommand(attachCmd)
}
