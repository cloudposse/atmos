package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	// Number of blank columns between adjacent items.
	columnGap = 2
	// Default width used when the caller has no real terminal width (e.g.
	// TerminalWidth() on a non-TTY stream), matching the fallback width used
	// elsewhere in this package (see pkg/ui/markdown.defaultWidth).
	columnDefaultWidth = 80
)

// FormatColumns lays out items in evenly spaced, left-aligned columns sized
// to fit width, filling top-to-bottom then left-to-right (the same order
// `ls` uses for a terminal listing), instead of a single comma-separated
// line that wraps unpredictably and can break mid-word. Every column is the
// same width (the longest item plus a small gap); only the last column may
// be shorter than the others when items don't divide evenly. Returns "" for
// an empty items slice.
func FormatColumns(items []string, width int) string {
	if len(items) == 0 {
		return ""
	}
	if width <= 0 {
		width = columnDefaultWidth
	}

	layout := columnGrid(len(items), maxItemWidth(items)+columnGap, width)

	var b strings.Builder
	for row := 0; row < layout.numRows; row++ {
		writeColumnRow(&b, items, row, layout)
	}
	return strings.TrimRight(b.String(), "\n")
}

// maxItemWidth returns the display width of the longest item.
func maxItemWidth(items []string) int {
	maxLen := 0
	for _, item := range items {
		if w := lipgloss.Width(item); w > maxLen {
			maxLen = w
		}
	}
	return maxLen
}

// columnLayout describes a column-major grid: how many columns and rows it
// has, and how wide each column is (including the gap to the next column).
type columnLayout struct {
	numCols  int
	numRows  int
	colWidth int
}

// columnGrid computes how many columns and rows a grid of count items needs
// to fit within width, given each column is colWidth wide.
func columnGrid(count, colWidth, width int) columnLayout {
	numCols := width / colWidth
	if numCols < 1 {
		numCols = 1
	}
	numRows := (count + numCols - 1) / numCols
	// Re-derive the column count from the row count so the grid is as full as
	// possible -- avoids a sparse trailing column left over from the
	// width-based estimate above.
	numCols = (count + numRows - 1) / numRows
	return columnLayout{numCols: numCols, numRows: numRows, colWidth: colWidth}
}

// writeColumnRow writes one row of a column-major grid (ls-style: column 0
// holds the first numRows items top-to-bottom, column 1 the next numRows,
// and so on), padding every item except the last populated one in the row.
func writeColumnRow(b *strings.Builder, items []string, row int, layout columnLayout) {
	for col := 0; col < layout.numCols; col++ {
		idx := col*layout.numRows + row
		if idx >= len(items) {
			// Only the last column can be short (see columnGrid), so nothing
			// later in this row is populated either.
			break
		}
		item := items[idx]
		b.WriteString(item)
		nextIdx := (col+1)*layout.numRows + row
		if col < layout.numCols-1 && nextIdx < len(items) {
			b.WriteString(strings.Repeat(" ", layout.colWidth-lipgloss.Width(item)))
		}
	}
	b.WriteString("\n")
}
