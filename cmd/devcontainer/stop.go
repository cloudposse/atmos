package devcontainer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	defaultStopTimeout = 10 // seconds
)

var stopParser *flags.StandardParser

// StopOptions contains parsed flags for the stop command.
type StopOptions struct {
	Instance string
	Timeout  int
	Rm       bool
}

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

		// Parse flags using new options pattern.
		v := viper.GetViper()
		if err := stopParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseStopOptions(cmd, v, args)
		if err != nil {
			return err
		}

		mgr := devcontainer.NewManager()
		name := args[0]

		// Stop the container.
		if err := mgr.Stop(atmosConfigPtr, name, opts.Instance, opts.Timeout); err != nil {
			return err
		}

		// If --rm flag is set, remove the container after stopping.
		if opts.Rm {
			if err := mgr.Remove(atmosConfigPtr, name, opts.Instance, true); err != nil {
				return err
			}
		}

		return nil
	},
}

// parseStopOptions parses command flags into StopOptions.
//
//nolint:unparam // args parameter kept for consistency with other parse functions
func parseStopOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*StopOptions, error) {
	return &StopOptions{
		Instance: v.GetString("instance"),
		Timeout:  v.GetInt("timeout"),
		Rm:       v.GetBool("rm"),
	}, nil
}

func init() {
	// Create parser with stop-specific flags using functional options.
	stopParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithIntFlag("timeout", "", defaultStopTimeout, "Timeout in seconds for stopping the container"),
		flags.WithBoolFlag("rm", "", false, "Automatically remove the container after stopping"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("timeout", "ATMOS_DEVCONTAINER_TIMEOUT"),
		flags.WithEnvVars("rm", "ATMOS_DEVCONTAINER_RM"),
	)

	initCommandWithFlags(stopCmd, stopParser)
	devcontainerCmd.AddCommand(stopCmd)
}
