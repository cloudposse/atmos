package toolchain

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// buildVersionRows builds row data and calculates column widths for version table.
func buildVersionRows(versions []string, defaultVersion string, showInstalled bool) ([]versionRow, int, int) {
	var rows []versionRow
	statusWidth := 2 // For checkmark character.
	versionWidth := len("VERSION")

	for _, v := range versions {
		row := versionRow{
			version:     v,
			isDefault:   v == defaultVersion,
			isInstalled: showInstalled, // All versions in installed list are installed.
		}

		// Set status indicator.
		switch {
		case row.isDefault:
			row.status = theme.Styles.Checkmark.String() // Checkmark for default.
		case row.isInstalled:
			row.status = theme.Styles.Checkmark.String() // Checkmark for installed.
		default:
			row.status = " " // No indicator for available-only.
		}

		// Update column widths (account for " (default)" suffix).
		versionLen := len(v)
		if row.isDefault {
			versionLen += len(" (default)")
		}
		if versionLen > versionWidth {
			versionWidth = versionLen
		}

		rows = append(rows, row)
	}

	return rows, statusWidth, versionWidth
}

// adjustColumnWidths adjusts column widths to fit within terminal width.
func adjustColumnWidths(statusWidth, versionWidth, terminalWidth int) (int, int) {
	// Add padding.
	statusWidth += 4
	versionWidth += 4

	// Calculate total width needed.
	totalNeededWidth := statusWidth + versionWidth

	// If screen is narrow, truncate version column.
	if totalNeededWidth > terminalWidth {
		excess := totalNeededWidth - terminalWidth
		versionReduce := min(excess, versionWidth-12)
		versionWidth -= versionReduce
	}

	return statusWidth, versionWidth
}

// createVersionTableColumns creates table column definitions for version table.
func createVersionTableColumns(statusWidth, versionWidth int) []table.Column {
	return []table.Column{
		{Title: " ", Width: statusWidth}, // Status column.
		{Title: "VERSION", Width: versionWidth},
	}
}

// convertVersionRowsToTableFormat converts versionRow structs to table.Row format.
func convertVersionRowsToTableFormat(rows []versionRow) []table.Row {
	var tableRows []table.Row
	for _, row := range rows {
		suffix := ""
		if row.isDefault {
			suffix = " (default)"
		}
		tableRows = append(tableRows, table.Row{
			row.status,
			row.version + suffix,
		})
	}
	return tableRows
}

// applyTableStyles creates and applies theme styles to table.
func applyTableStyles(t *table.Model) {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		BorderBottom(true).
		Bold(true)
	s.Cell = s.Cell.PaddingLeft(1).PaddingRight(1)
	s.Selected = s.Cell

	t.SetStyles(s)
}
