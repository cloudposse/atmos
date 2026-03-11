package toolchain

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	"golang.org/x/term"
)

// columnWidths holds the calculated widths for each table column.
type columnWidths struct {
	alias       int
	registry    int
	binary      int
	version     int
	status      int
	installDate int
	size        int
}

// getTerminalWidth returns the current terminal width, or fallback if unavailable.
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width == 0 {
		return fallbackTerminalWidth
	}
	return width
}

// calculateColumnWidths computes optimal column widths based on content and terminal size.
func calculateColumnWidths(rows []toolRow, terminalWidth int) columnWidths {
	widths := calculateContentWidths(rows)
	widths = ensureMinimumHeaderWidths(widths)
	widths = addColumnBuffers(widths)

	totalNeededWidth := calculateTotalWidth(widths)

	// Debug: log the width calculation.
	log.Debug("Width calculation",
		"alias", widths.alias,
		"registry", widths.registry,
		"binary", widths.binary,
		versionLogKey, widths.version,
		"status", widths.status,
		"installDate", widths.installDate,
		sizeLogKey, widths.size,
		"total", totalNeededWidth,
		"terminal", terminalWidth)

	// If screen is narrow, truncate columns proportionally.
	if totalNeededWidth > terminalWidth {
		widths = truncateColumnsProportionally(widths, totalNeededWidth-terminalWidth)
	}

	return widths
}

// calculateContentWidths finds the maximum width needed for each column based on row content.
func calculateContentWidths(rows []toolRow) columnWidths {
	widths := columnWidths{
		status: 2, // Space for " ●" indicator (space + dot = 2 visual chars).
	}

	for _, row := range rows {
		if len(row.alias) > widths.alias {
			widths.alias = len(row.alias)
		}
		if len(row.registry) > widths.registry {
			widths.registry = len(row.registry)
		}
		if len(row.binary) > widths.binary {
			widths.binary = len(row.binary)
		}
		if len(row.version) > widths.version {
			widths.version = len(row.version)
		}
		if len(row.installDate) > widths.installDate {
			widths.installDate = len(row.installDate)
		}
		if len(row.size) > widths.size {
			widths.size = len(row.size)
		}

		// Debug: log the size width calculation.
		log.Debug("Size width calculation",
			"row_size", row.size,
			"row_size_len", len(row.size),
			"current_size_width", widths.size)
	}

	return widths
}

// ensureMinimumHeaderWidths ensures columns are never narrower than their headers.
func ensureMinimumHeaderWidths(widths columnWidths) columnWidths {
	const (
		headerAliasWidth       = len("ALIAS")
		headerRegistryWidth    = len("REGISTRY")
		headerBinaryWidth      = len("BINARY")
		headerVersionWidth     = len("VERSION")
		headerStatusWidth      = 2 // Space for " ●" indicator (space + dot = 2 visual chars).
		headerInstallDateWidth = len("INSTALL DATE")
		headerSizeWidth        = len("SIZE")
	)

	if widths.alias < headerAliasWidth {
		widths.alias = headerAliasWidth
	}
	if widths.registry < headerRegistryWidth {
		widths.registry = headerRegistryWidth
	}
	if widths.binary < headerBinaryWidth {
		widths.binary = headerBinaryWidth
	}
	if widths.version < headerVersionWidth {
		widths.version = headerVersionWidth
	}
	if widths.status < headerStatusWidth {
		widths.status = headerStatusWidth
	}
	if widths.installDate < headerInstallDateWidth {
		widths.installDate = headerInstallDateWidth
	}
	if widths.size < headerSizeWidth {
		widths.size = headerSizeWidth
	}

	return widths
}

// addColumnBuffers adds padding (2 spaces on each side) to column widths.
func addColumnBuffers(widths columnWidths) columnWidths {
	const bufferSize = 4

	widths.status += 1 // Small buffer for status column.
	widths.alias += bufferSize
	widths.registry += bufferSize
	widths.binary += bufferSize
	widths.version += bufferSize
	widths.installDate += bufferSize
	widths.size += bufferSize

	return widths
}

// calculateTotalWidth sums all column widths.
func calculateTotalWidth(widths columnWidths) int {
	return widths.alias + widths.registry + widths.binary + widths.version + widths.status + widths.installDate + widths.size
}

// truncateColumnsProportionally reduces column widths to fit within terminal width.
func truncateColumnsProportionally(widths columnWidths, excess int) columnWidths {
	const (
		minAliasWidth       = 6
		minRegistryWidth    = 8
		minBinaryWidth      = 6
		minVersionWidth     = 8
		minInstallDateWidth = 12
		minSizeWidth        = 8
	)

	// Truncate proportionally, but keep minimums.
	registryReduce := min(excess*3/10, widths.registry-minRegistryWidth)
	aliasReduce := min(excess*2/10, widths.alias-minAliasWidth)
	binaryReduce := min(excess*1/10, widths.binary-minBinaryWidth)
	versionReduce := min(excess*2/10, widths.version-minVersionWidth)
	installDateReduce := min(excess*1/10, widths.installDate-minInstallDateWidth)
	sizeReduce := min(excess*1/10, widths.size-minSizeWidth)

	widths.registry -= registryReduce
	widths.alias -= aliasReduce
	widths.binary -= binaryReduce
	widths.version -= versionReduce
	widths.installDate -= installDateReduce
	widths.size -= sizeReduce

	return widths
}

// createTableColumns creates Bubble Tea table columns from calculated widths.
func createTableColumns(widths columnWidths) []table.Column {
	return []table.Column{
		{Title: "  ", Width: widths.status}, // Status indicator column (dots), 2-char header to match data.
		{Title: "ALIAS", Width: widths.alias},
		{Title: "REGISTRY", Width: widths.registry},
		{Title: "BINARY", Width: widths.binary},
		{Title: "VERSION", Width: widths.version},
		{Title: "INSTALL DATE", Width: widths.installDate},
		{Title: "SIZE", Width: widths.size},
	}
}

// convertRowsToTableFormat converts toolRow structs to Bubble Tea table.Row format.
// Note: Status indicators are NOT pre-styled here because ANSI escape codes
// would confuse the table's width calculation. Styling is applied in
// renderTableWithConditionalStyling after the table is rendered.
func convertRowsToTableFormat(rows []toolRow) []table.Row {
	var tableRows []table.Row

	for i, row := range rows {
		// Debug: log the row data.
		log.Debug("Creating table row",
			"index", i,
			"alias", row.alias,
			"registry", row.registry,
			"binary", row.binary,
			versionLogKey, row.version,
			"status", row.status,
			"installDate", row.installDate,
			sizeLogKey, row.size,
			"size_len", len(row.size))

		// Check if size is empty or nil.
		if row.size == "" {
			log.Debug("WARNING: Empty size for row", "index", i, "tool", row.alias)
		}

		// Pass unstyled status indicator - styling applied in renderTableWithConditionalStyling.
		tableRows = append(tableRows, table.Row{
			row.status, // Status indicator first (unstyled).
			row.alias,
			row.registry,
			row.binary,
			row.version,
			row.installDate,
			row.size,
		})
	}

	return tableRows
}

// createAndConfigureTable creates a Bubble Tea table with the given columns and rows.
func createAndConfigureTable(columns []table.Column, tableRows []table.Row) table.Model {
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(tableRows),
		table.WithFocused(false), // Non-interactive.
		table.WithHeight(len(tableRows)+1),
	)

	// Debug: log table configuration.
	log.Debug("Table configuration",
		"num_columns", len(columns),
		"num_rows", len(tableRows),
		"height", len(tableRows))

	// Create custom styles for different states.
	defaultStyle := table.DefaultStyles()
	defaultStyle.Header = defaultStyle.Header.Bold(true).Align(lipgloss.Left).PaddingLeft(0).PaddingRight(1)
	defaultStyle.Cell = defaultStyle.Cell.PaddingLeft(0).PaddingRight(1)
	defaultStyle.Selected = defaultStyle.Cell

	// Set the default styles.
	t.SetStyles(defaultStyle)

	return t
}

// tableStyles holds the styles used for table rendering.
type tableStyles struct {
	active      lipgloss.Style
	installed   lipgloss.Style
	uninstalled lipgloss.Style
}

// newTableStyles creates the default styles for table rendering.
func newTableStyles() tableStyles {
	return tableStyles{
		active:      lipgloss.NewStyle().Foreground(lipgloss.Color("42")),  // Green for active/default.
		installed:   lipgloss.NewStyle().Foreground(lipgloss.Color("8")),   // Gray for installed non-default.
		uninstalled: lipgloss.NewStyle().Foreground(lipgloss.Color("240")), // Gray for uninstalled rows.
	}
}

// styleRowLine applies styling to a single row line based on the row data.
func styleRowLine(line string, rowData *toolRow, styles *tableStyles) string {
	if !rowData.isInstalled {
		return styles.uninstalled.Render(line)
	}
	return styleInstalledRowLine(line, rowData, styles)
}

// styleInstalledRowLine applies status indicator styling to an installed row.
func styleInstalledRowLine(line string, rowData *toolRow, styles *tableStyles) string {
	paddedIndicator := " " + activeIndicatorChar
	if !strings.Contains(line, paddedIndicator) {
		return line
	}

	var styledIndicator string
	if rowData.isDefault {
		styledIndicator = " " + styles.active.Render(activeIndicatorChar)
	} else {
		styledIndicator = " " + styles.installed.Render(installedIndicatorChar)
	}
	return strings.Replace(line, paddedIndicator, styledIndicator, 1)
}

// renderTableWithConditionalStyling renders the table with proper conditional styling.
// This applies both status indicator coloring and row-level styling based on install state.
func renderTableWithConditionalStyling(t *table.Model, rows []toolRow) string {
	tableView := t.View()
	styles := newTableStyles()
	lines := strings.Split(tableView, "\n")

	for i, line := range lines {
		// Skip header line and empty lines.
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		// Apply styling based on row data (adjust index for header).
		rowIndex := i - 1
		if rowIndex < len(rows) {
			lines[i] = styleRowLine(line, &rows[rowIndex], &styles)
		}
	}

	return strings.Join(lines, "\n")
}
