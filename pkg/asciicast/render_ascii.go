package asciicast

import (
	"os"
	"strings"

	"github.com/charmbracelet/x/cellbuf"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RenderASCII writes the final terminal content of a cast file as plain text
// with no ANSI escape sequences. The output is a durable, diffable artifact
// suitable for committing to git and for machine consumption (e.g. Atmos Pro).
func RenderASCII(input, output string) error {
	defer perf.Track(nil, "asciicast.RenderASCII")()

	grid, err := BuildGrid(input)
	if err != nil {
		return err
	}
	return os.WriteFile(output, []byte(gridText(grid)), castFilePerm)
}

// gridText flattens a cell grid to right-trimmed plain-text lines.
func gridText(grid *cellbuf.Buffer) string {
	var sb strings.Builder
	height := grid.Height()
	for y := 0; y < height; y++ {
		sb.WriteString(strings.TrimRight(gridLineText(grid, y), " "))
		sb.WriteByte('\n')
	}
	// Collapse trailing blank lines to a single terminating newline.
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func gridLineText(grid *cellbuf.Buffer, y int) string {
	var sb strings.Builder
	for x := 0; x < grid.Width(); x++ {
		cell := grid.Cell(x, y)
		if cell == nil || cell.Width == 0 {
			// Zero-width cells are continuations of a wide rune already emitted.
			continue
		}
		sb.WriteString(cell.String())
	}
	return sb.String()
}
