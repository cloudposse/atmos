package aws

// Tests that the device-authorization dialog fits an 80-column terminal.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ui/link"
)

// standardTerminalWidth is the narrowest terminal the dialog must fit without wrapping.
const standardTerminalWidth = 80

// TestRenderVerificationDialog_FitsStandardTerminal renders the dialog with every
// URL shape that can end up inside the box and asserts that no line, border
// included, is wider than an 80-column terminal (issue #3720, finding 1.3).
func TestRenderVerificationDialog_FitsStandardTerminal(t *testing.T) {
	urlWithWidth := func(width int) string {
		return "https://" + strings.Repeat("x", width-len("https://"))
	}

	tests := []struct {
		name string
		url  string
	}{
		{name: "legacy short device URL", url: testShortVerificationURL},
		{name: "URL as wide as the link inline limit", url: urlWithWidth(link.MaxInlineWidth)},
		{name: "URL one column under the link inline limit", url: urlWithWidth(link.MaxInlineWidth - 1)},
		{name: "long URL delivered outside the box", url: longVerificationURL()},
		{name: "no URL", url: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialog, _ := renderVerificationDialog("ABCD-1234", tt.url)

			lines := boxLines(dialog)
			require.NotEmpty(t, lines, "the dialog box must render")
			for _, line := range lines {
				assert.LessOrEqual(t, lipgloss.Width(line), standardTerminalWidth,
					"dialog line wraps in an 80-column terminal: %q", line)
			}
		})
	}
}
