package track

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackGetCmd = &cobra.Command{
	Use:   "get NAME",
	Short: "Show a dependency entry with its effective policy and lock state",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.get.RunE")()

		track := manager.EffectiveTrack(atmosConfig, trackFromArgs(cmd, nil))
		entries, err := manager.EffectiveEntries(atmosConfig, track)
		if err != nil {
			return err
		}
		entry, ok := entries[args[0]]
		if !ok {
			return fmt.Errorf("%w: %s in track %s", manager.ErrEntryNotFound, args[0], track)
		}
		if locked, err := manager.ResolveLocked(atmosConfig, track, args[0]); err == nil {
			entry.Locked = locked
		}

		show, _ := cmd.Flags().GetString("show")
		value, err := entry.Field(show)
		if err != nil {
			return err
		}
		return writeFormatted(cmd, value)
	},
}

// trackGetParserOptions returns the flag options for `version track get`,
// extending the shared track verbs with --show since get is the only verb
// that selects a single field from a single entry.
func trackGetParserOptions() []flags.Option {
	return trackParserOptions(
		flags.WithStringFlag("show", "", "locked",
			"Entry field to display: name, ecosystem, datasource, provider, package, desired, group, update, include, exclude, prerelease, labels, locked"),
	)
}

func init() {
	flags.NewStandardParser(trackGetParserOptions()...).RegisterFlags(trackGetCmd)
	trackCmd.AddCommand(trackGetCmd)
}
