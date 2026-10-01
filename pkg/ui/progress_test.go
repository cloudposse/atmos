package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func TestProgressThemeAndProfile(t *testing.T) {
	original := GetColorProfile()
	t.Cleanup(func() { SetColorProfile(original) })
	for _, name := range []string{"atmos", "dracula", "3024 Day"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ATMOS_THEME", name)
			_, err := theme.GetColorSchemeForTheme(name)
			require.NoError(t, err)
			for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.Ascii} {
				SetColorProfile(profile)
				bar := NewProgress(progress.WithWidth(10), progress.WithoutPercentage())
				view := bar.ViewAs(0.5)
				assert.Equal(t, "█████░░░░░", ansi.Strip(view))
				if profile == termenv.Ascii {
					assert.NotContains(t, view, "\x1b")
				} else {
					// Both the filled gradient and the empty cells must carry their own color.
					for _, cell := range []string{"█", "░"} {
						index := strings.Index(view, cell)
						require.Greater(t, index, 0)
						assert.Contains(t, view[:index], "\x1b[")
					}
					expectedEmpty := termenv.String("░").Foreground(profile.Color(theme.GetCurrentColorScheme().TextMuted)).String()
					assert.Contains(t, view, expectedEmpty)
				}
			}
		})
	}
}

func TestProgressResolvesChangedTheme(t *testing.T) {
	original := GetColorProfile()
	t.Cleanup(func() { SetColorProfile(original) })
	SetColorProfile(termenv.TrueColor)
	t.Setenv("ATMOS_THEME", "atmos")
	first := NewProgress(progress.WithWidth(10)).ViewAs(0.5)
	t.Setenv("ATMOS_THEME", "dracula")
	second := NewProgress(progress.WithWidth(10)).ViewAs(0.5)
	assert.NotEqual(t, first, second)
	assert.Equal(t, ansi.Strip(first), ansi.Strip(second))
}

func TestSpinnerThemeFramesAndRate(t *testing.T) {
	original := GetColorProfile()
	t.Cleanup(func() { SetColorProfile(original) })
	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.Ascii} {
		SetColorProfile(profile)
		for _, value := range []string{"", "5", "invalid", "0", "100"} {
			t.Run(fmt.Sprintf("%d/%s", profile, value), func(t *testing.T) {
				t.Setenv("ATMOS_SPINNER_FPS", value)
				s := NewSpinner()
				assert.Equal(t, spinner.Dot.Frames, s.Spinner.Frames)
				fps := spinner.Dot.FPS
				if value == "5" {
					fps = time.Second / 5
				}
				if value == "100" {
					fps = time.Second / 60
				}
				assert.Equal(t, fps, s.Spinner.FPS)
				assert.Equal(t, theme.GetCurrentStyles().Spinner.Render(spinner.Dot.Frames[0]), s.View())
				if profile == termenv.Ascii {
					assert.NotContains(t, s.View(), "\x1b")
				}
			})
		}
	}
}

func TestProgressConfiguredColorThroughPipe(t *testing.T) {
	_, stderr, cleanup := setupTestUI(t)
	defer cleanup()
	original := GetColorProfile()
	t.Cleanup(func() { SetColorProfile(original) })
	// CLI initialization applies forced color after detecting piped streams.
	SetColorProfile(termenv.TrueColor)
	bar := NewProgress(progress.WithWidth(10), progress.WithoutPercentage())
	Write(bar.ViewAs(0.5))
	assert.Equal(t, "█████░░░░░", ansi.Strip(stderr.String()))
	assert.Contains(t, stderr.String(), "\x1b[38;2;")
}

func TestProgressPreservesCallerOptions(t *testing.T) {
	original := GetColorProfile()
	t.Cleanup(func() { SetColorProfile(original) })
	SetColorProfile(termenv.TrueColor)
	bar := NewProgress(
		progress.WithWidth(6),
		progress.WithoutPercentage(),
		progress.WithSolidFill("#ff0000"),
		func(bar *progress.Model) { bar.EmptyColor = "#0000ff" },
	)
	view := bar.ViewAs(0.5)
	assert.Equal(t, "███░░░", ansi.Strip(view))
	assert.Contains(t, view, termenv.String("█").Foreground(termenv.TrueColor.Color("#ff0000")).String())
	assert.Contains(t, view, termenv.String("░").Foreground(termenv.TrueColor.Color("#0000ff")).String())
}

func TestAtmosProgressMatchesOriginalGradient(t *testing.T) {
	t.Setenv("ATMOS_THEME", "atmos")
	original := GetColorProfile()
	t.Cleanup(func() { SetColorProfile(original) })
	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.Ascii} {
		SetColorProfile(profile)
		actual := NewProgress(progress.WithWidth(20), progress.WithoutPercentage())
		legacy := progress.New(progress.WithDefaultGradient(), progress.WithColorProfile(profile),
			progress.WithWidth(20), progress.WithoutPercentage())
		// Empty cells retain Atmos's muted styling; compare every filled gradient cell.
		assert.Equal(t, legacy.ViewAs(1), actual.ViewAs(1))
	}
}
