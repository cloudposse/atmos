package devcontainer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var rebuildParser *flags.StandardParser

// RebuildOptions contains parsed flags for the rebuild command.
type RebuildOptions struct {
	Instance string
	Attach   bool
	NoPull   bool
	Identity string
}

var rebuildCmd = &cobra.Command{
	Use:   "rebuild <name>",
	Short: "Rebuild a devcontainer",
	Long: `Rebuild a devcontainer from scratch.

This command stops and removes the existing container, pulls the latest image
(unless --no-pull is specified), and creates a new container with the current
configuration. This is useful when you've updated the devcontainer.json or
need to start fresh.`,
	Example:           markdown.DevcontainerRebuildUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.rebuild.RunE")()

		// Parse flags using new options pattern.
		v := viper.GetViper()
		if err := rebuildParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseRebuildOptions(cmd, v, args)
		if err != nil {
			return err
		}

		name := args[0]
		mgr := devcontainer.NewManager()
		if err := mgr.Rebuild(atmosConfigPtr, name, opts.Instance, opts.Identity, opts.NoPull); err != nil {
			return err
		}

		// If --attach flag is set, attach to the container after rebuilding.
		if opts.Attach {
			return mgr.Attach(atmosConfigPtr, name, opts.Instance, false)
		}

		return nil
	},
}

// parseRebuildOptions parses command flags into RebuildOptions.
//
//nolint:unparam // args parameter kept for consistency with other parse functions
func parseRebuildOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*RebuildOptions, error) {
	return &RebuildOptions{
		Instance: v.GetString("instance"),
		Attach:   v.GetBool("attach"),
		NoPull:   v.GetBool("no-pull"),
		Identity: v.GetString("identity"),
	}, nil
}

func init() {
	// Create parser with rebuild-specific flags using functional options.
	rebuildParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("attach", "", false, "Attach to the container after rebuilding"),
		flags.WithBoolFlag("no-pull", "", false, "Don't pull the latest image before rebuilding"),
		flags.WithStringFlag("identity", "i", "", "Authenticate with specified identity"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("attach", "ATMOS_DEVCONTAINER_ATTACH"),
		flags.WithEnvVars("no-pull", "ATMOS_DEVCONTAINER_NO_PULL"),
		flags.WithEnvVars("identity", "ATMOS_DEVCONTAINER_IDENTITY"),
	)

	// Register flags using the standard RegisterFlags method.
	rebuildParser.RegisterFlags(rebuildCmd)

	// Bind flags to Viper for environment variable support.
	if err := rebuildParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	devcontainerCmd.AddCommand(rebuildCmd)
}
