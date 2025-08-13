package scaffoldcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/experiments/init/internal/config"
	"github.com/cloudposse/atmos/experiments/init/internal/templating"
	"github.com/cloudposse/atmos/experiments/init/internal/ui"
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

		// If only one argument provided, check if template exists first
		if len(args) == 1 {
			templatePath := args[0]

			// Check if this is a valid scaffold template
			if isScaffoldTemplate(templatePath) {
				fmt.Printf("Template '%s' specified but no target directory provided.\n", templatePath)
				fmt.Println("Usage: atmos scaffold generate <template> <target-directory>")
				return nil
			}

			// Check if this is a valid local path
			if _, err := os.Stat(templatePath); err == nil {
				fmt.Printf("Template '%s' specified but no target directory provided.\n", templatePath)
				fmt.Println("Usage: atmos scaffold generate <template> <target-directory>")
				return nil
			}

			// Check if this looks like a remote URL
			if isRemoteTemplate(templatePath) {
				fmt.Printf("Template '%s' specified but no target directory provided.\n", templatePath)
				fmt.Println("Usage: atmos scaffold generate <template> <target-directory>")
				return nil
			}

			// Template doesn't exist, show error and available templates
			fmt.Printf("Template '%s' not found.\n", templatePath)
			fmt.Println("Available templates:")
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
	ui.SetThreshold(threshold)

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
	// Read scaffold section from atmos.yaml to check if template exists
	scaffoldSection, err := config.ReadAtmosScaffoldSection(".")
	if err != nil {
		return false
	}

	templates, ok := scaffoldSection["templates"]
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
	// Read scaffold section from atmos.yaml
	scaffoldSection, err := config.ReadAtmosScaffoldSection(".")
	if err != nil {
		return fmt.Errorf("failed to read scaffold section from atmos.yaml: %w", err)
	}

	templates, ok := scaffoldSection["templates"]
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

	// Validate scaffold template if .atmos/scaffold.yaml exists
	if err := validateScaffoldTemplate(targetPath, templateName); err != nil {
		return fmt.Errorf("scaffold validation failed: %w", err)
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
	// Read scaffold section from atmos.yaml
	scaffoldSection, err := config.ReadAtmosScaffoldSection(".")
	if err != nil {
		return fmt.Errorf("failed to read scaffold section from atmos.yaml: %w", err)
	}

	// Get the templates section
	templates, ok := scaffoldSection["templates"]
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

	// Get template data for display
	rows, header := getTemplateTableData(templatesMap)

	// Use the UI package to display the table
	uiInstance := ui.NewInitUI()
	uiInstance.DisplayTemplateTable(header, rows)

	return nil
}

// getTemplateTableData extracts template data for table display
func getTemplateTableData(templatesMap map[string]interface{}) ([][]string, []string) {
	var rows [][]string
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

		rows = append(rows, []string{templateName, source, ref, description})
	}

	header := []string{"Template", "Source", "Version", "Description"}
	return rows, header
}

// promptForTemplateAndTarget prompts the user to select a template and target directory
func promptForTemplateAndTarget() error {
	// Read scaffold section from atmos.yaml
	scaffoldSection, err := config.ReadAtmosScaffoldSection(".")
	if err != nil {
		return fmt.Errorf("failed to read scaffold section from atmos.yaml: %w", err)
	}

	// Get the templates section
	templates, ok := scaffoldSection["templates"]
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
	targetDirTemplate, _ := templateConfig["target_dir"].(string)

	// Check if the template has any field definitions
	// For scaffold templates from atmos.yaml, we need to check if there's a schema defined
	// For now, we'll assume no fields unless explicitly defined in the template
	var userValues map[string]interface{}

	// TODO: In the future, we could:
	// 1. Download the template first to check for scaffold.yaml
	// 2. Or define fields in the atmos.yaml template configuration
	// 3. Or use a default schema for scaffold templates

	// For now, use empty values since no fields are defined
	userValues = make(map[string]interface{})

	// Now render the target directory template with the collected values
	suggestedDir := "./" + selectedTemplate
	if targetDirTemplate != "" {
		// Since no fields are defined, the template will render with empty values
		// This is correct behavior - if no fields are defined, templates won't render properly
		processor := templating.NewProcessor()
		renderedTargetDir, err := processor.ProcessTemplate(targetDirTemplate, ".", nil, userValues)
		if err != nil {
			// If rendering fails, fall back to the template as-is
			suggestedDir = targetDirTemplate
		} else {
			suggestedDir = renderedTargetDir
		}
	}

	// Second form to get target directory with rendered suggestion
	var targetPath string
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

	// Now call the generate function with the selected template, target, and collected values
	return generateProject(selectedTemplate, targetPath, false, false, false, 50, userValues)
}

// validateScaffoldTemplate validates that the template being used matches the template specified in .atmos/scaffold.yaml
func validateScaffoldTemplate(targetPath, templateName string) error {
	// Check if .atmos/scaffold.yaml exists
	scaffoldConfigPath := filepath.Join(targetPath, ".atmos", config.ScaffoldConfigFileName)
	if _, err := os.Stat(scaffoldConfigPath); os.IsNotExist(err) {
		// File doesn't exist, no validation needed
		return nil
	}

	// Read the scaffold configuration file
	v := viper.New()
	v.SetConfigFile(scaffoldConfigPath)

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read .atmos/%s: %w", config.ScaffoldConfigFileName, err)
	}

	// Check if template key exists
	if !v.IsSet("template") {
		return fmt.Errorf(".atmos/%s exists but missing required 'template' key", config.ScaffoldConfigFileName)
	}

	// Get the template name from the file
	configuredTemplate := v.GetString("template")
	if configuredTemplate == "" {
		return fmt.Errorf(".atmos/%s exists but 'template' key is empty", config.ScaffoldConfigFileName)
	}

	// Validate that the template matches
	if configuredTemplate != templateName {
		return fmt.Errorf("template mismatch: trying to use template '%s' but .atmos/%s specifies template '%s'", templateName, config.ScaffoldConfigFileName, configuredTemplate)
	}

	return nil
}
