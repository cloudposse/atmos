//nolint:dupl // Cobra command boilerplate - structural similarity with attach.go is intentional.
package devcontainer

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var removeParser *flags.StandardFlagParser

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
	ValidArgsFunction: devcontainerNameCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "devcontainer.remove.RunE")()

		parsed, err := removeParser.Parse(cmd.Context(), args)
		if err != nil {
			return err
		}
		opts := parseRemoveOptions(parsed)

		name := parsed.PositionalArgs[0]
		mgr := devcontainer.NewManager()
		return mgr.Remove(atmosConfigPtr, name, opts.Instance, opts.Force)
	},
}

// parseRemoveOptions parses command flags into RemoveOptions.
//
// ParseRemoveOptions reads parsed flags into a RemoveOptions value.
func parseRemoveOptions(parsed *flags.ParsedConfig) *RemoveOptions {
	return &RemoveOptions{
		Instance: flags.GetString(parsed.Flags, "instance"),
		Force:    flags.GetBool(parsed.Flags, "force"),
	}
}

// init initializes the remove command's flag parser and registers the command with the devcontainer command.
func init() {
	// Create parser with remove-specific flags using functional options.
	var usage string
	removeParser, usage = newDevcontainerParser(
		true,
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithBoolFlag("force", "f", false, "Force remove even if running"),
		flags.WithEnvVars("instance", "ATMOS_DEVCONTAINER_INSTANCE"),
		flags.WithEnvVars("force", "ATMOS_DEVCONTAINER_FORCE"),
	)
	removeCmd.Use = "remove " + usage

	initCommandWithFlags(removeCmd, removeParser)
	devcontainerCmd.AddCommand(removeCmd)
}
