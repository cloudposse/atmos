//nolint:dupl // Cobra command boilerplate - structural similarity with remove.go is intentional.
package devcontainer

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var attachParser *flags.StandardFlagParser

// AttachOptions contains parsed flags for the attach command.
type AttachOptions struct {
	Instance string
	UsePTY   bool
}

var attachCmd = &cobra.Command{
	Use:   "attach <name>",
	Short: "Attach to a running devcontainer",
	Long: `Attach to a running devcontainer and get an interactive shell.

If the container is not running, it will be started automatically.

Experimental: Use --pty for PTY mode with masking support (not available on Windows).`,
	Example:           markdown.DevcontainerAttachUsageMarkdown,
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.attach.RunE")()

		parsed, err := attachParser.Parse(cmd.Context(), args)
		if err != nil {
			return err
		}
		opts := parseAttachOptions(parsed)

		name := parsed.PositionalArgs[0]
		mgr := devcontainer.NewManager()
		return mgr.Attach(atmosConfigPtr, name, opts.Instance, opts.UsePTY)
	},
}

// parseAttachOptions parses command flags into AttachOptions.
//
// ParseAttachOptions reads parsed flags into an AttachOptions value.
func parseAttachOptions(parsed *flags.ParsedConfig) *AttachOptions {
	return &AttachOptions{
		Instance: flags.GetString(parsed.Flags, "instance"),
		UsePTY:   flags.GetBool(parsed.Flags, "pty"),
	}
}

// init initializes the attach command by creating its flag parser (including `--instance` and `--pty` with environment variable bindings), attaching the parser to the command, and registering the command under devcontainerCmd.
func init() {
	// Create parser with attach-specific flags using functional options.
	var usage string
	attachParser, usage = newDevcontainerParser(
		true,
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("pty", "", false, "Experimental: Use PTY mode with masking support (not available on Windows)"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("pty", "ATMOS_DEVCONTAINER_PTY"),
	)
	attachCmd.Use = "attach " + usage

	initCommandWithFlags(attachCmd, attachParser)
	devcontainerCmd.AddCommand(attachCmd)
}
