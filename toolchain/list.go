package toolchain

import (
	"fmt"

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
func RunListInstalledAtmosVersions() error {
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

	// Build rows for installed Atmos versions.
	rows := buildAtmosVersionRows(installer, versions)
	sortToolRows(rows)

	// Calculate column widths and create table.
	terminalWidth := getTerminalWidth()
	widths := calculateColumnWidths(rows, terminalWidth)

	columns := createTableColumns(widths)
	tableRows := convertRowsToTableFormat(rows)
	t := createAndConfigureTable(columns, tableRows)

	// Print the table with conditional styling.
	_ = ui.Writeln(renderTableWithConditionalStyling(&t, rows))

	return nil
}

// buildAtmosVersionRows creates toolRow entries for installed Atmos versions.
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
