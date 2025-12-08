package theme

import (
	"math"
	"strconv"

	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
)

// WCAG sRGB gamma correction constants.
const (
	rgbMaxValue          = 255.0
	srgbThreshold        = 0.03928
	srgbGammaDivisor     = 12.92
	srgbGammaOffset      = 0.055
	srgbGammaDenominator = 1.055
	srgbGammaExponent    = 2.4
	// WCAG relative luminance weights (more accurate than simple RGB weights).
	wcagLuminanceRedWeight   = 0.2126
	wcagLuminanceGreenWeight = 0.7152
	wcagLuminanceBlueWeight  = 0.0722
)

// getContrastTextColor returns black or white text color based on background luminance.
// Uses WCAG relative luminance formula for accessibility.
func getContrastTextColor(bgColor string) string {
	// Parse hex color (handles both #RRGGBB and RRGGBB formats).
	hexColor := bgColor
	if len(hexColor) > 0 && hexColor[0] == '#' {
		hexColor = hexColor[1:]
	}

	// Default to white if parsing fails.
	if len(hexColor) != hexColorLength {
		return "#FFFFFF"
	}

	// Parse RGB components.
	r, err1 := strconv.ParseInt(hexColor[0:2], hexBase, intBitSize)
	g, err2 := strconv.ParseInt(hexColor[2:4], hexBase, intBitSize)
	b, err3 := strconv.ParseInt(hexColor[4:6], hexBase, intBitSize)

	if err1 != nil || err2 != nil || err3 != nil {
		return "#FFFFFF" // Default to white on parse error.
	}

	// Convert to 0-1 range and apply gamma correction.
	toLinear := func(c int64) float64 {
		v := float64(c) / rgbMaxValue
		if v <= srgbThreshold {
			return v / srgbGammaDivisor
		}
		return math.Pow((v+srgbGammaOffset)/srgbGammaDenominator, srgbGammaExponent)
	}

	rLinear := toLinear(r)
	gLinear := toLinear(g)
	bLinear := toLinear(b)

	// Calculate relative luminance (WCAG formula).
	luminance := wcagLuminanceRedWeight*rLinear + wcagLuminanceGreenWeight*gLinear + wcagLuminanceBlueWeight*bLinear

	// Use black text for light backgrounds (luminance > 0.5), white for dark backgrounds.
	if luminance > luminanceThreshold {
		return "#000000" // Black text for light backgrounds.
	}
	return "#FFFFFF" // White text for dark backgrounds.
}

// createLogLevelStyles creates log level styles with badge-like appearance.
// Text color automatically contrasts with background for readability.
func createLogLevelStyles(scheme *ColorScheme) map[log.Level]lipgloss.Style {
	levels := make(map[log.Level]lipgloss.Style)

	// Using 4-character log level format for consistent alignment.
	// Text color automatically chosen for contrast with background.
	levels[log.DebugLevel] = lipgloss.NewStyle().
		SetString("DEBU").
		Background(lipgloss.Color(scheme.LogDebug)).
		Foreground(lipgloss.Color(getContrastTextColor(scheme.LogDebug))).
		Bold(true).
		Padding(0, 1) // 1 space padding left and right

	levels[log.InfoLevel] = lipgloss.NewStyle().
		SetString("INFO").
		Background(lipgloss.Color(scheme.LogInfo)).
		Foreground(lipgloss.Color(getContrastTextColor(scheme.LogInfo))).
		Bold(true).
		Padding(0, 1)

	levels[log.WarnLevel] = lipgloss.NewStyle().
		SetString("WARN").
		Background(lipgloss.Color(scheme.LogWarning)).
		Foreground(lipgloss.Color(getContrastTextColor(scheme.LogWarning))).
		Bold(true).
		Padding(0, 1)

	levels[log.ErrorLevel] = lipgloss.NewStyle().
		SetString("ERRO").
		Background(lipgloss.Color(scheme.LogError)).
		Foreground(lipgloss.Color(getContrastTextColor(scheme.LogError))).
		Bold(true).
		Padding(0, 1)

	levels[log.FatalLevel] = lipgloss.NewStyle().
		SetString("FATA").
		Background(lipgloss.Color(scheme.LogError)). // Use error color for fatal.
		Foreground(lipgloss.Color(getContrastTextColor(scheme.LogError))).
		Bold(true).
		Padding(0, 1)

	return levels
}

// GetLogStyles returns charm/log styles configured with the current theme colors.
// This includes background colors for log level badges and key-value pair styling.
func GetLogStyles(scheme *ColorScheme) *log.Styles {
	if scheme == nil {
		// Return default styles if no theme colors are available
		return log.DefaultStyles()
	}

	styles := &log.Styles{
		Levels: createLogLevelStyles(scheme),
		Keys:   make(map[string]lipgloss.Style),
		Values: make(map[string]lipgloss.Style),
	}

	// Configure key-value pair colors
	// Keys use the theme's muted color for subtle emphasis
	styles.Key = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextMuted)).
		Bold(false)

	// Values use the theme's primary color for visibility
	styles.Value = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.Primary))

	// Set timestamp style using muted color
	styles.Timestamp = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextMuted)).
		Faint(true)

	// Set message style
	styles.Message = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextPrimary))

	// Set prefix and separator styles
	styles.Prefix = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextMuted)).
		Bold(true)

	styles.Separator = lipgloss.NewStyle().
		Foreground(lipgloss.Color(scheme.TextMuted)).
		Faint(true)

	return styles
}

// GetLogStylesNoColor returns charm/log styles with no colors for --no-color mode.
func GetLogStylesNoColor() *log.Styles {
	styles := &log.Styles{
		Levels: make(map[log.Level]lipgloss.Style),
	}

	// Set log level strings without colors for no-color mode.
	// Using 4-character log level format for consistent alignment.
	styles.Levels[log.DebugLevel] = lipgloss.NewStyle().SetString("DEBU")
	styles.Levels[log.InfoLevel] = lipgloss.NewStyle().SetString("INFO")
	styles.Levels[log.WarnLevel] = lipgloss.NewStyle().SetString("WARN")
	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().SetString("ERRO")
	styles.Levels[log.FatalLevel] = lipgloss.NewStyle().SetString("FATA")

	// Clear other style elements
	styles.Timestamp = lipgloss.NewStyle()
	styles.Message = lipgloss.NewStyle()
	styles.Key = lipgloss.NewStyle()
	styles.Value = lipgloss.NewStyle()
	styles.Prefix = lipgloss.NewStyle()
	styles.Separator = lipgloss.NewStyle()

	return styles
}
