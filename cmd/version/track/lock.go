package track

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackLockCmd = &cobra.Command{
	Use:   "lock [track]",
	Short: "Resolve desired versions and write the lock file",
	Long:  "Resolve each entry's desired version to a concrete version and write versions.lock.yaml. Lock resolves the desired versions as-is; use `update` to advance versions within the update policy.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.lock.RunE")()

		group, _ := cmd.Flags().GetString("group")
		track := trackFromArgs(cmd, args)
		lock, err := manager.LockTrack(atmosConfig, track, group)
		if err != nil {
			return err
		}
		if isStructuredFormat(cmd) {
			return writeFormatted(cmd, lock)
		}
		effectiveTrack := manager.EffectiveTrack(atmosConfig, track)
		rows := lockRows(effectiveTrack, lock.Tracks[effectiveTrack])
		return writeRows(cmd, lockColumns(), rows, "No entries in track.")
	},
}

// lockColumns defines the default table columns for `version track lock`.
// The table is scoped to the resolved track's entries; the full multi-track
// lock file remains available via --format=yaml or --format=json.
func lockColumns() []column.Config {
	return []column.Config{
		{Name: "Track", Value: "{{ .track }}"},
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "Version", Value: "{{ .version }}"},
		{Name: "Ecosystem", Value: "{{ .ecosystem }}"},
		{Name: "Digest", Value: "{{ .digest }}"},
		{Name: "Resolved At", Value: "{{ .resolved_at }}"},
	}
}

// lockRows adapts one track's lock entries to the renderer's row shape,
// sorted by name for stable output (map iteration order is not stable).
func lockRows(track string, entries map[string]manager.LockEntry) []map[string]any {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([]map[string]any, 0, len(names))
	for _, name := range names {
		e := entries[name]
		rows = append(rows, map[string]any{
			"track":       track,
			"name":        name,
			"version":     e.Version,
			"ecosystem":   e.Ecosystem,
			"digest":      e.Digest,
			"resolved_at": e.ResolvedAt,
		})
	}
	return rows
}

func init() {
	flags.NewStandardParser(trackTableParserOptions(groupFlagOption())...).RegisterFlags(trackLockCmd)
	trackCmd.AddCommand(trackLockCmd)
}
