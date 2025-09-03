package tui

import "github.com/charmbracelet/lipgloss"

const (
	// Unicode symbols.
	CheckPass = "✔" // UTF-8 check mark
	CheckFail = "✘" // UTF-8 cross mark
	CheckSkip = "⊘" // UTF-8 circled division slash

	// ANSI color constants for better CI/terminal compatibility.
	colorGreen     = "2"  // Bright green for pass (ANSI)
	colorRed       = "1"  // Bright red for fail (ANSI)
	colorAmber     = "3"  // Yellow/amber for skip (ANSI)
	colorLightGray = "7"  // Light gray for test names (ANSI)
	colorDarkGray  = "8"  // Dark gray for durations (ANSI)
	colorBlue      = "4"  // Blue for spinner (ANSI)
	colorDarkRed   = "88" // Dark red for error background (256 color)
	colorWhite     = "15" // White for error text (ANSI)
)

var (
	// Test result styles.
	PassStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	FailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed))
	SkipStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAmber))

	// Text styles.
	TestNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorLightGray))
	DurationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDarkGray))

	// UI element styles.
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorBlue))

	// Error display style.
	errorHeaderStyle = lipgloss.NewStyle().
				SetString("ERROR").
				Padding(0, 1, 0, 1).
				Background(lipgloss.Color(colorDarkRed)).
				Foreground(lipgloss.Color(colorWhite))

	// Statistics styles.
	StatsHeaderStyle = lipgloss.NewStyle().Bold(true)
)

// InitStyles reinitializes all styles after color profile is set.
// This must be called after ConfigureColors() to ensure styles use the correct profile.
func InitStyles() {
	// Reinitialize all styles with the current color profile
	PassStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	FailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed))
	SkipStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAmber))
	TestNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorLightGray))
	DurationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDarkGray))
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorBlue))
	errorHeaderStyle = lipgloss.NewStyle().
		SetString("ERROR").
		Padding(0, 1, 0, 1).
		Background(lipgloss.Color(colorDarkRed)).
		Foreground(lipgloss.Color(colorWhite))
	StatsHeaderStyle = lipgloss.NewStyle().Bold(true)
}
