package track

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackUpdateCmd = &cobra.Command{
	Use:   "update [track]",
	Short: "Update locked versions within the update policy",
	Long:  "Advance locked versions to the newest candidates allowed by each entry's effective update policy (strategy caps, cooldown, include/exclude, prerelease rules) and write the lock file. Newer versions held back by policy are reported with the blocking reason. Use `lock` to resolve desired versions as-is.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.update.RunE")()

		group, _ := cmd.Flags().GetString("group")
		only, _ := cmd.Flags().GetStringSlice("only")
		update, err := manager.UpdateTrackWithContext(cmd.Context(), atmosConfig, trackFromArgs(cmd, args), group, only)
		if err != nil {
			return err
		}
		if isStructuredFormat(cmd) {
			return writeFormatted(cmd, update)
		}
		return writeRows(cmd, updateColumns(), updateRows(update), "No entries in track.")
	},
}

// updateColumns defines the default table columns for `version track update`.
// Digests are omitted from the table for readability; use --format=yaml or
// --format=json for full-fidelity output.
func updateColumns() []column.Config {
	return []column.Config{
		{Name: "Track", Value: "{{ .track }}"},
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "From", Value: "{{ .from }}"},
		{Name: "To", Value: "{{ .to }}"},
		{Name: "Updated", Value: "{{ .updated }}"},
		{Name: "Reason", Value: "{{ .reason }}"},
	}
}

// updateRows adapts a TrackUpdate to the renderer's row shape.
func updateRows(u *manager.TrackUpdate) []map[string]any {
	rows := make([]map[string]any, 0, len(u.Results))
	for _, r := range u.Results {
		rows = append(rows, map[string]any{
			"track":   u.Track,
			"name":    r.Name,
			"from":    r.From,
			"to":      r.To,
			"updated": strconv.FormatBool(r.Updated),
			"reason":  r.Reason,
		})
	}
	return rows
}

func init() {
	flags.NewStandardParser(trackTableParserOptions(
		groupFlagOption(),
		flags.WithStringSliceFlag("only", "", nil, "Limit the update to the named entries (repeatable)"),
	)...).RegisterFlags(trackUpdateCmd)
	trackCmd.AddCommand(trackUpdateCmd)
}
