package toolchain

import (
	"fmt"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	installedIndicator      = "✓"       // Checkmark character for installed status.
	uninstalledIndicator    = "✗"       // X mark character for uninstalled status.
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
		_ = ui.MarkdownMessage(message)
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
	_ = ui.Writeln(headerStyle.Render(headerLine))

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
		_ = ui.Writeln(line)
	}
}

// buildAtmosVersionRows creates toolRow entries for installed Atmos versions.
// Deprecated: Use buildAtmosVersionRowsSimple instead for atmos version list --installed.
func buildAtmosVersionRows(installer *Installer, versions []string) []toolRow {
	var rows []toolRow

	for _, version := range versions {
		// Check installation status and get binary path.
		binaryPath, err := installer.FindBinaryPath("cloudposse", "atmos", version)
		isInstalled := err == nil

		// Get installation metadata.
		status, installDate, size := getInstallationMetadata(binaryPath, isInstalled)

		rows = append(rows, toolRow{
			alias:       "",
			registry:    "cloudposse/atmos",
			binary:      "atmos",
			version:     version,
			status:      status,
			installDate: installDate,
			size:        size,
			isDefault:   false,
			isInstalled: isInstalled,
		})
	}

	return rows
}

// RunList prints a table of tools from .tool-versions, marking installed/default versions.
func RunList() error {
	defer perf.Track(nil, "toolchain.RunList")()

	installer := NewInstaller()
	toolVersionsFile := GetToolVersionsFilePath()

	// Load tool versions from file.
	toolVersions, err := LoadToolVersions(toolVersionsFile)
	if err != nil {
		return handleToolVersionsLoadError(err, toolVersionsFile)
	}

	if len(toolVersions.Tools) == 0 {
		_ = ui.Writeln("No tools configured in .tool-versions file")
		return nil
	}

	rows := buildToolRows(toolVersions, installer)
	sortToolRows(rows)

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
	_ = ui.Writeln(renderTableWithConditionalStyling(&t, rows))

	return nil
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
