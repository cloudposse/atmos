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

// listThemesCmd lists available terminal themes (alias for 'theme list').
var listThemesCmd = &cobra.Command{
	Use:   "themes",
	Short: "List available terminal themes (alias for 'theme list')",
	Long:  "Display available terminal themes that can be used for markdown rendering. By default shows recommended themes.\nThis is an alias for 'atmos theme list'.",
	Example: "atmos list themes\n" +
		"atmos list themes --all",
	Args: cobra.NoArgs,
	RunE: executeListThemes,
}

const (
	maxSourceLength   = 50
	sourceEllipsisLen = 47
	lineWidth         = 80
)

var listAllThemes bool

func init() {
	listThemesCmd.Flags().BoolVar(&listAllThemes, "all", false, "Show all available themes (default: show only recommended themes)")
	listCmd.AddCommand(listThemesCmd)
}

// executeListThemes runs the list themes command.
func executeListThemes(cmd *cobra.Command, args []string) error {
	// Get the current active theme from configuration
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	activeTheme := ""
	if err == nil {
		activeTheme = atmosConfig.Settings.Terminal.Theme
	}

	themes, err := listThemes()
	if err != nil {
		return err
	}

	// Default behavior: show only recommended themes
	// --all flag: show everything
	if !listAllThemes {
		themes = filterRecommendedThemes(themes, activeTheme)
	}

	return displayThemes(themes, activeTheme, !listAllThemes)
}

// listThemes retrieves all available themes.
func listThemes() ([]*theme.Theme, error) {
	registry, err := theme.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load theme registry: %w", err)
	}

	return registry.List(), nil
}

// filterRecommendedThemes returns only recommended themes, but ensures the active theme is included.
func filterRecommendedThemes(themes []*theme.Theme, activeTheme string) []*theme.Theme {
	var recommended []*theme.Theme
	hasActiveTheme := false

	for _, t := range themes {
		if theme.IsRecommended(t.Name) {
			recommended = append(recommended, t)
			if strings.EqualFold(t.Name, activeTheme) {
				hasActiveTheme = true
			}
		}
	}

	// If the active theme is not in the recommended list, add it
	if activeTheme != "" && !hasActiveTheme {
		for _, t := range themes {
			if strings.EqualFold(t.Name, activeTheme) {
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

// displayThemes formats and displays the themes to the terminal.
func displayThemes(themes []*theme.Theme, activeTheme string, showingRecommendedOnly bool) error {
	// Check if we're in TTY mode
	if !term.IsTTYSupportForStdout() {
		// Fall back to simple text output for non-TTY
		output := formatSimpleOutput(themes, activeTheme, showingRecommendedOnly)
		return ui.Write(output)
	}

	output := formatThemesTable(themes, activeTheme, showingRecommendedOnly)
	return ui.Write(output)
}

// formatThemesTable formats themes into a styled Charmbracelet table.
func formatThemesTable(themes []*theme.Theme, activeTheme string, showingRecommendedOnly bool) string {
	// Prepare headers and rows
	headers := []string{"", "Name", "Type", "Source"}
	var rows [][]string

	for _, t := range themes {
		// Active indicator
		activeIndicator := "  "
		if t.Name == activeTheme {
			activeIndicator = "> "
		}

		// Theme name with recommended indicator (only show star when --all is used)
		name := t.Name
		if listAllThemes && theme.IsRecommended(t.Name) {
			name += " ★"
		}

		// Theme type (Dark/Light)
		themeType := getThemeType(t)

		// Source
		source := getThemeSource(t)
		if len(source) > maxSourceLength {
			source = source[:sourceEllipsisLen] + "..."
		}

		row := []string{
			activeIndicator,
			name,
			themeType,
			source,
		}
		rows = append(rows, row)
	}

	// Use the new themed table creation
	output := theme.CreateThemedTable(headers, rows) + "\n"

	// Footer message
	styles := theme.GetCurrentStyles()

	footer := fmt.Sprintf("\n%d theme", len(themes))
	if len(themes) != 1 {
		footer += "s"
	}

	if showingRecommendedOnly {
		footer += " (recommended). Use --all to see all available themes."
	} else {
		footer += " available."
	}

	if activeTheme != "" {
		footer += fmt.Sprintf("\nActive theme: %s", activeTheme)
	}

	output += styles.Footer.Render(footer) + "\n"

	return output
}

// formatSimpleOutput formats themes as simple text for non-TTY output.
func formatSimpleOutput(themes []*theme.Theme, activeTheme string, showingRecommendedOnly bool) string {
	var output string

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
		if listAllThemes && theme.IsRecommended(t.Name) {
			recommended = "★"
		}

		themeType := getThemeType(t)
		source := getThemeSource(t)

		output += fmt.Sprintf("%-2s %-30s %-8s %-4s %s\n", activeIndicator, t.Name, themeType, recommended, source)
	}

	// Footer message
	output += fmt.Sprintf("\n%d theme", len(themes))
	if len(themes) != 1 {
		output += "s"
	}

	if showingRecommendedOnly {
		output += " (recommended). Use --all to see all available themes.\n"
	} else {
		output += " available.\n"
	}

	if activeTheme != "" {
		output += fmt.Sprintf("Active theme: %s\n", activeTheme)
	}

	return output
}

// getThemeType returns "Dark" or "Light" based on theme metadata.
func getThemeType(t *theme.Theme) string {
	if t.Meta.IsDark {
		return "Dark"
	}
	return "Light"
}

// getThemeSource extracts the source information from theme credits.
func getThemeSource(t *theme.Theme) string {
	if t.Meta.Credits != nil && len(*t.Meta.Credits) > 0 {
		credits := *t.Meta.Credits
		if credits[0].Link != "" {
			return credits[0].Link
		}
		return credits[0].Name
	}
	return ""
}
