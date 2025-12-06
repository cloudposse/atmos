package devcontainer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var logsParser *flags.StandardParser

// LogsOptions contains parsed flags for the logs command.
type LogsOptions struct {
	Instance string
	Follow   bool
	Tail     string
}

var logsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Show logs from a devcontainer",
	Long: `Show logs from a running or stopped devcontainer.

By default, shows all logs. Use --follow to stream logs in real-time,
or --tail to limit the number of lines shown.`,
	Example:           markdown.DevcontainerLogsUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.logs.RunE")()

		// Parse flags using new options pattern.
		v := viper.GetViper()
		if err := logsParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseLogsOptions(cmd, v, args)
		if err != nil {
			return err
		}

		name := args[0]
		mgr := devcontainer.NewManager()
		return mgr.Logs(atmosConfigPtr, name, opts.Instance, opts.Follow, opts.Tail)
	},
}

// parseLogsOptions parses command flags into LogsOptions.
//
// ParseLogsOptions creates a LogsOptions populated from the command's Viper-backed flags.
// The cmd and args parameters are unused but retained for API consistency with other parsers.
func parseLogsOptions(_ *cobra.Command, v *viper.Viper, _ []string) (*LogsOptions, error) {
	return &LogsOptions{
		Instance: v.GetString("instance"),
		Follow:   v.GetBool("follow"),
		Tail:     v.GetString("tail"),
	}, nil
}

// init initializes the logs command by creating its flags parser (instance, follow, tail with corresponding environment bindings), registering those flags with the command, and adding logsCmd to devcontainerCmd.
func init() {
	// Create parser with logs-specific flags using functional options.
	logsParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("follow", "f", false, "Follow log output"),
		flags.WithStringFlag("tail", "", "all", "Number of lines to show from the end of the logs"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("follow", "ATMOS_DEVCONTAINER_FOLLOW"),
		flags.WithEnvVars("tail", "ATMOS_DEVCONTAINER_TAIL"),
	)

	initCommandWithFlags(logsCmd, logsParser)
	devcontainerCmd.AddCommand(logsCmd)
}
