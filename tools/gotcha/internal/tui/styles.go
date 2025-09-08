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
	colorRed       = "#FF0000" // Bright red for fail (properly maps to ANSI red in all profiles)
	colorAmber     = "#FFDD00" // Golden yellow for skip (maps correctly to ANSI yellow, not red)
	colorLightGray = "#D3D3D3" // Light gray for test names
	colorDarkGray  = "#666666" // Dark gray for durations
	colorBlue      = "#5DADE2" // Blue for spinner
	colorDarkRed   = "#B22222" // Dark red for error background
	colorWhite     = "#FFFFFF" // White for error text
)

var (
	// Store the current renderer to ensure consistent style creation.
	currentRenderer *lipgloss.Renderer

	// Test result styles.
	PassStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	FailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed))
	SkipStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAmber))

	// Text styles.
	TestNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorLightGray)) // Light gray for test names
	DurationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDarkGray))  // Dark gray for durations

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

	// Package header style for displaying package paths.
	PackageHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorBlue)).
				Bold(true)
)

// SetRenderer sets the current renderer for style creation.
func SetRenderer(r *lipgloss.Renderer) {
	currentRenderer = r
}

// InitStyles reinitializes all styles after color profile is set.
// This must be called after ConfigureColors() to ensure styles use the correct profile.
func InitStyles() {
	// Use the current renderer if available, otherwise use default NewStyle
	if currentRenderer != nil {
		// Create styles using the custom renderer
		PassStyle = currentRenderer.NewStyle().Foreground(lipgloss.Color(colorGreen))
		FailStyle = currentRenderer.NewStyle().Foreground(lipgloss.Color(colorRed))
		SkipStyle = currentRenderer.NewStyle().Foreground(lipgloss.Color(colorAmber))
		TestNameStyle = currentRenderer.NewStyle().Foreground(lipgloss.Color(colorLightGray))
		DurationStyle = currentRenderer.NewStyle().Foreground(lipgloss.Color(colorDarkGray))
		spinnerStyle = currentRenderer.NewStyle().Foreground(lipgloss.Color(colorBlue))
		errorHeaderStyle = currentRenderer.NewStyle().
			SetString("ERROR").
			Padding(0, 1, 0, 1).
			Background(lipgloss.Color(colorDarkRed)).
			Foreground(lipgloss.Color(colorWhite))
		StatsHeaderStyle = currentRenderer.NewStyle().Bold(true)
		PackageHeaderStyle = currentRenderer.NewStyle().
			Foreground(lipgloss.Color(colorBlue)).
			Bold(true)
	} else {
		// Fallback to default style creation
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
		PackageHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorBlue)).
			Bold(true)
	}
}
