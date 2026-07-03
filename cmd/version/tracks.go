package version

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

// renderOutputPerm is the permission for rendered output files. Rendered files
// are ordinary project files (e.g. GitHub Actions workflows), not secrets.
const renderOutputPerm os.FileMode = 0o644

var (
	// ErrRenderFileRequired is returned when render is invoked without --file.
	ErrRenderFileRequired = errors.New("--file is required")
	// ErrRenderDrift is returned when rendered output differs from the committed file.
	ErrRenderDrift = errors.New("rendered output differs from committed file")
	// ErrUnsupportedFormat is returned for unknown --format values.
	ErrUnsupportedFormat = errors.New("unsupported output format (supported: yaml, json, table)")
)

var (
	tracksFormat string
	tracksGroup  string
	renderFile   string
	renderOutput string
	renderCheck  bool
)

var tracksCmd = &cobra.Command{
	Use:   "tracks",
	Short: "Manage Atmos version tracks",
	Long:  "Manage Atmos-native external version tracks, locks, status, and render workflows.",
}

var tracksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured version tracks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.tracks.list.RunE")()

		names := manager.TrackNames(atmosConfigPtr)
		return printFormatted(tracksFormat, names)
	},
}

var tracksShowCmd = &cobra.Command{
	Use:   "show [track]",
	Short: "Show a version track",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.tracks.show.RunE")()

		entries, err := manager.EffectiveEntries(atmosConfigPtr, args[0])
		if err != nil {
			return err
		}
		return printFormatted(tracksFormat, entries)
	},
}

var tracksLockCmd = &cobra.Command{
	Use:   "lock [track]",
	Short: "Resolve and lock a version track",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.tracks.lock.RunE")()

		lock, err := manager.LockTrack(atmosConfigPtr, args[0], tracksGroup)
		if err != nil {
			return err
		}
		return printFormatted(tracksFormat, lock)
	},
}

var tracksUpdateCmd = &cobra.Command{
	Use:   "update [track]",
	Short: "Update locked versions for a version track",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.tracks.update.RunE")()

		lock, err := manager.LockTrack(atmosConfigPtr, args[0], tracksGroup)
		if err != nil {
			return err
		}
		return printFormatted(tracksFormat, lock)
	},
}

var tracksStatusCmd = &cobra.Command{
	Use:   "status [track]",
	Short: "Show status for a version track",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.tracks.status.RunE")()

		status, err := manager.StatusTrack(atmosConfigPtr, args[0], tracksGroup)
		if err != nil {
			return err
		}
		return printFormatted(tracksFormat, status)
	},
}

var tracksDiffCmd = &cobra.Command{
	Use:   "diff [track]",
	Short: "Show locked versions that differ from the resolved target",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.tracks.diff.RunE")()

		status, err := manager.StatusTrack(atmosConfigPtr, args[0], tracksGroup)
		if err != nil {
			return err
		}
		var changed []manager.StatusEntry
		for i := range status.Entries {
			if status.Entries[i].Status == manager.StatusUpdateAvailable || status.Entries[i].Status == manager.StatusUnlocked {
				changed = append(changed, status.Entries[i])
			}
		}
		return printFormatted(tracksFormat, changed)
	},
}

var tracksVerifyCmd = &cobra.Command{
	Use:   "verify [track]",
	Short: "Verify that a version track is locked and current",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.tracks.verify.RunE")()

		status, err := manager.VerifyTrack(atmosConfigPtr, args[0])
		if err != nil {
			return err
		}
		return printFormatted(tracksFormat, status)
	},
}

var tracksRenderCmd = &cobra.Command{
	Use:   "render [track]",
	Short: "Render a file with .version template values",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.tracks.render.RunE")()

		if renderFile == "" {
			return ErrRenderFileRequired
		}
		content, err := os.ReadFile(renderFile)
		if err != nil {
			return err
		}
		versionMap, err := manager.VersionMap(atmosConfigPtr, args[0])
		if err != nil {
			return err
		}
		rendered, err := exec.ProcessTmpl(atmosConfigPtr, renderFile, string(content), map[string]any{"version": versionMap}, false)
		if err != nil {
			return err
		}
		if renderCheck {
			checkPath := renderFile
			if renderOutput != "" {
				checkPath = renderOutput
			}
			current, err := os.ReadFile(checkPath)
			if err != nil {
				return err
			}
			if string(current) != rendered {
				return fmt.Errorf("%w: %s", ErrRenderDrift, checkPath)
			}
			return nil
		}
		if renderOutput != "" {
			return os.WriteFile(renderOutput, []byte(rendered), renderOutputPerm) // #nosec G306 -- rendered output is a non-sensitive project file.
		}
		return data.Write(rendered)
	},
}

func printFormatted(format string, data any) error {
	switch strings.ToLower(format) {
	case "", "yaml":
		return utils.PrintAsYAMLSimple(atmosConfigPtr, data)
	case "json":
		return utils.PrintAsJSONSimple(atmosConfigPtr, data)
	case "table":
		return utils.PrintAsYAMLSimple(atmosConfigPtr, data)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
}

func init() {
	tracksCmd.PersistentFlags().StringVar(&tracksFormat, "format", "yaml", "Output format: yaml, json, table")
	tracksCmd.PersistentFlags().StringVar(&tracksGroup, "group", "", "Limit the command to a version group")
	tracksRenderCmd.Flags().StringVar(&renderFile, "file", "", "Template source file to render")
	tracksRenderCmd.Flags().StringVar(&renderOutput, "output", "", "Rendered output file")
	tracksRenderCmd.Flags().BoolVar(&renderCheck, "check", false, "Check that rendered output matches the output file")

	tracksCmd.AddCommand(
		tracksListCmd,
		tracksShowCmd,
		tracksLockCmd,
		tracksUpdateCmd,
		tracksStatusCmd,
		tracksDiffCmd,
		tracksVerifyCmd,
		tracksRenderCmd,
	)
	versionCmd.AddCommand(tracksCmd)
}
