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
	Border         string // Borders and dividers (typically blue or brightBlack)
	Background     string // Background colors
	BackgroundDark string // Dark background for status bars
	Surface        string // Card/panel backgrounds

	// Semantic elements
	Link      string // Links (typically cyan)
	Selected  string // Selected items (typically brightGreen)
	Highlight string // Highlighted text (typically magenta)
	Gold      string // Special indicators (typically yellow or brightYellow)
	Spinner   string // Loading/progress indicators (typically cyan)

	// Table specific
	HeaderText string // Table header text (typically brightCyan or green)
	RowText    string // Table row text (typically white)
	RowAlt     string // Alternating row background

	// Help/Documentation specific
	BackgroundHighlight string // Background for highlighted sections (usage/example blocks)

	// Interactive prompts (Huh library)
	ButtonForeground string // Button text color (light/cream)
	ButtonBackground string // Button background color (primary/purple)

	// Syntax highlighting
	ChromaTheme string // Chroma theme name for syntax highlighting

	// Log level colors (backgrounds)
	LogDebug   string // Debug log level background
	LogInfo    string // Info log level background
	LogWarning string // Warning log level background
	LogError   string // Error log level background
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
		Primary:   t.Blue,    // Blue for primary actions (commands, headings)
		Secondary: t.Magenta, // Magenta for secondary
		Success:   t.Green,   // Green for success
		Warning:   t.Yellow,  // Yellow for warnings
		Error:     t.Red,     // Red for errors

		// Text colors
		TextPrimary:   textPrimary,
		TextSecondary: textSecondary,
		TextMuted:     t.BrightBlack,
		TextInverse:   t.Background,
		TextLight:     t.White, // Always white for "Light" theme type indicator

		// UI elements
		Border:         t.Blue,        // Blue for borders
		Background:     t.Background,  // Theme background
		BackgroundDark: t.BrightBlack, // Dark background for status bars
		Surface:        t.BrightBlack, // Slightly elevated surface

		// Semantic elements
		Link:      t.BrightBlue,    // Bright blue for links
		Selected:  t.BrightGreen,   // Bright green for selected
		Highlight: t.BrightMagenta, // Bright magenta for highlights
		Gold:      t.BrightYellow,  // Bright yellow for special indicators
		Spinner:   t.Cyan,          // Cyan for loading/progress (calming, indicates activity)

		// Table specific
		HeaderText: t.Green,       // Green for headers
		RowText:    textPrimary,   // Same as primary text
		RowAlt:     t.BrightBlack, // Subtle alternating rows

		// Help/Documentation specific
		BackgroundHighlight: t.Black, // Dark background for code blocks

		// Interactive prompts (Huh library)
		ButtonForeground: t.BrightWhite, // Light cream text
		ButtonBackground: t.BrightBlue,  // Primary action color (purple/blue)

		// Syntax highlighting - map themes to appropriate Chroma themes
		ChromaTheme: getChromaThemeForAtmosTheme(t),

		// Log level colors - use standard colors as backgrounds
		LogDebug:   t.Cyan,   // Cyan for debug
		LogInfo:    t.Blue,   // Blue for info
		LogWarning: t.Yellow, // Yellow for warning
		LogError:   t.Red,    // Red for error
	}
}

// chromaThemeMap maps Atmos theme names to Chroma syntax highlighting themes.
var chromaThemeMap = map[string]string{
	"dracula":         "dracula",
	"monokai":         "monokai",
	"github-dark":     "github-dark",
	"nord":            "nord",
	"solarized-dark":  "solarized-dark",
	"solarized-light": "solarized-light",
	"github-light":    "github",
	"tokyo-night":     "onedark",
	"gruvbox":         "gruvbox",
	"catppuccin":      "catppuccin-mocha",
	"one-dark":        "onedark",
	"material":        "material",
}

// getChromaThemeForAtmosTheme returns an appropriate Chroma syntax highlighting theme
// based on the Atmos theme characteristics.
func getChromaThemeForAtmosTheme(t *Theme) string {
	// Try to find a mapped theme
	if chromaTheme, ok := chromaThemeMap[strings.ToLower(t.Name)]; ok {
		return chromaTheme
	}

	// For unknown themes, choose based on dark/light
	if t.Meta.IsDark {
		return "dracula" // Good default dark theme
	}
	return "github" // Good default light theme
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
