package devcontainer

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var execParser *flags.StandardFlagParser

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
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.exec.RunE")()

		parsed, err := execParser.Parse(cmd.Context(), args)
		if err != nil {
			return err
		}
		opts := parseExecOptions(parsed)

		name, command, err := parseExecInvocation(args, parsed)
		if err != nil {
			return err
		}

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
// ParseExecOptions constructs an ExecOptions populated from parsed configuration values.
func parseExecOptions(parsed *flags.ParsedConfig) *ExecOptions {
	return &ExecOptions{
		Instance:    flags.GetString(parsed.Flags, "instance"),
		Interactive: flags.GetBool(parsed.Flags, "interactive"),
		UsePTY:      flags.GetBool(parsed.Flags, "pty"),
	}
}

func parseExecInvocation(args []string, parsed *flags.ParsedConfig) (string, []string, error) {
	hasSeparator := slices.Contains(args, "--")
	if hasSeparator {
		if len(parsed.PositionalArgs) != 1 {
			return "", nil, fmt.Errorf("%w: devcontainer exec requires exactly one name before `--`", errUtils.ErrInvalidArguments)
		}
		if len(parsed.SeparatedArgs) == 0 {
			return "", nil, fmt.Errorf("%w: devcontainer exec requires a command after `--`", errUtils.ErrInvalidArguments)
		}
		return parsed.PositionalArgs[0], parsed.SeparatedArgs, nil
	}

	if len(parsed.PositionalArgs) < 2 {
		return "", nil, fmt.Errorf("%w: devcontainer exec requires a name and command", errUtils.ErrInvalidArguments)
	}
	return parsed.PositionalArgs[0], parsed.PositionalArgs[1:], nil
}

// init registers the exec subcommand and its flags.
func init() {
	// Create parser with exec-specific flags using functional options.
	execParser = flags.NewStandardFlagParser(
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
