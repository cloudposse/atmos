package vendor

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

var vendorVerifyParser *flags.StandardParser

// errVendorLockDrift is a command-local sentinel (matching vendor clean's errModifiedVendorFiles
// convention): CI-facing exit-code signaling for `atmos vendor verify`, not a cross-package
// contract other code matches against.
var errVendorLockDrift = errors.New("vendor lock drift detected")

// verifyRow is a Drift enriched with the human-readable component name lockfile.Verify's own
// Drift.Artifact doesn't carry (it's the opaque lock key/hash the artifact is stored under, not
// Artifact.Name) — resolved here from the loaded lock so both --component filtering and rendered
// output are readable.
type verifyRow struct {
	Component string `json:"component"`
	Path      string `json:"path"`
	Reason    string `json:"reason"`
}

// vendorVerifyCmd compares every lock-owned file on disk against its vendor.lock.yaml receipt and
// reports drift (missing or modified files), exiting non-zero when any is found — CI-friendly, and
// the read-only counterpart to `vendor update --check` (which checks for available *version*
// updates upstream, not on-disk drift from what's already locked).
var vendorVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify vendored files match vendor.lock.yaml",
	Long: `Compare every lock-owned file on disk against its recorded vendor.lock.yaml receipt and
report drift: missing files, or files whose contents no longer match what was last vendored.
Exits non-zero when any drift is found. This never checks for a newer upstream version — see
'atmos vendor update --check' for that.`,
	Example: "atmos vendor verify\natmos vendor verify --component vpc\natmos vendor verify --format json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.verifyRunE")()

		v := viper.GetViper()
		if err := vendorVerifyParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		info, err := e.ProcessCommandLineArgs("terraform", cmd, args, nil)
		if err != nil {
			return err
		}
		atmosConfig, err := cfg.InitCliConfig(info, false)
		if err != nil {
			return err
		}

		lock, err := lockfile.Load(&atmosConfig)
		if err != nil {
			return err
		}

		drifts, err := lockfile.Verify(&atmosConfig, lock)
		if err != nil {
			return err
		}

		rows := buildVerifyRows(lock, drifts, atmosConfig.BasePath, v.GetString("component"))

		if err := renderVerifyResult(rows, v.GetString("format")); err != nil {
			return err
		}

		if len(rows) > 0 {
			return fmt.Errorf("%w: %d artifact(s)", errVendorLockDrift, len(rows))
		}
		return nil
	},
}

// buildVerifyRows resolves each drift's lock key back to its artifact's human-readable Name,
// optionally filtering to a single component, and returns rows sorted for deterministic output
// (Verify's own drifts are produced from map iteration, so their order isn't stable run to run).
func buildVerifyRows(lock *lockfile.LockFile, drifts []lockfile.Drift, basePath, component string) []verifyRow {
	rows := make([]verifyRow, 0, len(drifts))
	for _, drift := range drifts {
		name := drift.Artifact
		if artifact, ok := lock.Artifacts[drift.Artifact]; ok && artifact.Name != "" {
			name = artifact.Name
		}
		if component != "" && name != component {
			continue
		}
		rows = append(rows, verifyRow{Component: name, Path: displayPath(basePath, drift.Path), Reason: drift.Reason})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Component != rows[j].Component {
			return rows[i].Component < rows[j].Component
		}
		return rows[i].Path < rows[j].Path
	})
	return rows
}

// displayPath renders an absolute lock-owned path relative to basePath for display, falling back
// to the absolute path when it can't be made relative (e.g. basePath unset).
func displayPath(basePath, path string) string {
	if basePath == "" {
		return path
	}
	rel, err := filepath.Rel(basePath, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func renderVerifyResult(rows []verifyRow, format string) error {
	if format == "json" {
		return data.WriteJSON(rows)
	}
	if len(rows) == 0 {
		ui.Success("No drift detected: every lock-owned file matches vendor.lock.yaml.")
		return nil
	}
	ui.Write(createVerifyTable(rows))
	ui.Errorf("Drift detected in %d artifact(s).", len(rows))
	return nil
}

func createVerifyTable(rows []verifyRow) string {
	headers := []string{"COMPONENT", "PATH", "REASON"}
	cells := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells = append(cells, []string{row.Component, row.Path, row.Reason})
	}
	t := table.New().
		Headers(headers...).
		Rows(cells...).
		BorderHeader(true).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderRow(false).
		BorderColumn(false).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(updateReportBorderColor))).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return updateReportHeaderStyle.Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})

	lineEnding := u.GetLineEnding()
	return lineEnding + t.String() + lineEnding + lineEnding
}

func init() {
	vendorVerifyParser = flags.NewStandardParser(
		flags.WithStringFlag("component", "c", "", "Verify only this component"),
		flags.WithStringFlag("format", "", "table", "Output format: table or json"),
	)
	vendorVerifyParser.RegisterFlags(vendorVerifyCmd)
	if err := vendorVerifyParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorCmd.AddCommand(vendorVerifyCmd)
}
