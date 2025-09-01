package main

import "github.com/charmbracelet/lipgloss"

const (
	// Unicode symbols
	checkPass = "✔" // UTF-8 check mark
	checkFail = "✘" // UTF-8 cross mark
	checkSkip = "⊘" // UTF-8 circled division slash

	// Hex color constants
	colorGreen     = "#2ECC40" // Bright green for pass
	colorRed       = "#DC143C" // Crimson red for fail
	colorAmber     = "#FFB347" // Peach orange for skip
	colorLightGray = "#D3D3D3" // Light gray for test names
	colorDarkGray  = "#666666" // Dark gray for durations
	colorBlue      = "#5DADE2" // Blue for spinner
	colorDarkRed   = "#B22222" // Dark red for error background
	colorWhite     = "#FFFFFF" // White for error text
)

var (
	// Test result styles
	passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed))
	skipStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAmber))

	// Text styles
	testNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorLightGray))
	durationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDarkGray))

	// UI element styles
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorBlue))

	// Error display style
	errorHeaderStyle = lipgloss.NewStyle().
				SetString("ERROR").
				Padding(0, 1, 0, 1).
				Background(lipgloss.Color(colorDarkRed)).
				Foreground(lipgloss.Color(colorWhite))

	// Statistics styles
	statsHeaderStyle = lipgloss.NewStyle().Bold(true)
)
