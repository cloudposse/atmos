package scaffoldcmd

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/experiments/init/internal/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// ScaffoldCmd represents the scaffold command
var ScaffoldCmd = &cobra.Command{
	Use:   "scaffold [command] [template] [target]",
	Short: "Scaffold projects from local or remote templates",
	Long: `Scaffold projects from local filesystem paths or remote Git repositories.

Examples:
  # Scaffold from local directory
  atmos scaffold generate ./my-template /tmp/my-project

  # Scaffold from remote Git repository
  atmos scaffold generate https://github.com/user/template.git /tmp/my-project

  # Scaffold with specific tag/ref
  atmos scaffold generate https://github.com/user/template.git?ref=v1.0.0 /tmp/my-project

  # List available templates (from embedded templates)
  atmos scaffold list`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// GenerateCmd represents the generate subcommand
var GenerateCmd = &cobra.Command{
	Use:   "generate [template] [target]",
	Short: "Generate a project from a template",
	Long: `Generate a project from a local template directory or remote Git repository.

The template can be:
- A local filesystem path (e.g., ./my-template)
- A remote Git repository URL (e.g., https://github.com/user/template.git)
- A remote Git repository with specific ref (e.g., https://github.com/user/template.git?ref=v1.0.0)

The target is the directory where the project will be generated.

If no template is provided, available templates will be listed.`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no arguments provided, show interactive menu
		if len(args) == 0 {
			return promptForTemplateAndTarget()
		}

		// If only one argument provided, show available templates
		if len(args) == 1 {
			fmt.Printf("Template '%s' specified but no target directory provided.\n", args[0])
			fmt.Println("Usage: atmos scaffold generate <template> <target-directory>")
			fmt.Println()
			return listScaffoldTemplates()
		}

		templatePath := args[0]
		targetPath := args[1]

		force, _ := cmd.Flags().GetBool("force")
		update, _ := cmd.Flags().GetBool("update")
		useDefaults, _ := cmd.Flags().GetBool("use-defaults")
		threshold, _ := cmd.Flags().GetInt("threshold")

		// Parse command-line values
		cmdValues := make(map[string]interface{})
		valueFlags, _ := cmd.Flags().GetStringArray("value")
		for _, valueFlag := range valueFlags {
			parts := strings.SplitN(valueFlag, "=", 2)
			if len(parts) == 2 {
				cmdValues[parts[0]] = parts[1]
			}
		}

		return generateProject(templatePath, targetPath, force, update, useDefaults, threshold, cmdValues)
	},
}

// ListCmd represents the list subcommand
var ListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available scaffold templates",
	Long:  "List all available scaffold templates configured in atmos.yaml.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listScaffoldTemplates()
	},
}

func init() {
	ScaffoldCmd.AddCommand(GenerateCmd)
	ScaffoldCmd.AddCommand(ListCmd)

	// Add flags to generate command
	GenerateCmd.Flags().BoolP("force", "f", false, "Force overwrite existing files")
	GenerateCmd.Flags().BoolP("update", "u", false, "Update existing files with 3-way merge")
	GenerateCmd.Flags().BoolP("use-defaults", "d", false, "Use default values without prompting")
	GenerateCmd.Flags().IntP("threshold", "t", 50, "Percentage threshold for merge changes (0-100)")
	GenerateCmd.Flags().StringArrayP("value", "v", []string{}, "Set a configuration value (format: key=value)")
}

// generateProject handles the generation of a project from a template
func generateProject(templatePath, targetPath string, force, update, useDefaults bool, threshold int, cmdValues map[string]interface{}) error {
	// Create UI instance
	ui := ui.NewInitUI()
	ui.SetMaxChanges(threshold)

	// Check if this is a scaffold template (from atmos.yaml config)
	if isScaffoldTemplate(templatePath) {
		return generateFromScaffoldTemplate(templatePath, targetPath, force, update, useDefaults, cmdValues, ui)
	}

	// Determine if template is local or remote
	if isRemoteTemplate(templatePath) {
		return generateFromRemote(templatePath, targetPath, force, update, useDefaults, cmdValues, ui)
	} else {
		return generateFromLocal(templatePath, targetPath, force, update, useDefaults, cmdValues, ui)
	}
}

// isScaffoldTemplate checks if the template is a scaffold template from atmos.yaml config
func isScaffoldTemplate(templatePath string) bool {
	// Read scaffold configuration to check if template exists
	scaffoldConfig, err := readScaffoldConfig(".")
	if err != nil {
		return false
	}

	templates, ok := scaffoldConfig["templates"]
	if !ok {
		return false
	}

	templatesMap, ok := templates.(map[string]interface{})
	if !ok {
		return false
	}

	_, exists := templatesMap[templatePath]
	return exists
}

// isRemoteTemplate checks if the template path is a remote Git repository
func isRemoteTemplate(templatePath string) bool {
	return strings.HasPrefix(templatePath, "http://") ||
		strings.HasPrefix(templatePath, "https://") ||
		strings.HasPrefix(templatePath, "git://") ||
		strings.HasPrefix(templatePath, "ssh://")
}

// generateFromScaffoldTemplate handles generation from a scaffold template defined in atmos.yaml
func generateFromScaffoldTemplate(templateName, targetPath string, force, update, useDefaults bool, cmdValues map[string]interface{}, ui *ui.InitUI) error {
	// Read scaffold configuration
	scaffoldConfig, err := readScaffoldConfig(".")
	if err != nil {
		return fmt.Errorf("failed to read scaffold config: %w", err)
	}

	templates, ok := scaffoldConfig["templates"]
	if !ok {
		return fmt.Errorf("no templates section found in scaffold configuration")
	}

	templatesMap, ok := templates.(map[string]interface{})
	if !ok {
		return fmt.Errorf("templates section is not a valid configuration")
	}

	templateConfig, exists := templatesMap[templateName]
	if !exists {
		return fmt.Errorf("scaffold template '%s' not found", templateName)
	}

	templateMap, ok := templateConfig.(map[string]interface{})
	if !ok {
		return fmt.Errorf("template configuration is not valid")
	}

	// Get template source
	source, ok := templateMap["source"].(string)
	if !ok {
		return fmt.Errorf("template '%s' missing source", templateName)
	}

	// For now, treat scaffold templates as remote templates
	// In the future, this could be enhanced to handle local scaffold templates
	return generateFromRemote(source, targetPath, force, update, useDefaults, cmdValues, ui)
}

// generateFromLocal handles generation from a local filesystem path
func generateFromLocal(templatePath, targetPath string, force, update, useDefaults bool, cmdValues map[string]interface{}, ui *ui.InitUI) error {
	// Resolve absolute path
	absTemplatePath, err := filepath.Abs(templatePath)
	if err != nil {
		return fmt.Errorf("failed to resolve template path: %w", err)
	}

	// Check if template directory exists
	if _, err := os.Stat(absTemplatePath); os.IsNotExist(err) {
		return fmt.Errorf("template directory does not exist: %s", absTemplatePath)
	}

	// Load template configuration from local filesystem
	config, err := loadLocalTemplate(absTemplatePath)
	if err != nil {
		return fmt.Errorf("failed to load template: %w", err)
	}

	// Execute the template generation using the existing UI logic
	return ui.Execute(*config, targetPath, force, update, useDefaults, cmdValues)
}

// listScaffoldTemplates lists all available scaffold templates from atmos.yaml
func listScaffoldTemplates() error {
	// Read scaffold configuration from current directory's atmos.yaml
	scaffoldConfig, err := readScaffoldConfig(".")
	if err != nil {
		// Check if it's a "file not found" error
		if strings.Contains(err.Error(), "Config File") && strings.Contains(err.Error(), "Not Found") {
			fmt.Println("No atmos.yaml configuration file found.")
			fmt.Println("Create an atmos.yaml file with a 'scaffold.templates' section to configure available templates.")
			return nil
		}
		return fmt.Errorf("failed to read scaffold config: %w", err)
	}

	// Get the templates section
	templates, ok := scaffoldConfig["templates"]
	if !ok {
		fmt.Println("No scaffold templates configured in atmos.yaml.")
		fmt.Println("Add a 'scaffold.templates' section to your atmos.yaml to configure available templates.")
		return nil
	}

	templatesMap, ok := templates.(map[string]interface{})
	if !ok {
		return fmt.Errorf("templates section is not a valid configuration")
	}

	// Check if there are any templates
	if len(templatesMap) == 0 {
		fmt.Println("No scaffold templates configured in atmos.yaml.")
		fmt.Println("Add templates to the 'scaffold.templates' section to get started.")
		return nil
	}

	// Get terminal width
	width, _, err := term.GetSize(uintptr(os.Stdout.Fd()))
	if err != nil {
		width = 80 // fallback width
	}

	// Calculate table width (leave some margin)
	tableWidth := width - 10

	// Create table data
	var rows []table.Row
	for templateName, templateConfig := range templatesMap {
		templateMap, ok := templateConfig.(map[string]interface{})
		if !ok {
			continue
		}

		// Get template source
		source, _ := templateMap["source"].(string)
		if source == "" {
			source = "unknown"
		}

		// Get template description (if available)
		description := ""
		if desc, ok := templateMap["description"].(string); ok {
			description = desc
		}

		// Get template ref (if available)
		ref := ""
		if r, ok := templateMap["ref"].(string); ok {
			ref = r
		}

		rows = append(rows, table.Row{templateName, source, ref, description})
	}

	// Calculate optimal column widths based on content and screen size
	templateWidth := 12    // Minimum width for template names
	sourceWidth := 20      // Minimum width for sources
	versionWidth := 8      // Minimum width for versions
	descriptionWidth := 15 // Minimum width for descriptions

	// Find the maximum content width for each column
	for _, row := range rows {
		if len(row[0]) > templateWidth {
			templateWidth = len(row[0])
		}
		if len(row[1]) > sourceWidth {
			sourceWidth = len(row[1])
		}
		if len(row[2]) > versionWidth {
			versionWidth = len(row[2])
		}
		if len(row[3]) > descriptionWidth {
			descriptionWidth = len(row[3])
		}
	}

	// Add padding to each column
	templateWidth += 2
	sourceWidth += 2
	versionWidth += 2
	descriptionWidth += 2

	// Calculate total content width needed
	totalContentWidth := templateWidth + sourceWidth + versionWidth + descriptionWidth + 6 // +6 for borders and spacing

	// If content is wider than screen, adjust column widths proportionally
	if totalContentWidth > tableWidth {
		// Calculate how much we need to reduce
		excess := totalContentWidth - tableWidth

		// Reduce source and description columns proportionally (they're most likely to be long)
		if sourceWidth > 15 {
			reduceSource := int(math.Min(float64(excess/2), float64(sourceWidth-15)))
			sourceWidth -= reduceSource
			excess -= reduceSource
		}

		if descriptionWidth > 10 && excess > 0 {
			reduceDesc := int(math.Min(float64(excess), float64(descriptionWidth-10)))
			descriptionWidth -= reduceDesc
		}
	}

	// Create and style the table
	columns := []table.Column{
		{Title: "Template", Width: templateWidth},
		{Title: "Source", Width: sourceWidth},
		{Title: "Version", Width: versionWidth},
		{Title: "Description", Width: descriptionWidth},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithWidth(tableWidth),
		table.WithFocused(false),
		table.WithHeight(len(rows)),
	)

	// Style the table with colors
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorWhite))
	s.Cell = s.Cell.
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Background(lipgloss.Color(theme.ColorDarkGray))

	t.SetStyles(s)

	// Print the table
	fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.ColorGreen)).Render("Available Scaffold Templates"))
	fmt.Println()
	fmt.Println(t.View())
	fmt.Println()

	return nil
}

// promptForTemplateAndTarget prompts the user to select a template and target directory
func promptForTemplateAndTarget() error {
	// Read scaffold configuration from current directory's atmos.yaml
	scaffoldConfig, err := readScaffoldConfig(".")
	if err != nil {
		// Check if it's a "file not found" error
		if strings.Contains(err.Error(), "Config File") && strings.Contains(err.Error(), "Not Found") {
			fmt.Println("No atmos.yaml configuration file found.")
			fmt.Println("Create an atmos.yaml file with a 'scaffold.templates' section to configure available templates.")
			return nil
		}
		return fmt.Errorf("failed to read scaffold config: %w", err)
	}

	// Get the templates section
	templates, ok := scaffoldConfig["templates"]
	if !ok {
		fmt.Println("No scaffold templates configured in atmos.yaml.")
		fmt.Println("Add a 'scaffold.templates' section to your atmos.yaml to configure available templates.")
		return nil
	}

	templatesMap, ok := templates.(map[string]interface{})
	if !ok {
		return fmt.Errorf("templates section is not a valid configuration")
	}

	// Check if there are any templates
	if len(templatesMap) == 0 {
		fmt.Println("No scaffold templates configured in atmos.yaml.")
		fmt.Println("Add templates to the 'scaffold.templates' section to get started.")
		return nil
	}

	// Create options for the select
	var options []huh.Option[string]
	for templateName, templateConfig := range templatesMap {
		templateMap, ok := templateConfig.(map[string]interface{})
		if !ok {
			continue
		}

		// Get template description
		description := ""
		if desc, ok := templateMap["description"].(string); ok {
			description = desc
		}

		// Get template source
		source := ""
		if src, ok := templateMap["source"].(string); ok {
			source = src
		}

		// Format display text
		displayText := fmt.Sprintf("%-20s   %s", templateName, description)
		if source != "" {
			displayText += fmt.Sprintf(" (from %s)", source)
		}

		options = append(options, huh.NewOption(displayText, templateName))
	}

	var selectedTemplate string
	var targetPath string

	// Create the selection form
	selectionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a scaffold template").
				Description("Choose from the available scaffold templates (press 'q' to quit)").
				Options(options...).
				Value(&selectedTemplate),
		),
	)

	err = selectionForm.Run()
	if err != nil {
		return err
	}

	// Display selected template details
	fmt.Println()
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)

	fmt.Println(descStyle.Render(fmt.Sprintf("Selected template: %s", selectedTemplate)))

	// Get template config for target directory suggestion
	templateConfig := templatesMap[selectedTemplate].(map[string]interface{})
	targetDir, _ := templateConfig["target_dir"].(string)

	// Generate suggested directory name
	suggestedDir := "./" + selectedTemplate
	if targetDir != "" {
		suggestedDir = targetDir
	}

	// Second form to get target directory
	pathForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Target directory").
				Description(fmt.Sprintf("Where should the scaffold be generated? (suggested: %s)", suggestedDir)).
				Placeholder(suggestedDir).
				Value(&targetPath).
				Validate(func(s string) error {
					if s == "" {
						return nil // Empty is OK, will use suggested default
					}
					return nil
				}),
		),
	)

	err = pathForm.Run()
	if err != nil {
		return err
	}

	// Use suggested directory if empty
	if targetPath == "" {
		targetPath = suggestedDir
	}

	// Now call the generate function with the selected template and target
	return generateProject(selectedTemplate, targetPath, false, false, false, 50, make(map[string]interface{}))
}
