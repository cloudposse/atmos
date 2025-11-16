package devcontainer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var execParser *flags.StandardParser

// ExecOptions contains parsed flags for the exec command.
type ExecOptions struct {
	Instance    string
	Interactive bool
	UsePTY      bool
}

var execCmd = &cobra.Command{
	Use:   "exec <name> -- <command> [args...]",
	Short: "Execute a command in a running devcontainer",
	Long: `Execute a command in a running devcontainer.

By default, runs in non-interactive mode where output is automatically masked.
Use --interactive for full TTY support (tab completion, colors, etc.) but note
that output masking will not be available in interactive mode.

Experimental: Use --pty for PTY mode with masking support (not available on Windows).

The container must already be running. Use '--' to separate devcontainer arguments
from the command to execute.`,
	Example: markdown.DevcontainerExecUsageMarkdown,
	Args:    cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.exec.RunE")()

		// Parse flags using new options pattern.
		v := viper.GetViper()
		if err := execParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseExecOptions(cmd, v, args)
		if err != nil {
			return err
		}

		name := args[0]
		command := args[1:]
		mgr := devcontainer.NewManager()
		return mgr.Exec(atmosConfigPtr, devcontainer.ExecParams{
			Name:        name,
			Instance:    opts.Instance,
			Interactive: opts.Interactive,
			UsePTY:      opts.UsePTY,
			Command:     command,
		})
	},
}

// parseExecOptions parses command flags into ExecOptions.
//
//nolint:unparam // args parameter kept for consistency with other parse functions
func parseExecOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*ExecOptions, error) {
	return &ExecOptions{
		Instance:    v.GetString("instance"),
		Interactive: v.GetBool("interactive"),
		UsePTY:      v.GetBool("pty"),
	}, nil
}

func init() {
	// Create parser with exec-specific flags using functional options.
	execParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("interactive", "i", false, "Enable interactive TTY mode (disables output masking)"),
		flags.WithBoolFlag("pty", "", false, "Experimental: Use PTY mode with masking support (not available on Windows)"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("interactive", "ATMOS_DEVCONTAINER_INTERACTIVE"),
		flags.WithEnvVars("pty", "ATMOS_DEVCONTAINER_PTY"),
	)

	initCommandWithFlags(execCmd, execParser)
	devcontainerCmd.AddCommand(execCmd)
}
