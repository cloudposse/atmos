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
