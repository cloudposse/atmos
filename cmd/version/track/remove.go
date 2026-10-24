package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackRemoveCmd = &cobra.Command{
	Use:     "remove NAME",
	Aliases: []string{"rm"},
	Short:   "Remove a dependency entry from atmos.yaml",
	Long:    "Remove a dependency entry from a version track in atmos.yaml, preserving comments and formatting.",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.remove.RunE")()

		track := manager.EffectiveTrack(atmosConfig, trackFromArgs(cmd, nil))
		file, err := manager.RemoveEntry(atmosConfig, track, args[0])
		if err != nil {
			return err
		}
		return writeFormatted(cmd, crudResult{Name: args[0], Track: track, File: file})
	},
}

func init() {
	flags.NewStandardParser(trackParserOptions()...).RegisterFlags(trackRemoveCmd)
	trackCmd.AddCommand(trackRemoveCmd)
}
