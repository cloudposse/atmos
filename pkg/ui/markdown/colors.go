package markdown

import "github.com/charmbracelet/lipgloss"

// Colors defines the color scheme for markdown rendering
var Colors = struct {
	Primary    lipgloss.AdaptiveColor
	Secondary  lipgloss.AdaptiveColor
	Success    lipgloss.AdaptiveColor
	Warning    lipgloss.AdaptiveColor
	Error      lipgloss.AdaptiveColor
	Info       lipgloss.AdaptiveColor
	Subtle     lipgloss.AdaptiveColor
	HeaderBg   lipgloss.AdaptiveColor
	Border     lipgloss.AdaptiveColor
	Background lipgloss.AdaptiveColor
}{
	Primary:    lipgloss.AdaptiveColor{Light: "#00A3E0", Dark: "#00A3E0"}, // Atmos blue
	Secondary:  lipgloss.AdaptiveColor{Light: "#4A5568", Dark: "#A0AEC0"}, // Slate gray
	Success:    lipgloss.AdaptiveColor{Light: "#48BB78", Dark: "#68D391"}, // Green
	Warning:    lipgloss.AdaptiveColor{Light: "#ECC94B", Dark: "#F6E05E"}, // Yellow
	Error:      lipgloss.AdaptiveColor{Light: "#F56565", Dark: "#FC8181"}, // Red
	Info:       lipgloss.AdaptiveColor{Light: "#4299E1", Dark: "#63B3ED"}, // Light blue
	Subtle:     lipgloss.AdaptiveColor{Light: "#718096", Dark: "#A0AEC0"}, // Gray
	HeaderBg:   lipgloss.AdaptiveColor{Light: "#2D3748", Dark: "#4A5568"}, // Dark slate
	Border:     lipgloss.AdaptiveColor{Light: "#CBD5E0", Dark: "#4A5568"}, // Light gray
	Background: lipgloss.AdaptiveColor{Light: "#F7FAFC", Dark: "#1A202C"}, // Off white/dark
}
