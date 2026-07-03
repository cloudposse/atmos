package version

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured version tracks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.track.list.RunE")()

		return writeFormatted(cmd, manager.TrackNames(atmosConfigPtr))
	},
}

func init() {
	flags.NewStandardParser(formatParserOptions()...).RegisterFlags(trackListCmd)
	trackCmd.AddCommand(trackListCmd)
}
