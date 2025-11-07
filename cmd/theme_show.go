package cmd

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
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
	providerCount        = 5 // Number of Terraform providers in demo
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
		return fmt.Errorf("%w: %s", theme.ErrThemeNotFound, themeName)
	}

	// Generate color scheme for the theme
	scheme := theme.GenerateColorScheme(selectedTheme)
	styles := theme.GetStyles(&scheme)

	// Display theme information
	output := formatThemeDetails(selectedTheme, &scheme, styles)
	ui.Write(output)

	return nil
}

// formatThemeHeader creates the header section with theme name and badges.
func formatThemeHeader(t *theme.Theme, styles *theme.StyleSet) string {
	themeHeader := fmt.Sprintf("Theme: %s", t.Name)
	if theme.IsRecommended(t.Name) {
		themeHeader += " " + styles.Success.Render("★ Recommended")
	}
	return styles.Title.Render(themeHeader) + sectionSeparator
}

// formatThemeMetadata formats the theme's metadata information.
func formatThemeMetadata(t *theme.Theme, styles *theme.StyleSet) string {
	var output strings.Builder

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

	return output.String()
}

// formatUIElements formats the sample UI elements section.
func formatUIElements(styles *theme.StyleSet) string {
	var output strings.Builder

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

	return output.String()
}

// formatMarkdownPreview renders a sample markdown document with the theme.
func formatMarkdownPreview(t *theme.Theme, _ *theme.StyleSet) string {
	// Sample markdown content
	markdownContent := `# Heading Level 1
## Heading Level 2
### Heading Level 3

This is a paragraph with **bold text** and *italic text*.

> This is a blockquote with important information.
> It can span multiple lines.

- First item in list
- Second item with **emphasis**
  - Nested item
  - Another nested item

1. Numbered list item
2. Second numbered item

` + "```go\n" + `// Code block with syntax highlighting
func main() {
    fmt.Println("Hello, Atmos!")
}` + "\n```\n\n" +
		`Here's a [link to documentation](https://atmos.tools).

| Column 1 | Column 2 | Column 3 |
|----------|----------|----------|
| Data 1   | Data 2   | Data 3   |
| Value A  | Value B  | Value C  |
`

	// Create a markdown renderer with the theme
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				Theme: t.Name,
			},
		},
	}

	renderer, err := markdown.NewTerminalMarkdownRenderer(atmosConfig)
	if err != nil {
		return "Error rendering markdown preview\n"
	}

	rendered, err := renderer.Render(markdownContent)
	if err != nil {
		return "Error rendering markdown preview\n"
	}

	return rendered
}

// generateLogDemo creates a demonstration of log output with styled levels and key-value pairs.
func generateLogDemo(scheme *theme.ColorScheme) string {
	// Get theme styles
	logStyles := theme.GetLogStyles(scheme)

	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Create a new logger instance with our theme styles
	demoLogger := log.New(&buf)
	demoLogger.SetStyles(logStyles)

	// Configure logger to show the level prefix
	demoLogger.SetReportTimestamp(false)
	demoLogger.SetReportCaller(false)

	// Temporarily set the log level to show all messages
	demoLogger.SetLevel(log.DebugLevel)

	// Generate sample log messages with key-value pairs
	var output strings.Builder

	// Debug message
	demoLogger.Debug("Processing component", "component", "vpc", "stack", "dev")
	output.WriteString(buf.String())
	buf.Reset()

	// Info message
	demoLogger.Info("Terraform init completed", "duration", "3.2s", "providers", providerCount)
	output.WriteString(buf.String())
	buf.Reset()

	// Warn message
	demoLogger.Warn("Resource already exists", "resource", "aws_vpc.main", "action", "skip")
	output.WriteString(buf.String())
	buf.Reset()

	// Error message
	demoLogger.Error("Failed to connect", "endpoint", "api.example.com", "retry", 3)
	output.WriteString(buf.String())

	return output.String()
}

// formatUsageInstructions generates the usage instructions section.
func formatUsageInstructions(t *theme.Theme, styles *theme.StyleSet) string {
	footer := fmt.Sprintf("\nTo use this theme, set ATMOS_THEME=%s or add to atmos.yaml:\n", t.Name)
	footer += "settings:\n"
	footer += "  terminal:\n"
	footer += fmt.Sprintf("    theme: %s\n", t.Name)
	return styles.Footer.Render(footer)
}

// formatThemeDetails creates a detailed preview of the theme.
func formatThemeDetails(t *theme.Theme, scheme *theme.ColorScheme, styles *theme.StyleSet) string {
	var output strings.Builder

	// Theme header
	output.WriteString(formatThemeHeader(t, styles))

	// Theme metadata
	output.WriteString(formatThemeMetadata(t, styles))
	output.WriteString(lineBreak)

	// Color Palette Section
	output.WriteString(styles.Heading.Render("COLOR PALETTE"))
	output.WriteString(sectionSeparator)
	output.WriteString(formatColorPalette(t))
	output.WriteString(lineBreak)

	// Log Output Preview Section
	output.WriteString(styles.Heading.Render("LOG OUTPUT PREVIEW"))
	output.WriteString(sectionSeparator)
	output.WriteString(generateLogDemo(scheme))
	output.WriteString(lineBreak)

	// Markdown Preview Section
	output.WriteString(styles.Heading.Render("MARKDOWN PREVIEW"))
	output.WriteString(sectionSeparator)
	output.WriteString(formatMarkdownPreview(t, styles))
	output.WriteString(lineBreak)

	// Sample UI Elements Section
	output.WriteString(styles.Heading.Render("SAMPLE UI ELEMENTS"))
	output.WriteString(sectionSeparator)
	output.WriteString(formatUIElements(styles))
	output.WriteString(sectionSeparator)

	// Footer with activation instructions
	output.WriteString(formatUsageInstructions(t, styles))

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
