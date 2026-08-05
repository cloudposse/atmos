package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackShowCmd = &cobra.Command{
	Use:   "show [track]",
	Short: "Show the effective entries of a version track",
	Long:  "Show a version track's entries after global defaults, track defaults, and group policies are applied.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.show.RunE")()

		entries, err := manager.EffectiveEntries(atmosConfig, trackFromArgs(cmd, args))
		if err != nil {
			return err
		}
		return writeFormatted(cmd, entries)
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions()...).RegisterFlags(trackShowCmd)
	trackCmd.AddCommand(trackShowCmd)
}
