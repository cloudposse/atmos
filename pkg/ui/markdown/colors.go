package markdown

import "github.com/charmbracelet/lipgloss"

// Base colors used throughout the application.
var (
	White       = "#FFFFFF"
	Purple      = "#9B51E0"
	Blue        = "#00A3E0"
	Gray        = "#4A5568"
	Green       = "#48BB78"
	Yellow      = "#ECC94B"
	Red         = "#F56565"
	LightBlue   = "#4299E1"
	BlueLight   = "#63B3ED"
	GrayLight   = "#718096"
	GrayMid     = "#A0AEC0"
	GrayDark    = "#2D3748"
	GrayBorder  = "#CBD5E0"
	OffWhite    = "#F7FAFC"
	DarkSlate   = "#1A202C"
	GreenLight  = "#68D391"
	YellowLight = "#F6E05E"
	RedLight    = "#FC8181"
)

// Colors defines the color scheme for markdown rendering.
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
	Primary:    lipgloss.AdaptiveColor{Light: Blue, Dark: Blue},
	Secondary:  lipgloss.AdaptiveColor{Light: Gray, Dark: GrayMid},
	Success:    lipgloss.AdaptiveColor{Light: Green, Dark: GreenLight},
	Warning:    lipgloss.AdaptiveColor{Light: Yellow, Dark: YellowLight},
	Error:      lipgloss.AdaptiveColor{Light: Red, Dark: RedLight},
	Info:       lipgloss.AdaptiveColor{Light: LightBlue, Dark: BlueLight},
	Subtle:     lipgloss.AdaptiveColor{Light: GrayLight, Dark: GrayMid},
	HeaderBg:   lipgloss.AdaptiveColor{Light: GrayDark, Dark: Gray},
	Border:     lipgloss.AdaptiveColor{Light: GrayBorder, Dark: Gray},
	Background: lipgloss.AdaptiveColor{Light: OffWhite, Dark: DarkSlate},
}
