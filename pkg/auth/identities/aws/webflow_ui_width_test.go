package aws

// Tests that the browser sign-in dialog fits an 80-column terminal.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ui/link"
)

// webflowTerminalWidth is the narrowest terminal the dialog must fit without wrapping.
const webflowTerminalWidth = 80

// webflowBoxLines returns the lines of a rendered dialog that belong to its border.
func webflowBoxLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.ContainsAny(line, "│╭╰") {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestRenderWebflowDialog_FitsStandardTerminal asserts that no dialog line,
// border included, is wider than an 80-column terminal for any URL that can be
// drawn inside the box (issue #3720, finding 1.3).
func TestRenderWebflowDialog_FitsStandardTerminal(t *testing.T) {
	urlWithWidth := func(width int) string {
		return "https://" + strings.Repeat("x", width-len("https://"))
	}

	tests := []struct {
		name string
		url  string
	}{
		{name: "short URL", url: "https://example.com/auth"},
		{name: "URL as wide as the link inline limit", url: urlWithWidth(link.MaxInlineWidth)},
		{name: "URL one column under the link inline limit", url: urlWithWidth(link.MaxInlineWidth - 1)},
		{name: "long URL delivered outside the box", url: urlWithWidth(400)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialog, _ := renderWebflowDialog(tt.url)

			lines := webflowBoxLines(dialog)
			require.NotEmpty(t, lines, "the dialog box must render")
			for _, line := range lines {
				assert.LessOrEqual(t, lipgloss.Width(line), webflowTerminalWidth,
					"dialog line wraps in an 80-column terminal: %q", line)
			}
		})
	}
}
