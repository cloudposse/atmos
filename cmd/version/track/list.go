package track

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	f "github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

var trackListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured version tracks as a dependency x track version matrix",
	Long:  "List configured version tracks. Each row is a dependency and each column is a track, so versions can be compared across tracks at a glance.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfig, "version.track.list.RunE")()

		show, _ := cmd.Flags().GetString("show")
		formatStr, _ := cmd.Flags().GetString("format")
		maxColumns, _ := cmd.Flags().GetInt("max-columns")

		matrix, err := manager.TrackVersionMatrix(atmosConfig, show)
		if err != nil {
			return err
		}

		if err := f.ValidateFormat(formatStr); err != nil {
			return err
		}
		formatter, err := f.NewFormatter(f.Format(formatStr))
		if err != nil {
			return err
		}

		output, err := formatter.Format(matrixToValues(matrix), f.FormatOptions{
			Format:        f.Format(formatStr),
			MaxColumns:    maxColumns,
			TTY:           term.IsTTYSupportForStdout(),
			CustomHeaders: append([]string{"Dependency"}, manager.TrackNames(atmosConfig)...),
		})
		if err != nil {
			return err
		}
		return data.Write(output)
	},
}

// matrixToValues adapts the track-name -> dependency-name -> version matrix
// to the map[string]interface{} shape expected by pkg/list/format.Formatter.
func matrixToValues(matrix map[string]map[string]string) map[string]interface{} {
	values := make(map[string]interface{}, len(matrix))
	for track, deps := range matrix {
		row := make(map[string]interface{}, len(deps))
		for name, version := range deps {
			row[name] = version
		}
		values[track] = row
	}
	return values
}

// trackListParserOptions returns the flag options for `version track list`,
// kept separate from formatParserOptions/trackParserOptions in track.go since
// list supports a richer format set (table/json/yaml/csv/tsv) and its own
// --show selector, unlike the other track verbs.
func trackListParserOptions() []flags.Option {
	return []flags.Option{
		flags.WithStringFlag("format", "", "table", "Output format: table, json, yaml, csv, tsv"),
		flags.WithStringFlag("show", "", manager.ShowLocked, "Version value to show per track: desired, locked"),
		flags.WithIntFlag("max-columns", "", 0, "Limit the number of track columns displayed (0 = no limit)"),
	}
}

func init() {
	flags.NewStandardParser(trackListParserOptions()...).RegisterFlags(trackListCmd)
	trackCmd.AddCommand(trackListCmd)
}
