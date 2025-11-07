package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// themeListCmd lists available terminal themes.
var themeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available terminal themes",
	Long:  "Display available terminal themes that can be used for markdown rendering. By default shows all themes.",
	Example: `# List all themes
atmos theme list

# Show only recommended themes
atmos theme list --recommended`,
	Args: cobra.NoArgs,
	RunE: executeThemeList,
}

var themeListRecommendedOnly bool

func init() {
	themeListCmd.Flags().BoolVar(&themeListRecommendedOnly, "recommended", false, "Show only recommended themes")
	themeCmd.AddCommand(themeListCmd)
}

// executeThemeList runs the theme list command.
func executeThemeList(cmd *cobra.Command, args []string) error {
	// Get the current active theme from configuration
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	activeTheme := ""
	if err == nil {
		activeTheme = atmosConfig.Settings.Terminal.Theme
	}

	themes, err := listAvailableThemes()
	if err != nil {
		return err
	}

	// Filter for recommended themes if requested
	if themeListRecommendedOnly {
		themes = filterRecommended(themes, activeTheme)
	}

	// When showing all themes, display stars for recommended ones
	showStars := !themeListRecommendedOnly

	return displayThemeList(themes, activeTheme, themeListRecommendedOnly, showStars)
}

// listAvailableThemes retrieves all available themes.
func listAvailableThemes() ([]*theme.Theme, error) {
	registry, err := theme.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load theme registry: %w", err)
	}

	return registry.List(), nil
}

// filterRecommended returns only recommended themes, but ensures the active theme is included.
func filterRecommended(themes []*theme.Theme, activeTheme string) []*theme.Theme {
	var recommended []*theme.Theme
	hasActiveTheme := false

	for _, t := range themes {
		if theme.IsRecommended(t.Name) {
			recommended = append(recommended, t)
			if t.Name == activeTheme {
				hasActiveTheme = true
			}
		}
	}

	// If the active theme is not in the recommended list, add it
	if activeTheme != "" && !hasActiveTheme {
		for _, t := range themes {
			if t.Name == activeTheme {
				recommended = append(recommended, t)
				break
			}
		}
	}

	// Sort the themes by name for consistent output
	sort.Slice(recommended, func(i, j int) bool {
		return recommended[i].Name < recommended[j].Name
	})

	return recommended
}

// displayThemeList formats and displays the themes to the terminal.
func displayThemeList(themes []*theme.Theme, activeTheme string, showingRecommendedOnly bool, showStars bool) error {
	// Check if we're in TTY mode
	if !term.IsTTYSupportForStdout() {
		// Fall back to simple text output for non-TTY
		output := formatSimpleThemeList(themes, activeTheme, showingRecommendedOnly, showStars)
		ui.Write(output)
		return nil
	}

	output := formatThemeTable(themes, activeTheme, showingRecommendedOnly, showStars)
	ui.Write(output)
	return nil
}

// buildThemeRows converts themes to table rows for display.
func buildThemeRows(themes []*theme.Theme, activeTheme string, showStars bool) [][]string {
	var rows [][]string
	for _, t := range themes {
		row := formatThemeRow(t, activeTheme, showStars)
		rows = append(rows, row)
	}
	return rows
}

// formatThemeRow formats a single theme as a table row.
func formatThemeRow(t *theme.Theme, activeTheme string, showStars bool) []string {
	// Active indicator
	activeIndicator := "  "
	if t.Name == activeTheme {
		activeIndicator = "> "
	}

	// Theme name with recommended indicator (only show star when requested)
	name := t.Name
	if showStars && theme.IsRecommended(t.Name) {
		name += " ★"
	}

	// Theme type (Dark/Light)
	themeType := getThemeTypeString(t)

	// Source
	source := getThemeSourceString(t)
	const maxSourceLen = 50
	const sourceElipsis = 47
	if len(source) > maxSourceLen {
		source = source[:sourceElipsis] + "..."
	}

	return []string{
		activeIndicator,
		name,
		themeType,
		source,
	}
}

// buildFooterMessage creates the footer text for the theme list.
func buildFooterMessage(themeCount int, showingRecommendedOnly bool, showStars bool, activeTheme string) string {
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
func formatThemeTable(themes []*theme.Theme, activeTheme string, showingRecommendedOnly bool, showStars bool) string {
	// Prepare headers and rows
	headers := []string{"", "Name", "Type", "Source"}
	rows := buildThemeRows(themes, activeTheme, showStars)

	// Use the new themed table creation
	output := theme.CreateThemedTable(headers, rows) + "\n"

	// Footer message
	styles := theme.GetCurrentStyles()
	footer := buildFooterMessage(len(themes), showingRecommendedOnly, showStars, activeTheme)
	output += styles.Footer.Render(footer) + "\n"

	return output
}

// formatSimpleThemeList formats themes as simple text for non-TTY output.
func formatSimpleThemeList(themes []*theme.Theme, activeTheme string, showingRecommendedOnly bool, showStars bool) string {
	var output string
	const lineWidth = 80

	// Header
	output += fmt.Sprintf("   %-30s %-8s %-4s %s\n", "Name", "Type", "Rec", "Source")
	output += fmt.Sprintf("%s\n", strings.Repeat("=", lineWidth))

	// Theme rows
	for _, t := range themes {
		activeIndicator := "  "
		if t.Name == activeTheme {
			activeIndicator = "> "
		}

		recommended := ""
		if showStars && theme.IsRecommended(t.Name) {
			recommended = "★"
		}

		themeType := getThemeTypeString(t)
		source := getThemeSourceString(t)

		output += fmt.Sprintf("%-2s %-30s %-8s %-4s %s\n", activeIndicator, t.Name, themeType, recommended, source)
	}

	// Footer message
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
func getThemeTypeString(t *theme.Theme) string {
	if t.Meta.IsDark {
		return "Dark"
	}
	return "Light"
}

// getThemeSourceString extracts the source information from theme credits.
func getThemeSourceString(t *theme.Theme) string {
	if t.Meta.Credits != nil && len(*t.Meta.Credits) > 0 {
		credits := *t.Meta.Credits
		if credits[0].Link != "" {
			return credits[0].Link
		}
		return credits[0].Name
	}
	return ""
}
