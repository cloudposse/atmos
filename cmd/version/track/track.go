// Package track implements the `atmos version track` command group: the Atmos
// Version Tracker's verbs for locking, updating, inspecting, and applying
// managed external versions.
package track

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// atmosConfig is set by SetAtmosConfig before command execution.
var atmosConfig *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the track command group.
// The parent version command forwards it from cmd/root.go initialization.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfig = config
}

// GetTrackCommand returns the `atmos version track` command group for the
// parent version command to register.
func GetTrackCommand() *cobra.Command {
	return trackCmd
}

var (
	// ErrRenderFileRequired is returned when render is invoked without --file.
	ErrRenderFileRequired = errUtils.ErrVersionRenderFileRequired
	// ErrRenderDrift is returned when rendered output differs from the committed file.
	ErrRenderDrift = errUtils.ErrVersionRenderDrift
	// ErrUnsupportedFormat is returned for unknown --format values.
	ErrUnsupportedFormat = errUtils.ErrUnsupportedVersionTrackFormat
)

// formatFlagName is the shared --format flag name used by all track verbs.
const formatFlagName = "format"

// trackCmd is the parent for all Atmos Version Tracker verbs. The version
// tracker manages externally versioned dependencies declared in atmos.yaml;
// the top-level `atmos version` command remains about the Atmos CLI itself.
var trackCmd = &cobra.Command{
	Use:     "track",
	Aliases: []string{"tracks"},
	Short:   "Manage tracked external versions (the Atmos Version Tracker)",
	Long:    "The Atmos Version Tracker manages external versions declared in atmos.yaml: named version tracks, a deterministic lock file (versions.lock.yaml), CI status and verification, and rendering locked versions into project files.",
}

// structuredFormatParserOptions returns the --format flag for verbs that only
// support full-fidelity yaml/json output via writeFormatted (show/get/add/
// set/remove/apply/verify). The format flag defaults to empty, which
// resolves to yaml.
func structuredFormatParserOptions() []flags.Option {
	return []flags.Option{
		flags.WithStringFlag(formatFlagName, "", "", "Output format: json, yaml"),
	}
}

// tableFormatParserOptions returns the --format flag for verbs with a
// curated table view (update/lock/status/diff). The format flag defaults to
// empty, which resolves to a human-readable, TTY-aware table (matching
// `version track list`); yaml/json remain available as an explicit opt-in
// for full-fidelity, machine-readable output.
func tableFormatParserOptions() []flags.Option {
	return []flags.Option{
		flags.WithStringFlag(formatFlagName, "", "", "Output format: table, json, yaml, csv, tsv"),
	}
}

// trackParserOptions returns flag options for verbs that select a track and
// only support the full-fidelity yaml/json output (writeFormatted).
func trackParserOptions(extra ...flags.Option) []flags.Option {
	opts := append(
		structuredFormatParserOptions(),
		flags.WithStringFlag("track", "", "", "Version track to operate on (defaults to version.track in atmos.yaml)"),
	)
	return append(opts, extra...)
}

// trackTableParserOptions returns flag options for verbs with a curated
// table view (update/lock/status/diff), which support table/csv/tsv in
// addition to yaml/json.
func trackTableParserOptions(extra ...flags.Option) []flags.Option {
	opts := append(
		tableFormatParserOptions(),
		flags.WithStringFlag("track", "", "", "Version track to operate on (defaults to version.track in atmos.yaml)"),
	)
	return append(opts, extra...)
}

// groupFlagOption returns the --group flag shared by lock/update/status/diff.
func groupFlagOption() flags.Option {
	return flags.WithStringFlag("group", "", "", "Limit the command to a version group")
}

// trackFromArgs resolves the target track from the optional positional
// argument or the --track flag. An empty result means the configured default
// track (version.track) applies.
func trackFromArgs(cmd *cobra.Command, args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	track, _ := cmd.Flags().GetString("track")
	return track
}

// writeFormatted writes v, the full-fidelity result struct, to the data
// channel in the requested --format=yaml or --format=json (empty defaults to
// yaml), using the same canonically-indented, syntax-highlighted rendering as
// the rest of the CLI (pkg/utils.GetHighlightedYAML/GetHighlightedJSON, which
// self-detect TTY/ForceColor and degrade to plain output otherwise). Verbs
// with a curated table view (update/lock/status/diff) check
// isStructuredFormat first and use writeRows instead for the table default;
// verbs without one (show/get/add/set/remove/apply/verify) call this
// directly.
func writeFormatted(cmd *cobra.Command, v any) error {
	formatStr, _ := cmd.Flags().GetString(formatFlagName)
	switch strings.ToLower(formatStr) {
	case "", "yaml":
		if atmosConfig == nil {
			return data.WriteYAML(v)
		}
		y, err := u.GetHighlightedYAML(atmosConfig, v)
		if err != nil {
			return err
		}
		return data.Write(y)
	case "json":
		if atmosConfig == nil {
			return data.WriteJSON(v)
		}
		j, err := u.GetHighlightedJSON(atmosConfig, v)
		if err != nil {
			return err
		}
		return data.Write(j + "\n")
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedFormat, formatStr)
	}
}

// isStructuredFormat reports whether --format requests full-fidelity
// yaml/json output (via writeFormatted) rather than the curated table/csv/tsv
// row view (via writeRows). The default (empty) format resolves to table.
func isStructuredFormat(cmd *cobra.Command) bool {
	formatStr, _ := cmd.Flags().GetString(formatFlagName)
	switch strings.ToLower(formatStr) {
	case "yaml", "json":
		return true
	default:
		return false
	}
}

// writeRows renders rows as a human-readable table by default (TTY-aware),
// or as csv/tsv on request, via the same pkg/list/renderer pipeline used by
// `version track list` and `secret list`. An empty rows slice prints
// emptyMessage instead of an empty table.
func writeRows(cmd *cobra.Command, columns []column.Config, rows []map[string]any, emptyMessage string) error {
	if len(rows) == 0 {
		ui.Info(emptyMessage)
		return nil
	}

	formatStr, _ := cmd.Flags().GetString(formatFlagName)
	formatStr = strings.ToLower(formatStr)
	switch formatStr {
	case "", string(format.FormatTable), string(format.FormatCSV), string(format.FormatTSV):
		// Valid: table (default) or a delimited row export.
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedFormat, formatStr)
	}

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	r := renderer.New(nil, selector, nil, format.Format(formatStr), "")
	return r.Render(rows)
}
