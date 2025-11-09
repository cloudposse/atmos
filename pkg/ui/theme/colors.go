package theme

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
)

// Legacy color constants - DEPRECATED.
// These hard-coded colors are no longer recommended for use in new code.
// Instead, use the theme-aware system via GetCurrentStyles() which automatically
// adapts colors based on the user's configured theme.
//
// Migration guide:
//   - Replace ColorGreen with styles.Success (or GetSuccessColor())
//   - Replace ColorRed with styles.Error (or GetErrorColor())
//   - Replace ColorCyan with styles.Info
//   - Replace ColorBlue with styles.Primary (or GetPrimaryColor())
//   - Replace ColorBorder with GetBorderColor()
//
// Example:
//
//	// Old (deprecated):
//	style := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorGreen))
//
//	// New (theme-aware):
//	styles := theme.GetCurrentStyles()
//	style := styles.Success
const (
	// Base colors - DEPRECATED: Use theme.GetCurrentStyles() instead.
	ColorGray         = "#808080" // Deprecated: Use styles.Muted
	ColorGreen        = "#00FF00" // Deprecated: Use styles.Success or GetSuccessColor()
	ColorCyan         = "#00FFFF" // Deprecated: Use styles.Info
	ColorPink         = "#FF69B4" // Deprecated: Use styles.Secondary
	ColorBlue         = "#5F5FFF" // Deprecated: Use styles.Primary or GetPrimaryColor()
	ColorDarkGray     = "#626262" // Deprecated: Use styles.Muted
	ColorRed          = "#FF0000" // Deprecated: Use styles.Error or GetErrorColor()
	ColorYellow       = "#FFFF00" // Deprecated: Use styles.Warning
	ColorOrange       = "#FFA500" // Deprecated: Use styles.Warning
	ColorCheckmark    = "#00D700" // Deprecated: Use styles.Success
	ColorWhite        = "#FFFFFF" // Deprecated: Use styles.Body
	ColorBrightYellow = "#FFFF00" // Deprecated: Use theme-aware colors
	ColorGold         = "#FFD700" // Deprecated: Use theme-aware colors

	ColorSelectedItem = "#10ff10" // Deprecated: Use styles.Selected
	ColorBorder       = "#5F5FD7" // Deprecated: Use GetBorderColor()
)

// HelpStyle - DEPRECATED: Use theme.GetCurrentStyles() instead.
// This struct provides legacy color.Color styling which doesn't support theme switching.
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

// Styles - DEPRECATED: Use theme.GetCurrentStyles() instead.
// This global variable uses hard-coded colors that don't respond to theme changes.
// New code should call GetCurrentStyles() to get theme-aware styles.
//
// Example migration:
//
//	// Old (deprecated):
//	fmt.Print(Styles.Checkmark.String())
//
//	// New (theme-aware):
//	styles := theme.GetCurrentStyles()
//	fmt.Print(styles.Success.Render(theme.IconCheckmark))
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
	Checkmark:     lipgloss.NewStyle().Foreground(lipgloss.Color(ColorCheckmark)).SetString(IconCheckmark),
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

// Colors - DEPRECATED: Use theme.GetCurrentStyles() instead.
// This global variable provides legacy fatih/color styling which doesn't support theme switching.
// New code should call GetCurrentStyles() to get theme-aware lipgloss styles.
//
// Example migration:
//
//	// Old (deprecated):
//	Colors.Success.Println("Operation complete")
//
//	// New (theme-aware):
//	styles := theme.GetCurrentStyles()
//	fmt.Println(styles.Success.Render("Operation complete"))
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
