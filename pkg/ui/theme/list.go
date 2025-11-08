package theme

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ListThemesOptions contains options for listing themes.
type ListThemesOptions struct {
	RecommendedOnly bool
	ActiveTheme     string
}

// ListThemesResult contains the formatted output for theme listing.
type ListThemesResult struct {
	Output string
	Error  error
}

// ListThemes generates a formatted list of available themes.
func ListThemes(opts ListThemesOptions) ListThemesResult {
	defer perf.Track(nil, "theme.ListThemes")()

	themes, err := listAvailableThemes()
	if err != nil {
		return ListThemesResult{
			Error: err,
		}
	}

	// Filter for recommended themes if requested.
	if opts.RecommendedOnly {
		themes = filterRecommended(themes, opts.ActiveTheme)
	}

	// When showing all themes, display stars for recommended ones.
	showStars := !opts.RecommendedOnly

	output := displayThemeList(themes, opts.ActiveTheme, opts.RecommendedOnly, showStars)

	return ListThemesResult{
		Output: output,
		Error:  nil,
	}
}

// listAvailableThemes retrieves all available themes.
func listAvailableThemes() ([]*Theme, error) {
	defer perf.Track(nil, "theme.listAvailableThemes")()

	registry, err := NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load theme registry: %w", err)
	}

	return registry.List(), nil
}

// filterRecommended returns only recommended themes, but ensures the active theme is included.
func filterRecommended(themes []*Theme, activeTheme string) []*Theme {
	defer perf.Track(nil, "theme.filterRecommended")()

	var recommended []*Theme
	hasActiveTheme := false

	for _, t := range themes {
		if IsRecommended(t.Name) {
			recommended = append(recommended, t)
			if strings.EqualFold(t.Name, activeTheme) {
				hasActiveTheme = true
			}
		}
	}

	// If the active theme is not in the recommended list, add it.
	if activeTheme != "" && !hasActiveTheme {
		for _, t := range themes {
			if strings.EqualFold(t.Name, activeTheme) {
				recommended = append(recommended, t)
				break
			}
		}
	}

	// Sort the themes by name for consistent output.
	sort.Slice(recommended, func(i, j int) bool {
		return recommended[i].Name < recommended[j].Name
	})

	return recommended
}

// displayThemeList formats and displays the themes to the terminal.
func displayThemeList(themes []*Theme, activeTheme string, showingRecommendedOnly bool, showStars bool) string {
	defer perf.Track(nil, "theme.displayThemeList")()

	// Check if we're in TTY mode.
	// Note: theme list output goes to stderr via ui.Write(), so check stderr TTY support.
	if !term.IsTTYSupportForStderr() {
		// Fall back to simple text output for non-TTY.
		return formatSimpleThemeList(themes, activeTheme, showingRecommendedOnly, showStars)
	}

	return formatThemeTable(themes, activeTheme, showingRecommendedOnly, showStars)
}

// buildThemeRows converts themes to table rows for display.
func buildThemeRows(themes []*Theme, activeTheme string, showStars bool) [][]string {
	defer perf.Track(nil, "theme.buildThemeRows")()

	var rows [][]string
	for _, t := range themes {
		row := formatThemeRow(t, activeTheme, showStars)
		rows = append(rows, row)
	}
	return rows
}

// formatThemeRow formats a single theme as a table row.
func formatThemeRow(t *Theme, activeTheme string, showStars bool) []string {
	defer perf.Track(nil, "theme.formatThemeRow")()

	// Active indicator.
	activeIndicator := "  "
	if strings.EqualFold(t.Name, activeTheme) {
		activeIndicator = "> "
	}

	// Theme name with recommended indicator (only show star when requested).
	name := t.Name
	if showStars && IsRecommended(t.Name) {
		name += " ★"
	}

	// Theme type (Dark/Light).
	themeType := getThemeTypeString(t)

	// Color palette - show the main 8 ANSI colors as colored blocks.
	palette := formatColorPalette(t)

	// Source.
	source := getThemeSourceString(t)
	const maxSourceLen = 50
	const sourceEllipsisLen = 47
	if len(source) > maxSourceLen {
		source = source[:sourceEllipsisLen] + "..."
	}

	return []string{
		activeIndicator,
		name,
		themeType,
		palette,
		source,
	}
}

// buildFooterMessage creates the footer text for the theme list.
func buildFooterMessage(themeCount int, showingRecommendedOnly bool, showStars bool, activeTheme string) string {
	defer perf.Track(nil, "theme.buildFooterMessage")()

	footer := fmt.Sprintf("\n%d theme", themeCount)
	if themeCount != 1 {
		footer += "s"
	}

	if showingRecommendedOnly {
		footer += " (recommended). Use without --recommended to see all themes."
	} else {
		footer += " available."
		if showStars {
			footer += " ★ indicates recommended themes."
		}
	}

	if activeTheme != "" {
		footer += fmt.Sprintf("\nActive theme: %s", activeTheme)
	}

	return footer
}

// formatThemeTable formats themes into a styled Charmbracelet table.
func formatThemeTable(themes []*Theme, activeTheme string, showingRecommendedOnly bool, showStars bool) string {
	defer perf.Track(nil, "theme.formatThemeTable")()

	// Prepare headers and rows.
	headers := []string{"", "Name", "Type", "Palette", "Source"}
	rows := buildThemeRows(themes, activeTheme, showStars)

	// Use the new themed table creation.
	output := CreateThemedTable(headers, rows) + "\n"

	// Footer message.
	styles := GetCurrentStyles()
	footer := buildFooterMessage(len(themes), showingRecommendedOnly, showStars, activeTheme)
	output += styles.Footer.Render(footer) + "\n"

	return output
}

// formatSimpleThemeList formats themes as simple text for non-TTY output.
func formatSimpleThemeList(themes []*Theme, activeTheme string, showingRecommendedOnly bool, showStars bool) string {
	defer perf.Track(nil, "theme.formatSimpleThemeList")()

	var output string
	const lineWidth = 80

	// Header.
	output += fmt.Sprintf("   %-30s %-8s %-4s %-10s %s\n", "Name", "Type", "Rec", "Palette", "Source")
	output += fmt.Sprintf("%s\n", strings.Repeat("=", lineWidth))

	// Theme rows.
	for _, t := range themes {
		activeIndicator := "  "
		if strings.EqualFold(t.Name, activeTheme) {
			activeIndicator = "> "
		}

		recommended := ""
		if showStars && IsRecommended(t.Name) {
			recommended = "★"
		}

		themeType := getThemeTypeString(t)
		source := getThemeSourceString(t)

		// For non-TTY, just show "8 colors" as a placeholder.
		palette := "8 colors"

		output += fmt.Sprintf("%-2s %-30s %-8s %-4s %-10s %s\n", activeIndicator, t.Name, themeType, recommended, palette, source)
	}

	// Footer message.
	output += fmt.Sprintf("\n%d theme", len(themes))
	if len(themes) != 1 {
		output += "s"
	}

	if showingRecommendedOnly {
		output += " (recommended). Use without --recommended to see all themes.\n"
	} else {
		output += " available."
		if showStars {
			output += " ★ indicates recommended themes."
		}
		output += "\n"
	}

	if activeTheme != "" {
		output += fmt.Sprintf("Active theme: %s\n", activeTheme)
	}

	return output
}

// getThemeTypeString returns "Dark" or "Light" based on theme metadata.
func getThemeTypeString(t *Theme) string {
	if t.Meta.IsDark {
		return "Dark"
	}
	return "Light"
}

// getThemeSourceString extracts the source information from theme credits.
func getThemeSourceString(t *Theme) string {
	if t.Meta.Credits != nil && len(*t.Meta.Credits) > 0 {
		credits := *t.Meta.Credits
		if credits[0].Link != "" {
			return credits[0].Link
		}
		return credits[0].Name
	}
	return ""
}

// formatColorPalette creates a visual representation of the theme's color palette.
func formatColorPalette(t *Theme) string {
	defer perf.Track(nil, "theme.formatColorPalette")()

	// Use the main 8 ANSI colors similar to the web gallery.
	colors := []string{
		t.Background,
		t.Foreground,
		t.Red,
		t.Green,
		t.Yellow,
		t.Blue,
		t.Magenta,
		t.Cyan,
	}

	var result strings.Builder
	for _, hexColor := range colors {
		// Create a colored block using lipgloss.
		block := lipgloss.NewStyle().
			Foreground(lipgloss.Color(hexColor)).
			Render("█")
		result.WriteString(block)
	}

	return result.String()
}
