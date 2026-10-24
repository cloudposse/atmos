package devcontainer

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	defaultStopTimeout = 10 // seconds
)

var stopParser *flags.StandardFlagParser

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
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.stop.RunE")()

		parsed, err := stopParser.Parse(cmd.Context(), args)
		if err != nil {
			return err
		}
		opts := parseStopOptions(parsed)

		mgr := devcontainer.NewManager()
		name := parsed.PositionalArgs[0]

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
// ParseStopOptions reads parsed flags into a StopOptions value.
func parseStopOptions(parsed *flags.ParsedConfig) *StopOptions {
	return &StopOptions{
		Instance: flags.GetString(parsed.Flags, "instance"),
		Timeout:  flags.GetInt(parsed.Flags, "timeout"),
		Rm:       flags.GetBool(parsed.Flags, "rm"),
	}
}

// init initializes the stop command's flag parser and registers the stop subcommand.
func init() {
	// Create parser with stop-specific flags using functional options.
	var usage string
	stopParser, usage = newDevcontainerParser(
		true,
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithIntFlag("timeout", "", defaultStopTimeout, "Timeout in seconds for stopping the container"),
		flags.WithBoolFlag("rm", "", false, "Automatically remove the container after stopping"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("timeout", "ATMOS_DEVCONTAINER_TIMEOUT"),
		flags.WithEnvVars("rm", "ATMOS_DEVCONTAINER_RM"),
	)
	stopCmd.Use = "stop " + usage

	initCommandWithFlags(stopCmd, stopParser)
	devcontainerCmd.AddCommand(stopCmd)
}
