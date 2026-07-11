package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackStatusCmd = &cobra.Command{
	Use:   "status [track]",
	Short: "Show lock and update status for a version track",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.status.RunE")()

		group, _ := cmd.Flags().GetString("group")
		status, err := manager.StatusTrackWithContext(cmd.Context(), atmosConfig, trackFromArgs(cmd, args), group)
		if err != nil {
			return err
		}
		if isStructuredFormat(cmd) {
			return writeFormatted(cmd, status)
		}
		return writeRows(cmd, statusColumns(), statusRows(status.Track, status.Entries), "No entries in track.")
	},
}

// statusColumns defines the default table columns shared by `version track
// status` and `version track diff`.
func statusColumns() []column.Config {
	return []column.Config{
		{Name: "Track", Value: "{{ .track }}"},
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "Group", Value: "{{ .group }}"},
		{Name: "Desired", Value: "{{ .desired }}"},
		{Name: "Locked", Value: "{{ .locked }}"},
		{Name: "Resolved", Value: "{{ .resolved }}"},
		{Name: "Status", Value: "{{ .status }}"},
		{Name: "Message", Value: "{{ .message }}"},
	}
}

// statusRows adapts a slice of StatusEntry (scoped to track) to the
// renderer's row shape. Shared by `status` and `diff`.
func statusRows(track string, entries []manager.StatusEntry) []map[string]any {
	rows := make([]map[string]any, 0, len(entries))
	for i := range entries {
		e := &entries[i]
		rows = append(rows, map[string]any{
			"track":    track,
			"name":     e.Name,
			"group":    e.Group,
			"desired":  e.Desired,
			"locked":   e.Locked,
			"resolved": e.Resolved,
			"status":   e.Status,
			"message":  e.Message,
		})
	}
	return rows
}

func init() {
	flags.NewStandardParser(trackTableParserOptions(groupFlagOption())...).RegisterFlags(trackStatusCmd)
	trackCmd.AddCommand(trackStatusCmd)
}
