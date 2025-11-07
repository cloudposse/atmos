package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	defaultStopTimeout = 10 // seconds
)

var (
	stopInstance string
	stopTimeout  int
	stopRm       bool // Remove container after stopping.
)

var stopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a running devcontainer",
	Long: `Stop a running devcontainer by name.

The container will be stopped but not removed, allowing you to restart it later
with all your work preserved.

Use --rm to automatically remove the container after stopping.`,
	Example:           markdown.DevcontainerStopUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.stop.RunE")()

		name := args[0]

		// Stop the container.
		if err := e.ExecuteDevcontainerStop(atmosConfigPtr, name, stopInstance, stopTimeout); err != nil {
			return err
		}

		// If --rm flag is set, remove the container after stopping.
		if stopRm {
			if err := e.ExecuteDevcontainerRemove(atmosConfigPtr, name, stopInstance, true); err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	stopCmd.Flags().StringVar(&stopInstance, "instance", "default", "Instance name for this devcontainer")
	stopCmd.Flags().IntVar(&stopTimeout, "timeout", defaultStopTimeout, "Timeout in seconds for stopping the container")
	stopCmd.Flags().BoolVar(&stopRm, "rm", false, "Automatically remove the container after stopping")
	devcontainerCmd.AddCommand(stopCmd)
}
