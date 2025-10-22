package devcontainer

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	logsInstance string
	logsFollow   bool
	logsTail     string
)

var logsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Show logs from a devcontainer",
	Long: `Show logs from a running or stopped devcontainer.

By default, shows all logs. Use --follow to stream logs in real-time,
or --tail to limit the number of lines shown.`,
	Example: `  # Show all logs from a devcontainer
  atmos devcontainer logs default

  # Follow logs in real-time
  atmos devcontainer logs default --follow

  # Show last 100 lines
  atmos devcontainer logs default --tail 100

  # Show logs from a specific instance
  atmos devcontainer logs terraform --instance my-instance`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.logs.RunE")()

		name := args[0]
		return e.ExecuteDevcontainerLogs(atmosConfigPtr, name, logsInstance, logsFollow, logsTail)
	},
}

func init() {
	logsCmd.Flags().StringVar(&logsInstance, "instance", "default", "Instance name for this devcontainer")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().StringVar(&logsTail, "tail", "all", "Number of lines to show from the end of the logs")
	devcontainerCmd.AddCommand(logsCmd)
}
