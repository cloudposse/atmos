package testreport

import (
	"io"
	"strings"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func TestReporterProgressRespectsColorProfile(t *testing.T) {
	original := ui.GetColorProfile()
	t.Cleanup(func() { ui.SetColorProfile(original) })
	for _, tc := range []struct {
		name    string
		profile termenv.Profile
		color   string
	}{
		{"truecolor recording", termenv.TrueColor, "\x1b[38;2;"},
		{"256 color terminal", termenv.ANSI256, "\x1b[38;5;"},
		{"no color", termenv.Ascii, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ui.SetColorProfile(tc.profile)
			reporter := New("Checks", []*Node{
				{ID: "passed", Name: "Passed", Status: Passed},
				{ID: "pending", Name: "Pending", Status: Pending},
			}, io.Discard)
			var bar string
			for _, line := range strings.Split(reporter.View(100, "", false), "\n") {
				if strings.Contains(line, "50%") {
					bar = line
					break
				}
			}
			require.NotEmpty(t, bar)
			assert.Contains(t, bar, "█")
			if tc.color == "" {
				assert.NotContains(t, bar, "\x1b")
			} else {
				assert.Contains(t, bar, tc.color)
			}
		})
	}
}

func TestReporterSpinnerRespectsThemeAndColorProfile(t *testing.T) {
	original := ui.GetColorProfile()
	t.Cleanup(func() { ui.SetColorProfile(original) })
	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.Ascii} {
		ui.SetColorProfile(profile)
		reporter := New("Checks", []*Node{{ID: "running", Name: "Health", Status: Running}}, io.Discard)
		for _, frame := range []string{"⣽ ", ""} {
			glyph := strings.TrimSpace(frame)
			if glyph == "" {
				glyph = "◌"
			}
			expected := theme.GetCurrentStyles().Spinner.Render(glyph)
			if profile != termenv.Ascii {
				require.Contains(t, expected, "\x1b[", "the shared spinner style must emit color")
			}
			view := reporter.View(100, frame, false)
			assert.Contains(t, view, expected+"  ", "running markers must use the shared spinner color")
			if profile == termenv.Ascii {
				assert.NotContains(t, view, "\x1b")
			}
		}
	}
}
