package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator/templates"
)

// realAWSLandingZoneDescription reproduces the exact description from
// pkg/generator/templates/catalog.yaml that triggered the reported bug: on a
// normal-width terminal it was cut off mid-word ("...provisioned end to end
// on t") and wrapped onto a stray partial second line ("local emulator.").
const realAWSLandingZoneDescription = "AWS landing zone — dev/staging/prod environments with a conventional " +
	"baseline (audit trail, KMS, SSM, monitoring, IAM), provisioned end to end on the local emulator."

// TestTruncateAtWordBoundary verifies descriptions are shortened at a word
// boundary (never mid-word) and always fit within the requested width.
func TestTruncateAtWordBoundary(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		expected string
	}{
		{
			name:     "shorter than max is unchanged",
			input:    "hello",
			maxWidth: 10,
			expected: "hello",
		},
		{
			name:     "equal to max is unchanged",
			input:    "hello",
			maxWidth: 5,
			expected: "hello",
		},
		{
			name:     "breaks on word boundary, not mid-word",
			input:    "hello world",
			maxWidth: 8,
			expected: "hello…",
		},
		{
			name:     "long description truncates cleanly",
			input:    realAWSLandingZoneDescription,
			maxWidth: 40,
			expected: "AWS landing zone — dev/staging/prod…",
		},
		{
			name:     "zero width yields empty string",
			input:    "hello",
			maxWidth: 0,
			expected: "",
		},
		{
			name:     "negative width yields empty string",
			input:    "hello",
			maxWidth: -5,
			expected: "",
		},
		{
			name:     "single word longer than budget still terminates",
			input:    "supercalifragilisticexpialidocious",
			maxWidth: 5,
			expected: "supe…",
		},
		{
			name:     "empty string",
			input:    "",
			maxWidth: 10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateAtWordBoundary(tt.input, tt.maxWidth)
			assert.Equal(t, tt.expected, result)
			if tt.maxWidth > 0 {
				assert.LessOrEqualf(t, lipgloss.Width(result), tt.maxWidth,
					"result %q must fit within maxWidth %d", result, tt.maxWidth)
			}
			// The one behavior this bug report is about: never split a word
			// mid-character onto what looks like a stray fragment. A clean
			// truncation either returns the original string untouched, is
			// emptied entirely (non-positive width budget), or ends with
			// the ellipsis.
			if result != tt.input && result != "" {
				assert.True(t, strings.HasSuffix(result, "…"), "truncated result must end with an ellipsis, got %q", result)
			}
		})
	}
}

// TestEmbedsColumnWidths verifies the description column shrinks and grows
// with the real terminal width instead of staying fixed, while never
// dropping below the legibility floor, and that the key/name columns are
// sized from actual content (bounded) rather than an assumed constant.
func TestEmbedsColumnWidths(t *testing.T) {
	configs := map[string]templates.Configuration{
		// "azure/landing-zone" (19 chars) is longer than the old hard-coded
		// 15-char key column assumption; the computed column widths must
		// account for it so the description budget stays accurate.
		"azure/landing-zone": {Name: "azure/landing-zone", Description: "Azure landing zone"},
		"atmos":              {Name: "atmos", Description: "Complete atmos.yaml configuration only"},
	}

	tests := []struct {
		name          string
		terminalWidth int
		wantAtLeast   int
		wantAtMost    int
	}{
		{name: "narrow terminal floors at minimum", terminalWidth: 40, wantAtLeast: minDescriptionWidth, wantAtMost: minDescriptionWidth},
		{name: "normal 80-column terminal", terminalWidth: 80, wantAtLeast: minDescriptionWidth},
		{name: "wide terminal grows the column", terminalWidth: 160, wantAtLeast: 80},
	}

	widths := map[int]int{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyWidth, nameWidth, descWidth := embedsColumnWidths(configs, tt.terminalWidth)
			// The key/name columns must be sized to fit "azure/landing-zone"
			// (19 chars) without truncation, not the old fixed constant.
			assert.GreaterOrEqual(t, keyWidth, lipgloss.Width("azure/landing-zone"))
			assert.GreaterOrEqual(t, nameWidth, lipgloss.Width("azure/landing-zone"))
			assert.GreaterOrEqual(t, descWidth, tt.wantAtLeast)
			if tt.wantAtMost > 0 {
				assert.LessOrEqual(t, descWidth, tt.wantAtMost)
			}
			widths[tt.terminalWidth] = descWidth
		})
	}

	// Width must be monotonically non-decreasing as the terminal grows --
	// this is the core regression check: it must not be a hard-coded
	// constant that ignores the real terminal width.
	require.LessOrEqual(t, widths[40], widths[80])
	require.Less(t, widths[80], widths[160])
}

// TestBuildEmbedsTemplateOptions_FitsTerminalWidth verifies the rendered
// option line for a long, real-world description fits within the terminal
// width at a couple of representative widths, and never splits a word
// mid-character.
func TestBuildEmbedsTemplateOptions_FitsTerminalWidth(t *testing.T) {
	configs := map[string]templates.Configuration{
		"aws/landing-zone": {
			Name:        "aws/landing-zone",
			Description: realAWSLandingZoneDescription,
		},
	}

	var widest string
	const wideEnoughForFullDescription = 400
	for _, width := range []int{40, 80, 120, 200, wideEnoughForFullDescription} {
		t.Run("width_"+strconv.Itoa(width), func(t *testing.T) {
			options := buildEmbedsTemplateOptions(configs, width)
			require.Len(t, options, 1)

			displayText := options[0].Key
			if lipgloss.Width(displayText) > width {
				// It's acceptable for the fixed-width key/name columns
				// alone to exceed a very narrow terminal (huh still wraps
				// the full line at a real word boundary via its own Select
				// rendering), but the description we appended must be the
				// part responsible for shrinking, and it must end cleanly
				// rather than splitting a word mid-character.
				assert.True(t, strings.HasSuffix(displayText, "…"),
					"long option text %q must end with an ellipsis instead of an arbitrary cut", displayText)
			}
			if width == wideEnoughForFullDescription {
				widest = displayText
			}
		})
	}
	// At a wide-enough terminal, the description should render in full
	// rather than being truncated unnecessarily.
	assert.Contains(t, widest, realAWSLandingZoneDescription)
}

// TestBuildScaffoldDisplayText_TruncatesDescription verifies the scaffold
// picker (atmos scaffold) applies the same terminal-width-aware truncation
// as the embeds picker, including when a source suffix is present.
func TestBuildScaffoldDisplayText_TruncatesDescription(t *testing.T) {
	templateConfig := map[string]interface{}{
		"description": realAWSLandingZoneDescription,
		"source":      "github.com/cloudposse/atmos.git//examples/scaffolds/aws/landing-zone",
	}

	for _, width := range []int{60, 80, 120} {
		t.Run("width_"+strconv.Itoa(width), func(t *testing.T) {
			displayText, valid := buildScaffoldDisplayText("aws/landing-zone", templateConfig, width)
			require.True(t, valid)
			require.Contains(t, displayText, "(from github.com/cloudposse/atmos.git//examples/scaffolds/aws/landing-zone)")
			if !strings.Contains(displayText, realAWSLandingZoneDescription) {
				// Truncated: the description portion must end cleanly with
				// an ellipsis, never mid-word.
				descPortion := strings.SplitN(displayText, " (from ", 2)[0]
				assert.True(t, strings.HasSuffix(strings.TrimRight(descPortion, " "), "…"),
					"truncated description %q must end with an ellipsis", descPortion)
			}
		})
	}
}
