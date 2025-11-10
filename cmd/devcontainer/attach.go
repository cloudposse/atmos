package devcontainer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var attachParser *flags.StandardParser

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
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.attach.RunE")()

		// Parse flags using new options pattern.
		v := viper.GetViper()
		if err := attachParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseAttachOptions(cmd, v, args)
		if err != nil {
			return err
		}

		name := args[0]
		mgr := devcontainer.NewManager()
		return mgr.Attach(atmosConfigPtr, name, opts.Instance, opts.UsePTY)
	},
}

// parseAttachOptions parses command flags into AttachOptions.
//
//nolint:unparam // args parameter kept for consistency with other parse functions
func parseAttachOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*AttachOptions, error) {
	return &AttachOptions{
		Instance: v.GetString("instance"),
		UsePTY:   v.GetBool("pty"),
	}, nil
}

func init() {
	// Create parser with attach-specific flags using functional options.
	attachParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("pty", "", false, "Experimental: Use PTY mode with masking support (not available on Windows)"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("pty", "ATMOS_DEVCONTAINER_PTY"),
	)

	// Register flags using the standard RegisterFlags method.
	attachParser.RegisterFlags(attachCmd)

	// Bind flags to Viper for environment variable support.
	if err := attachParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	devcontainerCmd.AddCommand(attachCmd)
}
