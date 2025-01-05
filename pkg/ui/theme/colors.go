package theme

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
)

// ANSI Colors (for lipgloss)
const (
	// Base colors
	ColorGray      = "8"   // Version number
	ColorGreen     = "10"  // Success, new version
	ColorCyan      = "14"  // Links, info
	ColorPink      = "211" // Package names
	ColorBlue      = "63"  // UI elements
	ColorDarkGray  = "241" // Subtle text
	ColorRed       = "9"   // Errors, x mark
	ColorCheckmark = "42"  // Checkmark
	ColorWhite     = "7"   // Default text

	// Hex colors
	ColorSelectedItem = "#10ff10" // Selected items in lists
	ColorBorder       = "62"      // UI borders
)

// Styles provides pre-configured lipgloss styles for common UI elements
var Styles = struct {
	VersionNumber lipgloss.Style
	NewVersion    lipgloss.Style
	Link          lipgloss.Style
	PackageName   lipgloss.Style
	Checkmark     lipgloss.Style
	XMark         lipgloss.Style
	GrayText      lipgloss.Style
	SelectedItem  lipgloss.Style
	CommandName   lipgloss.Style
	Description   lipgloss.Style
	Border        lipgloss.Style
}{
	VersionNumber: lipgloss.NewStyle().Foreground(lipgloss.Color(ColorGray)),
	NewVersion:    lipgloss.NewStyle().Foreground(lipgloss.Color(ColorGreen)),
	Link:          lipgloss.NewStyle().Foreground(lipgloss.Color(ColorCyan)),
	PackageName:   lipgloss.NewStyle().Foreground(lipgloss.Color(ColorPink)),
	Checkmark:     lipgloss.NewStyle().Foreground(lipgloss.Color(ColorCheckmark)).SetString("✓"),
	XMark:         lipgloss.NewStyle().Foreground(lipgloss.Color(ColorRed)).SetString("x"),
	GrayText:      lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDarkGray)),
	SelectedItem:  lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color(ColorSelectedItem)),
	CommandName:   lipgloss.NewStyle().Foreground(lipgloss.Color(ColorGreen)),
	Description:   lipgloss.NewStyle().Foreground(lipgloss.Color(ColorWhite)),
	Border:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(ColorBorder)),
}

// Colors provides color.Attribute mappings for the old color.New style
var Colors = struct {
	Error   *color.Color
	Info    *color.Color
	Success *color.Color
	Warning *color.Color
	Default *color.Color
}{
	Error:   color.New(color.FgRed),
	Info:    color.New(color.FgCyan),
	Success: color.New(color.FgGreen),
	Warning: color.New(color.FgYellow),
	Default: color.New(color.Reset),
}
