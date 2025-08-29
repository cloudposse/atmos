package theme

import "strings"

// ColorScheme defines semantic color mappings for UI elements.
// These are derived from a Theme's ANSI colors.
type ColorScheme struct {
	// Core semantic colors
	Primary   string // Main brand/action color (typically cyan or blue)
	Secondary string // Supporting actions (typically magenta)
	Success   string // Success states (typically green)
	Warning   string // Warning states (typically yellow)
	Error     string // Error states (typically red)

	// Text colors
	TextPrimary   string // Main text (typically white/brightWhite)
	TextSecondary string // Subtle/secondary text (typically brightBlack)
	TextMuted     string // Disabled/muted text (typically black)
	TextInverse   string // Text on dark backgrounds
	TextLight     string // Light theme indicator (white)

	// UI elements
	Border          string // Borders and dividers (typically blue or brightBlack)
	Background      string // Background colors
	BackgroundDark  string // Dark background for status bars
	Surface         string // Card/panel backgrounds

	// Semantic elements
	Link      string // Links (typically cyan)
	Selected  string // Selected items (typically brightGreen)
	Highlight string // Highlighted text (typically magenta)
	Gold      string // Special indicators (typically yellow or brightYellow)

	// Table specific
	HeaderText string // Table header text (typically brightCyan or green)
	RowText    string // Table row text (typically white)
	RowAlt     string // Alternating row background
	
	// Help/Documentation specific
	BackgroundHighlight string // Background for highlighted sections (usage/example blocks)
	
	// Syntax highlighting
	ChromaTheme string // Chroma theme name for syntax highlighting
}

// GenerateColorScheme creates a semantic color scheme from a Theme.
// This maps the 16 ANSI colors to semantic UI purposes.
func GenerateColorScheme(t *Theme) ColorScheme {
	// Default to light text on dark background
	textPrimary := t.White
	textSecondary := t.BrightBlack
	
	// For light themes, invert text colors
	if !t.Meta.IsDark {
		textPrimary = t.Black
		textSecondary = t.BrightBlack
	}
	
	return ColorScheme{
		// Core semantic colors - map ANSI colors to purposes
		Primary:   t.Blue,        // Blue for primary actions (commands, headings)
		Secondary: t.Magenta,     // Magenta for secondary
		Success:   t.Green,       // Green for success
		Warning:   t.Yellow,      // Yellow for warnings
		Error:     t.Red,         // Red for errors

		// Text colors
		TextPrimary:   textPrimary,
		TextSecondary: textSecondary,
		TextMuted:     t.BrightBlack,
		TextInverse:   t.Background,
		TextLight:     t.White, // Always white for "Light" theme type indicator

		// UI elements
		Border:         t.Blue,         // Blue for borders
		Background:     t.Background,   // Theme background
		BackgroundDark: t.BrightBlack,  // Dark background for status bars
		Surface:        t.BrightBlack,  // Slightly elevated surface

		// Semantic elements
		Link:      t.BrightBlue,   // Bright blue for links
		Selected:  t.BrightGreen,  // Bright green for selected
		Highlight: t.BrightMagenta, // Bright magenta for highlights
		Gold:      t.BrightYellow, // Bright yellow for special indicators

		// Table specific
		HeaderText: t.Green,       // Green for headers
		RowText:    textPrimary,   // Same as primary text
		RowAlt:     t.BrightBlack, // Subtle alternating rows
		
		// Help/Documentation specific
		BackgroundHighlight: t.Black, // Dark background for code blocks
		
		// Syntax highlighting - map themes to appropriate Chroma themes
		ChromaTheme: getChromaThemeForAtmosTheme(t),
	}
}

// getChromaThemeForAtmosTheme returns an appropriate Chroma syntax highlighting theme
// based on the Atmos theme characteristics.
func getChromaThemeForAtmosTheme(t *Theme) string {
	// Map specific themes to their best Chroma equivalents
	switch strings.ToLower(t.Name) {
	case "dracula":
		return "dracula"
	case "monokai":
		return "monokai"
	case "github-dark":
		return "github-dark"
	case "nord":
		return "nord"
	case "solarized-dark":
		return "solarized-dark"
	case "solarized-light":
		return "solarized-light"
	case "github-light":
		return "github"
	case "tokyo-night":
		return "onedark"
	case "gruvbox":
		return "gruvbox"
	case "catppuccin":
		return "catppuccin-mocha"
	case "one-dark":
		return "onedark"
	case "material":
		return "material"
	default:
		// For unknown themes, choose based on dark/light
		if t.Meta.IsDark {
			return "dracula" // Good default dark theme
		}
		return "github" // Good default light theme
	}
}

// GetColorSchemeForTheme loads a theme by name and generates its color scheme.
func GetColorSchemeForTheme(themeName string) (*ColorScheme, error) {
	registry, err := NewRegistry()
	if err != nil {
		return nil, err
	}
	
	theme := registry.GetOrDefault(themeName)
	scheme := GenerateColorScheme(theme)
	return &scheme, nil
}