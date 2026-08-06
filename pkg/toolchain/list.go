package toolchain

import (
	"fmt"
	"slices"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	activeIndicatorChar     = "●"       // Green dot for active/default installed version.
	installedIndicatorChar  = "●"       // Gray dot for installed non-default version.
	notAvailablePlaceholder = "N/A"     // Placeholder for unavailable information.
	versionLogKey           = "version" // Log key for version information.
	sizeLogKey              = "size"    // Log key for size information.
)

// Table row data structure.
type toolRow struct {
	alias       string
	registry    string
	binary      string
	version     string
	status      string
	installDate string
	size        string
	isDefault   bool
	isInstalled bool
}

// RunListAtmosVersions lists all installed versions of Atmos (returns version strings).
func RunListAtmosVersions() ([]string, error) {
	defer perf.Track(nil, "toolchain.RunListAtmosVersions")()

	installer := NewInstaller()
	return installer.ListInstalledVersions("cloudposse", "atmos")
}

// RunListInstalledAtmosVersions prints a formatted table of installed Atmos versions.
func RunListInstalledAtmosVersions(currentVersion string) error {
	defer perf.Track(nil, "toolchain.RunListInstalledAtmosVersions")()

	installer := NewInstaller()
	versions, err := installer.ListInstalledVersions("cloudposse", "atmos")
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		message := "# No Atmos Versions Installed\n\n" +
			"No versions of Atmos are currently installed.\n\n" +
			"## To install a version:\n\n" +
			"```shell\n" +
			"atmos version install 1.199.0\n" +
			"```\n\n" +
			"Or see available versions:\n\n" +
			"```shell\n" +
			"atmos version list\n" +
			"```\n"
		ui.MarkdownMessage(message)
		return nil
	}

	// Build and render a simple table for Atmos versions.
	renderAtmosVersionTable(installer, versions, currentVersion)

	return nil
}

// atmosVersionRow holds data for a single Atmos version in the simplified table.
type atmosVersionRow struct {
	version     string
	installDate string
	size        string
	isActive    bool
}

// renderAtmosVersionTable renders a simplified table for installed Atmos versions.
func renderAtmosVersionTable(installer *Installer, versions []string, currentVersion string) {
	defer perf.Track(nil, "toolchain.renderAtmosVersionTable")()

	// Build rows with metadata.
	rows := buildAtmosVersionRowsSimple(installer, versions, currentVersion)

	// Sort by version (newest first).
	sortAtmosVersionRows(rows)

	// Render the table.
	printAtmosVersionTable(rows)
}

// buildAtmosVersionRowsSimple creates simplified row entries for installed Atmos versions.
func buildAtmosVersionRowsSimple(installer *Installer, versions []string, currentVersion string) []atmosVersionRow {
	defer perf.Track(nil, "toolchain.buildAtmosVersionRowsSimple")()

	var rows []atmosVersionRow
	currentVersionNormalized := normalizeVersion(currentVersion)
	foundCurrentVersion := false

	for _, version := range versions {
		binaryPath, err := installer.FindBinaryPath("cloudposse", "atmos", version)
		if err != nil {
			continue // Skip versions we can't find.
		}

		_, installDate, size := getInstallationMetadata(binaryPath, true)

		// Check if this version is the currently running one.
		isActive := normalizeVersion(version) == currentVersionNormalized
		if isActive {
			foundCurrentVersion = true
		}

		rows = append(rows, atmosVersionRow{
			version:     version,
			installDate: installDate,
			size:        size,
			isActive:    isActive,
		})
	}

	// If current version wasn't found in installed versions (e.g., running from source/dev build),
	// add it to the list marked as active.
	if !foundCurrentVersion && currentVersion != "" {
		rows = append(rows, atmosVersionRow{
			version:     currentVersion + " (current)",
			installDate: notAvailablePlaceholder,
			size:        notAvailablePlaceholder,
			isActive:    true,
		})
	}

	return rows
}

// normalizeVersion strips the 'v' prefix for comparison.
func normalizeVersion(v string) string {
	if len(v) > 0 && v[0] == 'v' {
		return v[1:]
	}
	return v
}

// sortAtmosVersionRows sorts rows by semantic version (newest first).
func sortAtmosVersionRows(rows []atmosVersionRow) {
	defer perf.Track(nil, "toolchain.sortAtmosVersionRows")()

	sort.Slice(rows, func(i, j int) bool {
		vi, errI := semver.NewVersion(rows[i].version)
		vj, errJ := semver.NewVersion(rows[j].version)
		if errI != nil || errJ != nil {
			return rows[i].version > rows[j].version
		}
		return vi.GreaterThan(vj)
	})
}

// printAtmosVersionTable prints the Atmos version table to stdout.
func printAtmosVersionTable(rows []atmosVersionRow) {
	defer perf.Track(nil, "toolchain.printAtmosVersionTable")()

	// Define column headers.
	headers := []string{"", "VERSION", "INSTALL DATE", "SIZE"}

	// Calculate column widths.
	widths := []int{2, len(headers[1]), len(headers[2]), len(headers[3])}
	for _, row := range rows {
		if len(row.version) > widths[1] {
			widths[1] = len(row.version)
		}
		if len(row.installDate) > widths[2] {
			widths[2] = len(row.installDate)
		}
		if len(row.size) > widths[3] {
			widths[3] = len(row.size)
		}
	}

	// Add padding.
	for i := range widths {
		widths[i] += 2
	}

	// Print header.
	headerStyle := lipgloss.NewStyle().Bold(true)
	headerLine := fmt.Sprintf("%-*s %-*s %-*s %-*s",
		widths[0], headers[0],
		widths[1], headers[1],
		widths[2], headers[2],
		widths[3], headers[3])
	ui.Writeln(headerStyle.Render(headerLine))

	// Print rows.
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	for _, row := range rows {
		activeIndicator := "  "
		if row.isActive {
			activeIndicator = activeStyle.Render("● ")
		}

		line := fmt.Sprintf("%s%-*s %-*s %-*s",
			activeIndicator,
			widths[1], row.version,
			widths[2], row.installDate,
			widths[3], row.size)
		ui.Writeln(line)
	}
}

// RunList prints a table of tools from .tool-versions, marking installed/default versions.
//
// Deprecated: kept for backward compatibility with existing callers/tests. Prefer
// RunListWithOptions, which supports output format selection and installed/pending filtering.
func RunList() error {
	defer perf.Track(nil, "toolchain.RunList")()

	return RunListWithOptions(defaultListFormat, false, false)
}

// defaultListFormat is the default --format value for `atmos toolchain list`.
const defaultListFormat = "table"

// RunListWithOptions prints tools from .tool-versions in the requested format, optionally
// filtered to only installed or only pending (not-yet-installed) tools.
func RunListWithOptions(format string, installedOnly, pendingOnly bool) error {
	defer perf.Track(nil, "toolchain.RunListWithOptions")()

	if !slices.Contains(supportedListFormats, format) {
		return fmt.Errorf("%w: %q (supported: %v)", errUtils.ErrInvalidFlagValue, format, supportedListFormats)
	}
	if installedOnly && pendingOnly {
		return errUtils.ErrMutuallyExclusiveFlags
	}

	installer := NewInstaller()
	toolVersionsFile := GetToolVersionsFilePath()

	// Load tool versions from file.
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	if err != nil {
		return handleToolVersionsLoadError(err, toolVersionsFile)
	}

	if len(toolVersions.Tools) == 0 {
		if format == "json" {
			// JSON consumers expect valid (possibly empty) JSON, not a human-readable message.
			return printToolRowsJSON(nil)
		}
		ui.Writeln("No tools configured in .tool-versions file")
		return nil
	}

	rows := buildToolRows(toolVersions, installer)
	rows = filterToolRowsByStatus(rows, installedOnly, pendingOnly)
	sortToolRows(rows)

	switch format {
	case "json":
		return printToolRowsJSON(rows)
	case "plain":
		return printToolRowsPlain(rows)
	default:
		printToolRowsTable(rows)
		return nil
	}
}

// filterToolRowsByStatus filters rows down to only installed, or only pending (not installed),
// tools. Reuses the isInstalled field already computed per-row by buildToolRow. When neither
// filter is requested, rows is returned unchanged.
func filterToolRowsByStatus(rows []toolRow, installedOnly, pendingOnly bool) []toolRow {
	if !installedOnly && !pendingOnly {
		return rows
	}

	filtered := make([]toolRow, 0, len(rows))
	for _, row := range rows {
		if installedOnly && !row.isInstalled {
			continue
		}
		if pendingOnly && row.isInstalled {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

// ListToolEntry is one entry in a `atmos toolchain list --format=json` listing.
type ListToolEntry struct {
	Registry    string `json:"registry" yaml:"registry"`
	Alias       string `json:"alias,omitempty" yaml:"alias,omitempty"`
	Binary      string `json:"binary" yaml:"binary"`
	Version     string `json:"version" yaml:"version"`
	Default     bool   `json:"default" yaml:"default"`
	Installed   bool   `json:"installed" yaml:"installed"`
	InstallDate string `json:"install_date,omitempty" yaml:"install_date,omitempty"`
	Size        string `json:"size,omitempty" yaml:"size,omitempty"`
}

// ListToolsOutput is the --format=json output shape for `atmos toolchain list`.
type ListToolsOutput struct {
	Tools []ListToolEntry `json:"tools" yaml:"tools"`
}

// printToolRowsJSON writes structured tool listing data to the data channel (stdout).
func printToolRowsJSON(rows []toolRow) error {
	entries := make([]ListToolEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, ListToolEntry{
			Registry:    row.registry,
			Alias:       row.alias,
			Binary:      row.binary,
			Version:     row.version,
			Default:     row.isDefault,
			Installed:   row.isInstalled,
			InstallDate: row.installDate,
			Size:        row.size,
		})
	}
	return data.WriteJSON(ListToolsOutput{Tools: entries})
}

// printToolRowsPlain writes one "registry@version" line per tool to the data channel (stdout),
// with no styling — intended for shell scripting/pipelines.
func printToolRowsPlain(rows []toolRow) error {
	for _, row := range rows {
		if err := data.Writeln(fmt.Sprintf("%s@%s", row.registry, row.version)); err != nil {
			return err
		}
	}
	return nil
}

// printToolRowsTable renders rows as a formatted table to the UI channel (stderr).
func printToolRowsTable(rows []toolRow) {
	if len(rows) == 0 {
		ui.Writeln("No tools match the given filter")
		return
	}

	// Calculate column widths and create table.
	terminalWidth := getTerminalWidth()
	widths := calculateColumnWidths(rows, terminalWidth)

	// Debug: log the final column widths.
	totalWidth := calculateTotalWidth(widths)
	log.Debug("Final column widths",
		"alias", widths.alias,
		"registry", widths.registry,
		"binary", widths.binary,
		versionLogKey, widths.version,
		"status", widths.status,
		"installDate", widths.installDate,
		sizeLogKey, widths.size,
		"total_width", totalWidth,
		"terminal_width", terminalWidth)

	columns := createTableColumns(widths)
	tableRows := convertRowsToTableFormat(rows)
	t := createAndConfigureTable(columns, tableRows)

	// Print the table with conditional styling.
	ui.Writeln(renderTableWithConditionalStyling(&t, rows))
}

// min returns the smaller of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatFileSize formats file size in human readable format.
func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
