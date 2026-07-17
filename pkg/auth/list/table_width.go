package list

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/internal/tui/templates"
	termUtils "github.com/cloudposse/atmos/internal/tui/templates/term"
)

// Minimum column widths (floors) applied only under terminal-width pressure.
// Floors never go below the column header width so headers stay readable.
const (
	minProviderNameWidth   = 10
	minProviderKindWidth   = 10
	minProviderRegionWidth = 6  // Width of the "REGION" header.
	minProviderURLWidth    = 15 // Width of the "START URL / URL" header.

	minIdentityNameWidth        = 10
	minIdentityKindWidth        = 10
	minIdentityViaProviderWidth = 12 // Width of the "VIA PROVIDER" header.
	minIdentityViaIdentityWidth = 12 // Width of the "VIA IDENTITY" header.
	minIdentityAliasWidth       = 8
)

// tableCellPadding is the horizontal padding the bubbles table adds around
// every cell (one space on each side, per table.DefaultStyles).
const tableCellPadding = 2

// Seams for TTY detection and terminal width, overridable in tests.
// The width source is templates.GetTerminalWidth, which honors
// settings.terminal.max_width as a ceiling.
var (
	isTTYForTable         = termUtils.IsTTYSupportForStdout
	terminalWidthForTable = templates.GetTerminalWidth
)

// columnSizingSpec describes how a single table column is sized.
type columnSizingSpec struct {
	title  string
	legacy int // Fixed width used when no TTY is attached, keeping piped output stable.
	floor  int // Minimum width the column may shrink to under terminal-width pressure.
}

// computeColumnWidths returns the width for each column.
// With a TTY attached, columns size to their natural content width and shrink
// (in shrinkOrder, down to their floors) only when the table would exceed the
// terminal width. Without a TTY, the legacy fixed widths are returned so
// piped and redirected output stays stable.
func computeColumnWidths(specs []columnSizingSpec, rows []table.Row, shrinkOrder []int) []int {
	if !isTTYForTable() {
		widths := make([]int, len(specs))
		for i := range specs {
			widths[i] = specs[i].legacy
		}
		return widths
	}
	return fitColumnWidths(specs, rows, shrinkOrder, terminalWidthForTable())
}

// fitColumnWidths sizes columns to their natural content width, then shrinks
// columns in shrinkOrder down to their floors while the rendered table would
// exceed terminalWidth.
func fitColumnWidths(specs []columnSizingSpec, rows []table.Row, shrinkOrder []int, terminalWidth int) []int {
	widths := contentColumnWidths(specs, rows)

	totalNeededWidth := tableCellPadding * len(widths)
	for _, w := range widths {
		totalNeededWidth += w
	}

	excess := totalNeededWidth - terminalWidth
	for _, idx := range shrinkOrder {
		if excess <= 0 {
			break
		}
		if reducible := widths[idx] - specs[idx].floor; reducible > 0 {
			cut := min(reducible, excess)
			widths[idx] -= cut
			excess -= cut
		}
	}

	return widths
}

// contentColumnWidths computes the natural width of each column: the widest
// cell in the column, never narrower than the column header.
func contentColumnWidths(specs []columnSizingSpec, rows []table.Row) []int {
	widths := make([]int, len(specs))
	for i := range specs {
		widths[i] = lipgloss.Width(specs[i].title)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			// lipgloss.Width is ANSI-aware, so styled cells measure correctly.
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// tableColumns builds bubbles table columns from sizing specs and computed widths.
func tableColumns(specs []columnSizingSpec, widths []int) []table.Column {
	columns := make([]table.Column, len(specs))
	for i := range specs {
		columns[i] = table.Column{Title: specs[i].title, Width: widths[i]}
	}
	return columns
}
