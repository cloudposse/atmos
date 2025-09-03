package tui

import "github.com/charmbracelet/lipgloss"

const (
	// Unicode symbols.
	CheckPass = "✔" // UTF-8 check mark
	CheckFail = "✘" // UTF-8 cross mark
	CheckSkip = "⊘" // UTF-8 circled division slash

	// Color constants using hex values (mapped to ANSI by Lipgloss based on terminal capabilities).
	// These follow the PRD specification while maintaining compatibility across all environments.
	colorGreen     = "#2ECC40" // Bright green for pass (maps to ANSI green)
	colorRed       = "#DC143C" // Crimson red for fail (maps to ANSI red)
	colorAmber     = "#FFB347" // Peach orange for skip (maps to ANSI yellow)
	colorLightGray = "#D3D3D3" // Light gray for test names
	colorDarkGray  = "#666666" // Dark gray for durations
	colorBlue      = "#5DADE2" // Blue for spinner
	colorDarkRed   = "#B22222" // Dark red for error background
	colorWhite     = "#FFFFFF" // White for error text
)

var (
	// Test result styles.
	PassStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	FailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed))
	SkipStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAmber))

	// Text styles.
	TestNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorLightGray))  // Light gray for test names
	DurationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDarkGray)) // Dark gray for durations

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
	TestNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorLightGray))  // Light gray for test names
	DurationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDarkGray)) // Dark gray for durations
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorBlue))
	errorHeaderStyle = lipgloss.NewStyle().
		SetString("ERROR").
		Padding(0, 1, 0, 1).
		Background(lipgloss.Color(colorDarkRed)).
		Foreground(lipgloss.Color(colorWhite))
	StatsHeaderStyle = lipgloss.NewStyle().Bold(true)
}
