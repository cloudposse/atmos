package devcontainer

import (
	"github.com/cloudposse/atmos/cmd/markdown"
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
	Example: markdown.DevcontainerLogsUsageMarkdown,
	Args:    cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
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
