package devcontainer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var removeParser *flags.StandardParser

// RemoveOptions contains parsed flags for the remove command.
type RemoveOptions struct {
	Instance string
	Force    bool
}

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a devcontainer",
	Long: `Remove a devcontainer and all its data.

This will stop the container if it's running and remove it completely.
Use --force to remove a running container without stopping it first.`,
	Example:           markdown.DevcontainerRemoveUsageMarkdown,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.remove.RunE")()

		// Parse flags using new options pattern.
		v := viper.GetViper()
		if err := removeParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseRemoveOptions(cmd, v, args)
		if err != nil {
			return err
		}

		name := args[0]
		return e.ExecuteDevcontainerRemove(atmosConfigPtr, name, opts.Instance, opts.Force)
	},
}

// parseRemoveOptions parses command flags into RemoveOptions.
//
//nolint:unparam // args parameter kept for consistency with other parse functions
func parseRemoveOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*RemoveOptions, error) {
	return &RemoveOptions{
		Instance: v.GetString("instance"),
		Force:    v.GetBool("force"),
	}, nil
}

func init() {
	// Create parser with remove-specific flags using functional options.
	removeParser = flags.NewStandardParser(
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("force", "f", false, "Force remove even if running"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("force", "ATMOS_DEVCONTAINER_FORCE"),
	)

	// Register flags using the standard RegisterFlags method.
	removeParser.RegisterFlags(removeCmd)

	// Bind flags to Viper for environment variable support.
	if err := removeParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	devcontainerCmd.AddCommand(removeCmd)
}
