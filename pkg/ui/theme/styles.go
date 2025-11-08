package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"
)

// StyleSet provides pre-configured lipgloss styles for common UI elements.
type StyleSet struct {
	// Text styles
	Title   lipgloss.Style
	Heading lipgloss.Style
	Body    lipgloss.Style
	Muted   lipgloss.Style

	// Status styles
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Info    lipgloss.Style
	Debug   lipgloss.Style
	Trace   lipgloss.Style

	// UI element styles
	Selected    lipgloss.Style
	Link        lipgloss.Style
	Command     lipgloss.Style
	Description lipgloss.Style
	Label       lipgloss.Style // Section labels/headers (non-status)

	// Table styles
	TableHeader    lipgloss.Style
	TableRow       lipgloss.Style
	TableActive    lipgloss.Style
	TableBorder    lipgloss.Style
	TableSpecial   lipgloss.Style // For special indicators like stars
	TableDarkType  lipgloss.Style // For "Dark" theme type
	TableLightType lipgloss.Style // For "Light" theme type

	// Special elements
	Checkmark lipgloss.Style
	XMark     lipgloss.Style
	Footer    lipgloss.Style
	Border    lipgloss.Style

	// Version styles
	VersionNumber lipgloss.Style
	NewVersion    lipgloss.Style
	PackageName   lipgloss.Style

	// Pager styles
	Pager struct {
		StatusBar        lipgloss.Style
		StatusBarHelp    lipgloss.Style
		StatusBarMessage lipgloss.Style
		ErrorMessage     lipgloss.Style
		Highlight        lipgloss.Style
		HelpView         lipgloss.Style
	}

	// TUI component styles
	TUI struct {
		ItemStyle         lipgloss.Style
		SelectedItemStyle lipgloss.Style
		BorderFocused     lipgloss.Style
		BorderUnfocused   lipgloss.Style
	}

	// Diff/Output styles
	Diff struct {
		Added   lipgloss.Style
		Removed lipgloss.Style
		Changed lipgloss.Style
		Header  lipgloss.Style
	}

	// Help/Documentation styles
	Help struct {
		Heading      lipgloss.Style // Section headings (uppercase)
		CommandName  lipgloss.Style // Command names in lists
		CommandDesc  lipgloss.Style // Command descriptions
		FlagName     lipgloss.Style // Flag names
		FlagDesc     lipgloss.Style // Flag descriptions
		FlagDataType lipgloss.Style // Flag data types
		UsageBlock   lipgloss.Style // Styled box for usage examples
		ExampleBlock lipgloss.Style // Styled box for code examples
		Code         lipgloss.Style // Inline code elements
	}
}

// GetStyles generates styles from a color scheme.
//
//revive:disable:function-length
//nolint:funlen // Comprehensive style initialization better kept together.
func GetStyles(scheme *ColorScheme) *StyleSet {
	if scheme == nil {
		return nil
	}
	return &StyleSet{
		// Text styles
		Title:   lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Primary)).Bold(true),
		Heading: lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Primary)),
		Body:    lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.TextPrimary)),
		Muted:   lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.TextMuted)),

		// Status styles
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Success)),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Warning)),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Error)),
		Info:    lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Link)),
		Debug:   lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.TextMuted)),
		Trace:   lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.TextMuted)).Faint(true),

		// UI element styles
		Selected:    lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Selected)),
		Link:        lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Link)),
		Command:     lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Primary)),
		Description: lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.TextPrimary)),
		Label:       lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Primary)).Bold(true),

		// Table styles
		TableHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color(scheme.HeaderText)).
			Bold(true).
			Align(lipgloss.Center),
		TableRow: lipgloss.NewStyle().
			Foreground(lipgloss.Color(scheme.RowText)),
		TableActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(scheme.Selected)).
			Bold(true),
		TableBorder: lipgloss.NewStyle().
			Foreground(lipgloss.Color(scheme.Border)),
		TableSpecial: lipgloss.NewStyle().
			Foreground(lipgloss.Color(scheme.Gold)),
		TableDarkType: lipgloss.NewStyle().
			Foreground(lipgloss.Color(scheme.TextMuted)),
		TableLightType: lipgloss.NewStyle().
			Foreground(lipgloss.Color(scheme.TextLight)),

		// Special elements
		Checkmark: lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Success)).SetString("✓"),
		XMark:     lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Error)).SetString("✗"),
		Footer:    lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.TextMuted)).Italic(true),
		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(scheme.Border)),

		// Version styles
		VersionNumber: lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.TextMuted)),
		NewVersion:    lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Success)),
		PackageName:   lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Secondary)),

		// Pager styles
		Pager: struct {
			StatusBar        lipgloss.Style
			StatusBarHelp    lipgloss.Style
			StatusBarMessage lipgloss.Style
			ErrorMessage     lipgloss.Style
			Highlight        lipgloss.Style
			HelpView         lipgloss.Style
		}{
			StatusBar: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.TextMuted)).
				Background(lipgloss.Color(scheme.BackgroundDark)),
			StatusBarHelp: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.TextMuted)).
				Background(lipgloss.Color(scheme.BackgroundHighlight)),
			StatusBarMessage: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.Success)).
				Background(lipgloss.Color(scheme.BackgroundDark)),
			ErrorMessage: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.Error)).
				Background(lipgloss.Color(scheme.BackgroundDark)),
			Highlight: lipgloss.NewStyle().
				Background(lipgloss.Color(scheme.Warning)).
				Foreground(lipgloss.Color(scheme.TextInverse)).
				Bold(true),
			HelpView: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.TextMuted)).
				Background(lipgloss.Color(scheme.BackgroundHighlight)),
		},

		// TUI component styles
		TUI: struct {
			ItemStyle         lipgloss.Style
			SelectedItemStyle lipgloss.Style
			BorderFocused     lipgloss.Style
			BorderUnfocused   lipgloss.Style
		}{
			ItemStyle:         lipgloss.NewStyle().PaddingLeft(4),
			SelectedItemStyle: lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color(scheme.Selected)),
			BorderFocused: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(scheme.Border)),
			BorderUnfocused: lipgloss.NewStyle().
				Border(lipgloss.HiddenBorder()),
		},

		// Diff/Output styles
		Diff: struct {
			Added   lipgloss.Style
			Removed lipgloss.Style
			Changed lipgloss.Style
			Header  lipgloss.Style
		}{
			Added:   lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Success)),
			Removed: lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Error)),
			Changed: lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Warning)),
			Header:  lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Primary)).Bold(true),
		},

		// Help/Documentation styles
		Help: struct {
			Heading      lipgloss.Style
			CommandName  lipgloss.Style
			CommandDesc  lipgloss.Style
			FlagName     lipgloss.Style
			FlagDesc     lipgloss.Style
			FlagDataType lipgloss.Style
			UsageBlock   lipgloss.Style
			ExampleBlock lipgloss.Style
			Code         lipgloss.Style
		}{
			Heading: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.Primary)).
				Bold(true).
				Transform(func(s string) string {
					return strings.ToUpper(strings.ReplaceAll(s, "_", " "))
				}),
			CommandName: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.Primary)).
				Bold(true),
			CommandDesc: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.TextSecondary)),
			FlagName: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.TextSecondary)), // Lighter color for flag names
			FlagDesc: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.TextPrimary)),
			FlagDataType: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.TextMuted)), // Darker color for flag types (no faint)
			UsageBlock: lipgloss.NewStyle().
				Background(lipgloss.Color(scheme.BackgroundHighlight)).
				Padding(1, 2).
				Margin(1, 0),
			ExampleBlock: lipgloss.NewStyle().
				Background(lipgloss.Color(scheme.BackgroundHighlight)).
				Padding(1, 2).
				Margin(1, 0).
				Foreground(lipgloss.Color(scheme.Primary)),
			Code: lipgloss.NewStyle().
				Foreground(lipgloss.Color(scheme.Secondary)), // Purple for consistency with markdown
		},
	}
}

// CurrentStyles holds the active styles for the application.
var CurrentStyles *StyleSet

// currentThemeName tracks the currently loaded theme to avoid reloading.
var currentThemeName string

// lastColorScheme caches the last-used color scheme to avoid redundant unmarshalling in color getters.
var lastColorScheme *ColorScheme

// GetCurrentStyles returns the current active styles based on the configured theme.
// It loads the theme from configuration or environment variable.
func GetCurrentStyles() *StyleSet {
	// Determine the theme name from configuration or environment
	themeName := getActiveThemeName()

	// If the theme hasn't changed and we already have styles, return them
	if CurrentStyles != nil && currentThemeName == themeName {
		return CurrentStyles
	}

	// Load the new theme and generate styles
	scheme, err := GetColorSchemeForTheme(themeName)
	if err != nil {
		// Fall back to atmos theme if there's an error
		registry, _ := NewRegistry()
		if registry != nil {
			defaultTheme := registry.GetOrDefault("atmos")
			tmpScheme := GenerateColorScheme(defaultTheme)
			scheme = &tmpScheme
		}
	}

	CurrentStyles = GetStyles(scheme)
	currentThemeName = themeName
	lastColorScheme = scheme // Cache the color scheme for color getters
	return CurrentStyles
}

// InitializeStyles initializes the styles with a specific color scheme.
// Note: Does not clear currentThemeName to retain manually-passed scheme.
func InitializeStyles(scheme *ColorScheme) {
	CurrentStyles = GetStyles(scheme)
	lastColorScheme = scheme // Cache the color scheme for color getters
}

// InitializeStylesFromTheme initializes the global CurrentStyles from the specified theme name.
// The themeName parameter specifies which color scheme to load (e.g., "atmos", "dracula").
// Returns an error if the theme cannot be found or loaded.
func InitializeStylesFromTheme(themeName string) error {
	scheme, err := GetColorSchemeForTheme(themeName)
	if err != nil {
		return err
	}
	CurrentStyles = GetStyles(scheme)
	currentThemeName = themeName
	lastColorScheme = scheme // Cache the color scheme for color getters
	return nil
}

// getActiveThemeName determines the active theme name from configuration or environment.
func getActiveThemeName() string {
	// Bind environment variables on demand to ensure they're available
	// This handles both ATMOS_THEME and THEME as fallbacks
	_ = viper.BindEnv("settings.terminal.theme", "ATMOS_THEME", "THEME")

	// Check Viper configuration which now includes bound environment variables
	if viper.IsSet("settings.terminal.theme") {
		theme := viper.GetString("settings.terminal.theme")
		if theme != "" {
			return theme
		}
	}

	// Check for ATMOS_THEME environment variable directly as fallback
	if theme := viper.GetString("ATMOS_THEME"); theme != "" {
		return theme
	}

	// Check for THEME environment variable directly as second fallback
	if theme := viper.GetString("THEME"); theme != "" {
		return theme
	}

	// Default to "atmos" theme
	return "atmos"
}

// Helper functions for getting theme-aware colors and styles

// GetSuccessStyle returns the success style from the current theme.
func GetSuccessStyle() lipgloss.Style {
	styles := GetCurrentStyles()
	if styles == nil {
		return lipgloss.NewStyle()
	}
	return styles.Success
}

// GetErrorStyle returns the error style from the current theme.
func GetErrorStyle() lipgloss.Style {
	styles := GetCurrentStyles()
	if styles == nil {
		return lipgloss.NewStyle()
	}
	return styles.Error
}

// GetWarningStyle returns the warning style from the current theme.
func GetWarningStyle() lipgloss.Style {
	styles := GetCurrentStyles()
	if styles == nil {
		return lipgloss.NewStyle()
	}
	return styles.Warning
}

// GetInfoStyle returns the info style from the current theme.
func GetInfoStyle() lipgloss.Style {
	styles := GetCurrentStyles()
	if styles == nil {
		return lipgloss.NewStyle()
	}
	return styles.Info
}

// GetDebugStyle returns the debug style from the current theme.
func GetDebugStyle() lipgloss.Style {
	styles := GetCurrentStyles()
	if styles == nil {
		return lipgloss.NewStyle()
	}
	return styles.Debug
}

// GetTraceStyle returns the trace style from the current theme.
func GetTraceStyle() lipgloss.Style {
	styles := GetCurrentStyles()
	if styles == nil {
		return lipgloss.NewStyle()
	}
	return styles.Trace
}

// GetPrimaryColor returns the primary color from the current theme.
func GetPrimaryColor() string {
	// Use cached color scheme if available
	if lastColorScheme != nil {
		return lastColorScheme.Primary
	}

	// Fall back to loading from theme
	scheme, err := GetColorSchemeForTheme(getActiveThemeName())
	if err != nil || scheme == nil {
		return "#00A3E0" // Default blue
	}
	return scheme.Primary
}

// GetSuccessColor returns the success color from the current theme.
func GetSuccessColor() string {
	// Use cached color scheme if available
	if lastColorScheme != nil {
		return lastColorScheme.Success
	}

	// Fall back to loading from theme
	scheme, err := GetColorSchemeForTheme(getActiveThemeName())
	if err != nil || scheme == nil {
		return "#00FF00" // Default green
	}
	return scheme.Success
}

// GetErrorColor returns the error color from the current theme.
func GetErrorColor() string {
	// Use cached color scheme if available
	if lastColorScheme != nil {
		return lastColorScheme.Error
	}

	// Fall back to loading from theme
	scheme, err := GetColorSchemeForTheme(getActiveThemeName())
	if err != nil || scheme == nil {
		return "#FF0000" // Default red
	}
	return scheme.Error
}

// GetBorderColor returns the border color from the current theme.
func GetBorderColor() string {
	// Use cached color scheme if available
	if lastColorScheme != nil {
		return lastColorScheme.Border
	}

	// Fall back to loading from theme
	scheme, err := GetColorSchemeForTheme(getActiveThemeName())
	if err != nil || scheme == nil {
		return "#5F5FD7" // Default border color
	}
	return scheme.Border
}
