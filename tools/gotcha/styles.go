package main

import "github.com/charmbracelet/lipgloss"

const (
	checkPass = "✔" // UTF-8 check mark
	checkFail = "✘" // UTF-8 cross mark
	checkSkip = "⊘" // UTF-8 circled division slash
)

var (
	// Checkmarks with colors (NOT emoji)
	passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // Green
	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // Red
	skipStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // Amber

	// Test name in light gray
	testNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#D3D3D3"))

	// Duration in darker gray
	durationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))

	// Spinner style
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	// Error with solid background
	errorHeaderStyle = lipgloss.NewStyle().
				SetString("ERROR").
				Padding(0, 1, 0, 1).
				Background(lipgloss.Color("204")).
				Foreground(lipgloss.Color("255"))

	// Statistics styles
	statsHeaderStyle = lipgloss.NewStyle().Bold(true)
)
