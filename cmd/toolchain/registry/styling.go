package registry

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// rowStyler defines the interface for row data that can be styled.
type rowStyler interface {
	GetIsInstalled() bool
	GetIsInConfig() bool
}

// Ensure toolRow and searchRow implement rowStyler.
var (
	_ rowStyler = (*toolRow)(nil)
	_ rowStyler = (*searchRow)(nil)
)

// GetIsInstalled returns whether the tool is installed.
func (r toolRow) GetIsInstalled() bool {
	return r.isInstalled
}

// GetIsInConfig returns whether the tool is in config.
func (r toolRow) GetIsInConfig() bool {
	return r.isInConfig
}

// GetIsInstalled returns whether the tool is installed.
func (r searchRow) GetIsInstalled() bool {
	return r.isInstalled
}

// GetIsInConfig returns whether the tool is in config.
func (r searchRow) GetIsInConfig() bool {
	return r.isInConfig
}

// renderTableWithConditionalStyling applies color to status indicators and dims non-installed rows.
// This is a generic helper used by both list and search commands.
func renderTableWithConditionalStyling[T rowStyler](tableView string, rows []T) string {
	lines := strings.Split(tableView, "\n")

	// Define styles.
	greenDot := lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green for installed.
	grayDot := lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // Gray for in config but not installed.
	grayRow := lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // Gray for entire uninstalled row.

	// Apply conditional styling to each row.
	for i, line := range lines {
		if i == 0 || i == 1 {
			// Header and border lines - keep as is.
			continue
		}

		// Skip empty lines.
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Map line to row data (adjust index for header and border).
		rowIndex := i - 2
		if rowIndex >= 0 && rowIndex < len(rows) {
			rowData := rows[rowIndex]

			// Color the status dot and apply row styling.
			if rowData.GetIsInstalled() {
				// Replace the dot with a green dot.
				line = strings.Replace(line, statusIndicator, greenDot.Render(statusIndicator), 1)
			} else if rowData.GetIsInConfig() {
				// Replace the dot with a gray dot and gray the entire row.
				line = strings.Replace(line, statusIndicator, grayDot.Render(statusIndicator), 1)
				line = grayRow.Render(line)
			}

			lines[i] = line
		}
	}

	return strings.Join(lines, "\n")
}
