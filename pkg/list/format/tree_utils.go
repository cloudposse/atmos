package format

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// getBranchStyle returns the style for tree branches.
// Uses Muted style from the current theme for subtle branch lines.
func getBranchStyle() lipgloss.Style {
	styles := theme.GetCurrentStyles()
	return styles.Muted
}

// getStackStyle returns the style for stack names.
// Uses Body style from the current theme for primary text.
func getStackStyle() lipgloss.Style {
	styles := theme.GetCurrentStyles()
	return styles.Body
}

// getComponentStyle returns the style for component names.
// Uses Command style from the current theme for component names.
func getComponentStyle() lipgloss.Style {
	styles := theme.GetCurrentStyles()
	return styles.Command
}

// getImportStyle returns the style for import paths.
// Uses Info style from the current theme for import file paths.
func getImportStyle() lipgloss.Style {
	styles := theme.GetCurrentStyles()
	return styles.Info
}

// getCircularStyle returns the style for circular reference markers.
// Uses Error style from the current theme for warnings about circular imports.
func getCircularStyle() lipgloss.Style {
	styles := theme.GetCurrentStyles()
	return styles.Error
}

// stripANSI removes ANSI escape codes from a string for text processing.
func stripANSI(s string) string {
	result := ""
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result += string(r)
	}
	return result
}
