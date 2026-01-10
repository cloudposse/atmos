package ui

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// DefaultTerminalWidth is the fallback width when terminal size cannot be determined.
	defaultTerminalWidth = 120
	// TreeIndentWidth is the approximate width of tree prefix and symbols.
	treeIndentWidth = 20
)

// getRawStringValue returns the raw string content and whether it's multi-line.
func getRawStringValue(v interface{}, sensitive bool) (string, bool) {
	if sensitive {
		return "(sensitive)", false
	}
	if v == nil {
		return "(none)", false
	}
	if s, ok := v.(string); ok {
		isMultiline := strings.Contains(s, "\n")
		return s, isMultiline
	}
	// Non-string values are not multi-line.
	return "", false
}

// formatSimpleValue formats a non-multi-line value for display.
func formatSimpleValue(v interface{}, sensitive bool) string {
	if sensitive {
		return "(sensitive)"
	}
	if v == nil {
		return "(none)"
	}
	switch val := v.(type) {
	case string:
		// Single-line string - truncate if needed.
		const maxWidth = 40
		if len(val) > maxWidth-2 {
			return fmt.Sprintf("\"%s...\"", val[:maxWidth-5])
		}
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case map[string]interface{}, []interface{}:
		const maxWidth = 40
		jsonBytes, err := json.Marshal(val)
		if err != nil {
			return "(complex)"
		}
		s := string(jsonBytes)
		if len(s) > maxWidth {
			return s[:maxWidth-3] + "..."
		}
		return s
	default:
		return fmt.Sprintf("%v", val)
	}
}

// valuesEqual compares two interface values for equality.
func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Use JSON encoding for deep comparison of complex types.
	aJSON, errA := json.Marshal(a)
	bJSON, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}

// getMaxLineWidth returns the maximum width for content lines based on terminal width.
func getMaxLineWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		width = defaultTerminalWidth
	}
	// Subtract space for tree indent, symbols, and some margin.
	maxWidth := width - treeIndentWidth
	if maxWidth < 40 {
		maxWidth = 40 // Minimum reasonable width.
	}
	return maxWidth
}

// getContrastTextColor returns black or white text color based on background luminance.
// Uses WCAG relative luminance formula for accessibility.
func getContrastTextColor(bgColor string) string {
	// Parse hex color (handles both #RRGGBB and RRGGBB formats).
	hexColor := bgColor
	if len(hexColor) > 0 && hexColor[0] == '#' {
		hexColor = hexColor[1:]
	}

	// Default to white if parsing fails.
	if len(hexColor) != 6 {
		return theme.ColorWhite
	}

	// Parse RGB components.
	r, err1 := parseHexComponent(hexColor[0:2])
	g, err2 := parseHexComponent(hexColor[2:4])
	b, err3 := parseHexComponent(hexColor[4:6])

	if err1 != nil || err2 != nil || err3 != nil {
		return theme.ColorWhite // Default to white on parse error.
	}

	// Convert to 0-1 range and apply gamma correction.
	toLinear := func(c int64) float64 {
		v := float64(c) / 255.0
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}

	rLinear := toLinear(r)
	gLinear := toLinear(g)
	bLinear := toLinear(b)

	// Calculate relative luminance (WCAG formula).
	luminance := 0.2126*rLinear + 0.7152*gLinear + 0.0722*bLinear

	// Use black text for light backgrounds (luminance > 0.5), white for dark backgrounds.
	if luminance > 0.5 {
		return theme.ColorBlack // Black text for light backgrounds.
	}
	return theme.ColorWhite // White text for dark backgrounds.
}

// parseHexComponent parses a 2-character hex string to int64.
func parseHexComponent(hex string) (int64, error) {
	var result int64
	for _, c := range hex {
		result *= 16
		switch {
		case c >= '0' && c <= '9':
			result += int64(c - '0')
		case c >= 'a' && c <= 'f':
			result += int64(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			result += int64(c - 'A' + 10)
		default:
			return 0, fmt.Errorf("%w: invalid hex character: %c", errUtils.ErrParseHexColor, c)
		}
	}
	return result, nil
}
