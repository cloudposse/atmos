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
		return writeFormatted(cmd, entry)
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions()...).RegisterFlags(trackGetCmd)
	trackCmd.AddCommand(trackGetCmd)
}
