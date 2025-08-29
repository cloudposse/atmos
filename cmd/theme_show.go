package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// Constants for formatting.
const (
	colorColumnSeparator = "\n  "
	sectionSeparator     = "\n\n"
	lineBreak            = "\n"
	hexColorLength       = 6
	hexBase              = 16
	intBitSize           = 64
	luminanceRedWeight   = 0.299
	luminanceGreenWeight = 0.587
	luminanceBlueWeight  = 0.114
	luminanceThreshold   = 0.5
	rgbMax               = 255
)

// themeShowCmd shows details and preview of a specific theme.
var themeShowCmd = &cobra.Command{
	Use:   "show [theme-name]",
	Short: "Show details and preview of a specific theme",
	Long:  "Display color palette and sample UI elements for a specific terminal theme.",
	Example: `# Show details for the Dracula theme
atmos theme show dracula

# Show details for the Solarized Dark theme
atmos theme show "solarized-dark"`,
	Args: cobra.ExactArgs(1),
	RunE: executeThemeShow,
}

func init() {
	themeCmd.AddCommand(themeShowCmd)
}

// executeThemeShow displays detailed information about a specific theme.
func executeThemeShow(cmd *cobra.Command, args []string) error {
	themeName := args[0]

	// Load the theme registry
	registry, err := theme.NewRegistry()
	if err != nil {
		return fmt.Errorf("failed to load theme registry: %w", err)
	}

	// Get the specified theme
	selectedTheme, exists := registry.Get(themeName)
	if !exists {
		return fmt.Errorf("%w: %s", errUtils.ErrThemeNotFound, themeName)
	}

	// Generate color scheme for the theme
	scheme := theme.GenerateColorScheme(selectedTheme)
	styles := theme.GetStyles(&scheme)

	// Display theme information
	output := formatThemeDetails(selectedTheme, &scheme, styles)
	fmt.Fprint(os.Stderr, output)

	return nil
}

// formatThemeDetails creates a detailed preview of the theme.
func formatThemeDetails(t *theme.Theme, scheme *theme.ColorScheme, styles *theme.StyleSet) string {
	var output strings.Builder

	// Theme header with recommended badge on same line
	themeHeader := fmt.Sprintf("Theme: %s", t.Name)
	if theme.IsRecommended(t.Name) {
		themeHeader += " " + styles.Success.Render("★ Recommended")
	}
	output.WriteString(styles.Title.Render(themeHeader))
	output.WriteString(sectionSeparator)

	// Theme metadata with styled labels
	output.WriteString(styles.Label.Render("Type:"))
	if t.Meta.IsDark {
		output.WriteString(" Dark\n")
	} else {
		output.WriteString(" Light\n")
	}

	if t.Meta.Credits != nil && len(*t.Meta.Credits) > 0 {
		credits := *t.Meta.Credits
		output.WriteString(styles.Label.Render("Source:"))
		output.WriteString(fmt.Sprintf(" %s\n", credits[0].Name))
		if credits[0].Link != "" {
			output.WriteString(styles.Label.Render("Link:"))
			output.WriteString(fmt.Sprintf(" %s\n", credits[0].Link))
		}
	}

	output.WriteString(lineBreak)

	// Color Palette Section
	output.WriteString(styles.Heading.Render("COLOR PALETTE"))
	output.WriteString(sectionSeparator)

	// Display color swatches
	output.WriteString(formatColorPalette(t))
	output.WriteString(lineBreak)

	// Sample UI Elements Section
	output.WriteString(styles.Heading.Render("SAMPLE UI ELEMENTS"))
	output.WriteString(sectionSeparator)

	// Status messages
	output.WriteString(styles.Label.Render("Status Messages:"))
	output.WriteString(colorColumnSeparator)
	output.WriteString(styles.Success.Render("✓ Success message"))
	output.WriteString(colorColumnSeparator)
	output.WriteString(styles.Warning.Render("⚠ Warning message"))
	output.WriteString(colorColumnSeparator)
	output.WriteString(styles.Error.Render("✗ Error message"))
	output.WriteString(colorColumnSeparator)
	output.WriteString(styles.Info.Render("ℹ Info message"))
	output.WriteString(sectionSeparator)

	// Sample table
	output.WriteString(styles.Label.Render("Sample Table:"))
	output.WriteString(lineBreak)
	headers := []string{"Component", "Stack", "Status"}
	rows := [][]string{
		{"vpc", "dev", "deployed"},
		{"rds", "staging", "pending"},
		{"eks", "prod", "active"},
	}
	tableOutput := theme.CreateMinimalTable(headers, rows)
	output.WriteString(tableOutput)
	output.WriteString(lineBreak)

	// Command examples
	output.WriteString(lineBreak)
	output.WriteString(styles.Label.Render("Command Examples:"))
	output.WriteString(lineBreak)
	output.WriteString("  ")
	output.WriteString(styles.Command.Render("atmos terraform plan"))
	output.WriteString(" - ")
	output.WriteString(styles.Description.Render("Plan terraform changes"))
	output.WriteString(colorColumnSeparator)
	output.WriteString(styles.Command.Render("atmos describe stacks"))
	output.WriteString(" - ")
	output.WriteString(styles.Description.Render("Show stack configurations"))
	output.WriteString(sectionSeparator)

	// Footer with activation instructions
	footer := fmt.Sprintf("\nTo use this theme, set ATMOS_THEME=%s or add to atmos.yaml:\n", t.Name)
	footer += "settings:\n"
	footer += "  terminal:\n"
	footer += fmt.Sprintf("    theme: %s\n", t.Name)

	output.WriteString(styles.Footer.Render(footer))

	return output.String()
}

// formatColorPalette displays the theme's color palette.
func formatColorPalette(t *theme.Theme) string {
	var output strings.Builder

	// Create color blocks for each color
	colors := []struct {
		name  string
		value string
		label string
	}{
		{"Black", t.Black, "Black"},
		{"Red", t.Red, "Red"},
		{"Green", t.Green, "Green"},
		{"Yellow", t.Yellow, "Yellow"},
		{"Blue", t.Blue, "Blue"},
		{"Magenta", t.Magenta, "Magenta"},
		{"Cyan", t.Cyan, "Cyan"},
		{"White", t.White, "White"},
		{"BrightBlack", t.BrightBlack, "Bright Black"},
		{"BrightRed", t.BrightRed, "Bright Red"},
		{"BrightGreen", t.BrightGreen, "Bright Green"},
		{"BrightYellow", t.BrightYellow, "Bright Yellow"},
		{"BrightBlue", t.BrightBlue, "Bright Blue"},
		{"BrightMagenta", t.BrightMagenta, "Bright Magenta"},
		{"BrightCyan", t.BrightCyan, "Bright Cyan"},
		{"BrightWhite", t.BrightWhite, "Bright White"},
		{"Background", t.Background, "Background"},
		{"Foreground", t.Foreground, "Foreground"},
	}

	// Display colors in a grid-like format
	for i, color := range colors {
		// Create a color block
		block := lipgloss.NewStyle().
			Background(lipgloss.Color(color.value)).
			Foreground(lipgloss.Color(getContrastColor(color.value))).
			Padding(0, 1).
			Render(fmt.Sprintf("%-14s", color.label))

		// Add hex value
		hexValue := fmt.Sprintf(" %s", color.value)

		output.WriteString(block)
		output.WriteString(hexValue)

		// Add newline every 2 colors for better layout
		if i%2 == 1 {
			output.WriteString(lineBreak)
		} else {
			output.WriteString("  ")
		}
	}

	return output.String()
}

// getContrastColor returns white or black depending on the background color.
func getContrastColor(hexColor string) string {
	// Remove # prefix if present
	color := strings.TrimPrefix(hexColor, "#")

	// Handle short hex colors (e.g., #FFF)
	if len(color) == 3 {
		color = string(color[0]) + string(color[0]) +
			string(color[1]) + string(color[1]) +
			string(color[2]) + string(color[2])
	}

	// Parse hex values
	if len(color) != hexColorLength {
		return "#000000" // Default to black for invalid colors
	}

	// Convert hex to RGB
	r, _ := strconv.ParseInt(color[0:2], hexBase, intBitSize)
	g, _ := strconv.ParseInt(color[2:4], hexBase, intBitSize)
	b, _ := strconv.ParseInt(color[4:6], hexBase, intBitSize)

	// Calculate relative luminance using WCAG formula
	// See: https://www.w3.org/TR/WCAG20-TECHS/G17.html
	luminance := (luminanceRedWeight*float64(r) + luminanceGreenWeight*float64(g) + luminanceBlueWeight*float64(b)) / rgbMax

	// Use black text for light backgrounds, white for dark
	if luminance > luminanceThreshold {
		return "#000000"
	}
	return "#ffffff"
}
