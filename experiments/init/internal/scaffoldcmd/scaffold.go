package scaffoldcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/experiments/init/embeds"
	"github.com/cloudposse/atmos/experiments/init/internal/config"
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
		return generateFromRemote(templatePath, targetPath, force, update, useDefaults, cmdValues, ui, []string{"{{", "}}"})
	} else {
		return generateFromLocal(templatePath, targetPath, force, update, useDefaults, cmdValues, ui, []string{"{{", "}}"})
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

	// If no target path was provided, prompt for it after setup
	if targetPath == "" {
		// Check if source is a local filesystem path
		if !isRemoteTemplate(source) {
			// Load the local template to check if it has scaffold configuration
			config, err := loadLocalTemplate(source, templateName)
			if err != nil {
				return fmt.Errorf("failed to load template: %w", err)
			}

			// For templates with scaffold configuration, we need to run setup first
			if embeds.HasScaffoldConfig(config.Files) {
				// Create a temporary directory for setup
				tempDir, err := os.MkdirTemp("", "atmos-scaffold-*")
				if err != nil {
					return fmt.Errorf("failed to create temporary directory: %w", err)
				}
				defer os.RemoveAll(tempDir)

				// Run setup to get configuration values
				mergedValues, _, err := ui.RunSetupForm(nil, tempDir, useDefaults, cmdValues)
				if err != nil {
					return fmt.Errorf("failed to run setup form: %w", err)
				}

				// Now prompt for target directory with evaluated template
				targetPath, err = ui.PromptForTargetDirectory(templateMap, mergedValues)
				if err != nil {
					return fmt.Errorf("failed to prompt for target directory: %w", err)
				}
			} else {
				// For simple templates, prompt directly
				targetPath, err = ui.PromptForTargetDirectory(templateMap, nil)
				if err != nil {
					return fmt.Errorf("failed to prompt for target directory: %w", err)
				}
			}
		} else {
			// For remote templates, prompt with basic template info
			targetPath, err = ui.PromptForTargetDirectory(templateMap, nil)
			if err != nil {
				return fmt.Errorf("failed to prompt for target directory: %w", err)
			}
		}
	}

	// Validate scaffold template if .atmos/scaffold.yaml exists
	if err := validateScaffoldTemplate(targetPath, templateName); err != nil {
		return fmt.Errorf("scaffold validation failed: %w", err)
	}

	// Get delimiters from template configuration
	var delimiters []string
	if delims, exists := templateMap["delimiters"]; exists {
		if delimsSlice, ok := delims.([]interface{}); ok && len(delimsSlice) == 2 {
			delimiters = []string{delimsSlice[0].(string), delimsSlice[1].(string)}
		}
	}
	// Use default delimiters if not specified
	if len(delimiters) == 0 {
		delimiters = []string{"{{", "}}"}
	}

	// Check if source is a local filesystem path
	if !isRemoteTemplate(source) {
		// Treat as local template
		return generateFromLocal(source, targetPath, force, update, useDefaults, cmdValues, ui, delimiters)
	}

	// For remote sources, treat as remote templates
	return generateFromRemote(source, targetPath, force, update, useDefaults, cmdValues, ui, delimiters)
}

// generateFromLocal handles generation from a local filesystem path
func generateFromLocal(templatePath, targetPath string, force, update, useDefaults bool, cmdValues map[string]interface{}, ui *ui.InitUI, delimiters []string) error {
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
	// For non-scaffold templates, use the directory name as the template key
	templateKey := filepath.Base(absTemplatePath)
	config, err := loadLocalTemplate(absTemplatePath, templateKey)
	if err != nil {
		return fmt.Errorf("failed to load template: %w", err)
	}

	// Execute the template generation using the existing UI logic
	// Pass delimiters to the UI for template processing
	return ui.ExecuteWithDelimiters(*config, targetPath, force, update, useDefaults, cmdValues, delimiters)
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

	// Use the UI package to display the table
	uiInstance := ui.NewInitUI()
	uiInstance.DisplayScaffoldTemplateTable(templatesMap)

	return nil
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

	// Use the UI package to prompt for template selection
	uiInstance := ui.NewInitUI()
	selectedTemplate, err := uiInstance.PromptForTemplate("scaffold", templatesMap)
	if err != nil {
		return err
	}

	// Now call the generate function with just the selected template
	// The target directory will be prompted for after setup
	return generateProject(selectedTemplate, "", false, false, false, 50, make(map[string]interface{}))
}

// validateScaffoldTemplate validates that the template being used matches the template specified in .atmos/scaffold.yaml
func validateScaffoldTemplate(targetPath, templateName string) error {
	// Check if .atmos/scaffold.yaml exists
	scaffoldConfigPath := filepath.Join(targetPath, config.ScaffoldConfigDir, config.ScaffoldConfigFileName)
	if _, err := os.Stat(scaffoldConfigPath); os.IsNotExist(err) {
		// File doesn't exist, no validation needed
		return nil
	}

	// Read the scaffold configuration file
	v := viper.New()
	v.SetConfigFile(scaffoldConfigPath)

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read %s/%s: %w", config.ScaffoldConfigDir, config.ScaffoldConfigFileName, err)
	}

	// Check if template key exists
	if !v.IsSet("template") {
		return fmt.Errorf("%s/%s exists but missing required 'template' key", config.ScaffoldConfigDir, config.ScaffoldConfigFileName)
	}

	// Get the template name from the file
	configuredTemplate := v.GetString("template")
	if configuredTemplate == "" {
		return fmt.Errorf("%s/%s exists but 'template' key is empty", config.ScaffoldConfigDir, config.ScaffoldConfigFileName)
	}

	// Validate that the template matches
	if configuredTemplate != templateName {
		return fmt.Errorf("template mismatch: trying to use template '%s' but %s/%s specifies template '%s'", templateName, config.ScaffoldConfigDir, config.ScaffoldConfigFileName, configuredTemplate)
	}

	return nil
}
