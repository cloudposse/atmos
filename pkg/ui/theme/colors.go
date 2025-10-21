package theme

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
)

const (
	// Base colors
	ColorGray      = "#808080" // Version number
	ColorGreen     = "#00FF00" // Success, new version
	ColorCyan      = "#00FFFF" // Links, info
	ColorPink      = "#FF69B4" // Package names
	ColorBlue      = "#5F5FFF" // UI elements
	ColorDarkGray  = "#626262" // Subtle text
	ColorRed       = "#FF0000" // Errors, x mark
	ColorYellow    = "#FFFF00" // Available for future use (pure yellow can be hard to read on light terminals)
	ColorOrange    = "#FFA500" // Warnings, moderate depth (preferred over pure yellow for better readability)
	ColorCheckmark = "#00D700" // Checkmark
	ColorWhite     = "#FFFFFF" // Default text

	ColorSelectedItem = "#10ff10" // Selected items in lists
	ColorBorder       = "#5F5FD7" // UI borders
)

type HelpStyle struct {
	Headings      *color.Color
	Commands      *color.Color
	Example       *color.Color
	ExecName      *color.Color
	Flags         *color.Color
	CmdShortDescr *color.Color
	FlagsDescr    *color.Color
	FlagsDataType *color.Color
	Aliases       *color.Color
}

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
	Help          HelpStyle
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
	Help: HelpStyle{
		Headings: color.New(color.FgHiCyan).Add(color.Bold).Add(color.Underline),
		Commands: color.New(color.FgHiGreen).Add(color.Bold),
		Example:  color.New(color.Italic),
		ExecName: color.New(color.Bold),
		Flags:    color.New(color.Bold),
	},
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
