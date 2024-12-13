package markdown

import "github.com/charmbracelet/lipgloss"

// Colors defines the color scheme for markdown rendering
var Colors = struct {
	Primary   lipgloss.AdaptiveColor
	Secondary lipgloss.AdaptiveColor
	Success   lipgloss.AdaptiveColor
}{
	Primary:   lipgloss.AdaptiveColor{Light: "#00A3E0", Dark: "#00A3E0"}, // Atmos blue
	Secondary: lipgloss.AdaptiveColor{Light: "#4A5568", Dark: "#A0AEC0"}, // Slate gray
	Success:   lipgloss.AdaptiveColor{Light: "#48BB78", Dark: "#68D391"}, // Green
}
