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
	"github.com/cloudposse/atmos/pkg/schema"
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

// trackCmd is the parent for all Atmos Version Tracker verbs. The version
// tracker manages externally versioned dependencies declared in atmos.yaml;
// the top-level `atmos version` command remains about the Atmos CLI itself.
var trackCmd = &cobra.Command{
	Use:     "track",
	Aliases: []string{"tracks"},
	Short:   "Manage tracked external versions (the Atmos Version Tracker)",
	Long:    "The Atmos Version Tracker manages external versions declared in atmos.yaml: named version tracks, a deterministic lock file (versions.lock.yaml), CI status and verification, and rendering locked versions into project files.",
}

// formatParserOptions returns the flag options shared by all track verbs.
func formatParserOptions() []flags.Option {
	return []flags.Option{
		flags.WithStringFlag("format", "", "yaml", "Output format: yaml, json"),
	}
}

// trackParserOptions returns flag options for verbs that select a track.
func trackParserOptions(extra ...flags.Option) []flags.Option {
	opts := append(
		formatParserOptions(),
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

// writeFormatted writes v to the data channel in the requested --format,
// using the same canonically-indented, syntax-highlighted rendering as the
// rest of the CLI (pkg/utils.GetHighlightedYAML/GetHighlightedJSON, which
// self-detect TTY/ForceColor and degrade to plain output otherwise).
func writeFormatted(cmd *cobra.Command, v any) error {
	format, _ := cmd.Flags().GetString("format")
	switch strings.ToLower(format) {
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
		return fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
}
