package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/ui/theme"
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
		return fmt.Errorf("theme %q not found", themeName)
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
	
	// Theme header
	output.WriteString(styles.Title.Render(fmt.Sprintf("Theme: %s", t.Name)))
	output.WriteString("\n\n")
	
	// Theme metadata
	if t.Meta.IsDark {
		output.WriteString("Type: Dark\n")
	} else {
		output.WriteString("Type: Light\n")
	}
	
	if theme.IsRecommended(t.Name) {
		output.WriteString(styles.Success.Render("★ Recommended"))
		output.WriteString("\n")
	}
	
	if t.Meta.Credits != nil && len(*t.Meta.Credits) > 0 {
		credits := *t.Meta.Credits
		output.WriteString(fmt.Sprintf("Source: %s\n", credits[0].Name))
		if credits[0].Link != "" {
			output.WriteString(fmt.Sprintf("Link: %s\n", credits[0].Link))
		}
	}
	
	output.WriteString("\n")
	
	// Color Palette Section
	output.WriteString(styles.Heading.Render("COLOR PALETTE"))
	output.WriteString("\n\n")
	
	// Display color swatches
	output.WriteString(formatColorPalette(t))
	output.WriteString("\n")
	
	// Sample UI Elements Section
	output.WriteString(styles.Heading.Render("SAMPLE UI ELEMENTS"))
	output.WriteString("\n\n")
	
	// Status messages
	output.WriteString("Status Messages:\n")
	output.WriteString("  ")
	output.WriteString(styles.Success.Render("✓ Success message"))
	output.WriteString("\n  ")
	output.WriteString(styles.Warning.Render("⚠ Warning message"))
	output.WriteString("\n  ")
	output.WriteString(styles.Error.Render("✗ Error message"))
	output.WriteString("\n  ")
	output.WriteString(styles.Info.Render("ℹ Info message"))
	output.WriteString("\n\n")
	
	// Sample table
	output.WriteString("Sample Table:\n")
	headers := []string{"Component", "Stack", "Status"}
	rows := [][]string{
		{"vpc", "dev", "deployed"},
		{"rds", "staging", "pending"},
		{"eks", "prod", "active"},
	}
	tableOutput := theme.CreateMinimalTable(headers, rows)
	output.WriteString(tableOutput)
	output.WriteString("\n")
	
	// Command examples
	output.WriteString("\nCommand Examples:\n")
	output.WriteString("  ")
	output.WriteString(styles.Command.Render("atmos terraform plan"))
	output.WriteString(" - ")
	output.WriteString(styles.Description.Render("Plan terraform changes"))
	output.WriteString("\n  ")
	output.WriteString(styles.Command.Render("atmos describe stacks"))
	output.WriteString(" - ")
	output.WriteString(styles.Description.Render("Show stack configurations"))
	output.WriteString("\n\n")
	
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
			Render(fmt.Sprintf("%-12s", color.label))
		
		// Add hex value
		hexValue := fmt.Sprintf(" %s", color.value)
		
		output.WriteString(block)
		output.WriteString(hexValue)
		
		// Add newline every 2 colors for better layout
		if i%2 == 1 {
			output.WriteString("\n")
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
	if len(color) != 6 {
		return "#000000" // Default to black for invalid colors
	}
	
	// Convert hex to RGB
	r, _ := strconv.ParseInt(color[0:2], 16, 64)
	g, _ := strconv.ParseInt(color[2:4], 16, 64)
	b, _ := strconv.ParseInt(color[4:6], 16, 64)
	
	// Calculate relative luminance using WCAG formula
	// See: https://www.w3.org/TR/WCAG20-TECHS/G17.html
	luminance := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255
	
	// Use black text for light backgrounds, white for dark
	if luminance > 0.5 {
		return "#000000"
	}
	return "#ffffff"
}