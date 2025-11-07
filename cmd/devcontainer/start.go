package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	startInstance string
	startAttach   bool
	startIdentity string
)

var startCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a devcontainer",
	Long: `Start a devcontainer by name.

If the container doesn't exist, it will be created. If it exists but is stopped,
it will be started. Use --instance to manage multiple instances of the same devcontainer.

Use --identity to launch the container with Atmos-managed credentials.`,
	Example:           markdown.DevcontainerStartUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.start.RunE")()

		name := args[0]
		if err := e.ExecuteDevcontainerStart(atmosConfigPtr, name, startInstance, startIdentity); err != nil {
			return err
		}

		// If --attach flag is set, attach to the container after starting
		if startAttach {
			return e.ExecuteDevcontainerAttach(atmosConfigPtr, name, startInstance, false)
		}

		return nil
	},
}

func init() {
	startCmd.Flags().StringVar(&startInstance, "instance", "default", "Instance name for this devcontainer")
	startCmd.Flags().BoolVar(&startAttach, "attach", false, "Attach to the container after starting")
	startCmd.Flags().StringVarP(&startIdentity, "identity", "i", "", "Authenticate with specified identity")
	devcontainerCmd.AddCommand(startCmd)
}
