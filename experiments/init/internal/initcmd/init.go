package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/experiments/init/embeds"
	"github.com/cloudposse/atmos/experiments/init/internal/ui"
)

// generateHelpText creates dynamic help text based on available configurations
func generateHelpText() string {
	configs, err := embeds.GetAvailableConfigurations()
	if err != nil {
		// Fallback to static help if we can't get configurations
		return `The 'atmos init' command initializes configurations.

Usage: atmos init [configuration] [target path]
    (default)                   Initialize a typical project for atmos
    atmos.yaml                  Initialize a local atmos CLI configuration file
    .editorconfig               Initialize a local Editor Config file
    .gitignore                  Initialize a recommend Git ignore file.
    examples/demo-stacks        Demonstration of using Atmos stacks
    examples/demo-localstack    Demonstration of using Atmos with localstack
    examples/demo-helmfile      Demonstration of using Atmos with Helmfile
    ...etc

Examples:
- Initialize a typical project for atmos
  $ atmos init

- Initialize an 'atmos.yaml' CLI configuration file
  $ atmos init atmos.yaml

- Initialize the atmos.yml in the ./ location
  $ atmos init atmos.yaml

- Initialize the atmos.yaml as /tmp/atmos.yaml
  $ atmos init atmos.yaml /tmp/atmos.yaml

- Initialize the Localstack demo in the ./examples/demo-localstack folder
  $ atmos init examples/demo-localstack

- Or, simply install it into the current path
  $ atmos init examples/demo-localstack ./demo`
	}

	var helpText strings.Builder
	helpText.WriteString("The 'atmos init' command initializes configurations.\n\n")
	helpText.WriteString("Usage: atmos init [configuration] [target path]\n")

	// Add available configurations
	for name, cfg := range configs {
		helpText.WriteString(fmt.Sprintf("    %-25s %s\n", name, cfg.Description))
	}

	helpText.WriteString("\nExamples:\n")
	helpText.WriteString("- Initialize a typical project for atmos\n")
	helpText.WriteString("  $ atmos init default\n\n")
	helpText.WriteString("- Initialize an 'atmos.yaml' CLI configuration file\n")
	helpText.WriteString("  $ atmos init atmos.yaml\n\n")
	helpText.WriteString("- Initialize the atmos.yml in the ./ location\n")
	helpText.WriteString("  $ atmos init atmos.yaml\n\n")
	helpText.WriteString("- Initialize the atmos.yaml as /tmp/atmos.yaml\n")
	helpText.WriteString("  $ atmos init atmos.yaml /tmp/atmos.yaml\n\n")
	helpText.WriteString("- Initialize the Localstack demo in the ./examples/demo-localstack folder\n")
	helpText.WriteString("  $ atmos init examples/demo-localstack\n\n")
	helpText.WriteString("- Or, simply install it into the current path\n")
	helpText.WriteString("  $ atmos init examples/demo-localstack ./demo\n\n")
	helpText.WriteString("- Force overwrite existing files\n")
	helpText.WriteString("  $ atmos init default --force\n\n")
	helpText.WriteString("- Update existing files with 3-way merge\n")
	helpText.WriteString("  $ atmos init default --update\n\n")
	helpText.WriteString("- Set template values via command line\n")
	helpText.WriteString("  $ atmos init rich-project --values author=John --values year=2024 --values license=MIT\n\n")
	helpText.WriteString("- Set template values and skip prompts\n")
	helpText.WriteString("  $ atmos init rich-project --values project_name=my-project --values cloud_provider=aws --values enable_monitoring=true\n\n")
	helpText.WriteString("- Use default values without prompting\n")
	helpText.WriteString("  $ atmos init rich-project --use-defaults")

	return helpText.String()
}

// NewInitCmd creates the init command
func NewInitCmd() *cobra.Command {
	var force, update, useDefaults bool
	var templateValues []string
	var threshold int

	initCmd := &cobra.Command{
		Use:   "init [configuration] [target path]",
		Short: "Initialize configurations and examples",
		Long:  generateHelpText(),
		Args:  cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeInit(cmd, args, force, update, useDefaults, templateValues, threshold)
		},
	}

	// Add flags
	initCmd.Flags().BoolVarP(&force, "force", "f", false, "Force overwrite existing files")
	initCmd.Flags().BoolVarP(&update, "update", "u", false, "Attempt 3-way merge for existing files")
	initCmd.Flags().BoolVar(&useDefaults, "use-defaults", false, "Use default values without prompting")
	initCmd.Flags().StringArrayVarP(&templateValues, "values", "V", []string{}, "Set template values (format: key=value, can be specified multiple times)")
	initCmd.Flags().IntVar(&threshold, "threshold", 0, "Percentage threshold for 3-way merge (0-100, 0 = use default 50%%)")
	initCmd.MarkFlagsMutuallyExclusive("force", "update")

	return initCmd
}

// promptForConfigurationAndPath prompts the user to select a configuration and target path
func promptForConfigurationAndPath(configs map[string]embeds.Configuration) (string, string, error) {
	// Build config keys for consistent ordering (basic ones first)
	var configKeys []string
	basicConfigs := []string{"default", "rich-project", "atmos.yaml", ".editorconfig", ".gitignore"}

	// Add basic configs first if they exist
	for _, key := range basicConfigs {
		if _, exists := configs[key]; exists {
			configKeys = append(configKeys, key)
		}
	}

	// Add remaining configs (examples, demos, etc.)
	for key := range configs {
		// Skip if already added in basic configs
		found := false
		for _, basicKey := range configKeys {
			if key == basicKey {
				found = true
				break
			}
		}
		if !found {
			configKeys = append(configKeys, key)
		}
	}

	// Create a select with the table data
	var options []huh.Option[string]
	for _, key := range configKeys {
		config := configs[key]
		// Format as "ID   Name   Description" for better readability
		displayText := fmt.Sprintf("%-15s   %-35s   %s", key, config.Name, config.Description)
		options = append(options, huh.NewOption(displayText, key))
	}

	var selectedConfig string

	// Create the selection form
	selectionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a configuration to initialize").
				Description("Choose from the available project templates and configurations (press 'q' to quit)").
				Options(options...).
				Value(&selectedConfig),
		),
	)

	err := selectionForm.Run()
	if err != nil {
		return "", "", err
	}

	// Display selected template details
	selectedTemplate := configs[selectedConfig]
	fmt.Println()

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)

	// Display the template details
	fmt.Println(descStyle.Render(selectedTemplate.Name))
	if selectedTemplate.Description != "" {
		fmt.Println(descStyle.Render(selectedTemplate.Description))
	}
	fmt.Println()

	// Generate suggested directory name based on template
	suggestedDir := generateSuggestedDirectory(selectedConfig, configs[selectedConfig])
	targetPath := suggestedDir

	// Second form to get target directory with smart default
	pathForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Target directory").
				Description(fmt.Sprintf("Where should the files be created? (suggested: %s)", suggestedDir)).
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
		return "", "", err
	}

	// Use suggested directory if empty
	if targetPath == "" {
		targetPath = suggestedDir
	}

	return selectedConfig, targetPath, nil
}

// generateSuggestedDirectory creates a sensible default directory name based on the template configuration
func generateSuggestedDirectory(configName string, config embeds.Configuration) string {
	// If the template specifies a target_dir, use that
	if config.TargetDir != "" {
		return config.TargetDir
	}

	// Check if this template creates only single files at the root level
	// If so, and it's a single file, suggest current directory
	if len(config.Files) == 1 {
		file := config.Files[0]
		// If it's a single file at the root (no subdirectories), use current directory
		if !strings.Contains(file.Path, "/") {
			return "."
		}
	}

	// For all other cases, use the template folder name as the suggested directory
	return "./" + filepath.Base(configName)
}

func executeInit(cmd *cobra.Command, args []string, force, update, useDefaults bool, templateValues []string, threshold int) error {
	// Get available configurations first
	configs, err := embeds.GetAvailableConfigurations()
	if err != nil {
		return fmt.Errorf("failed to get available configurations: %w", err)
	}

	var configName, targetPath string

	// Handle interactive mode when no arguments provided
	if len(args) == 0 {
		selectedConfig, selectedPath, err := promptForConfigurationAndPath(configs)
		if err != nil {
			return fmt.Errorf("failed to prompt for configuration: %w", err)
		}
		configName = selectedConfig
		targetPath = selectedPath
	} else {
		// Parse arguments when provided
		configName = args[0]
		targetPath = "."
		if len(args) == 2 {
			targetPath = args[1]
		}
	}

	// Resolve target path
	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %w", err)
	}

	// Find the requested configuration
	config, exists := configs[configName]
	if !exists {
		return fmt.Errorf("configuration '%s' not found", configName)
	}

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(absTargetPath, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Parse template values from command line
	parsedTemplateValues, err := parseTemplateValues(templateValues)
	if err != nil {
		return fmt.Errorf("failed to parse template values: %w", err)
	}

	// Initialize the UI with custom threshold if specified
	initUI := ui.NewInitUI()
	if threshold > 0 {
		initUI.SetThresholdPercent(threshold)
	}

	// Execute the initialization
	return initUI.Execute(config, absTargetPath, force, update, useDefaults, parsedTemplateValues)
}

// parseTemplateValues parses template values from command line arguments
// Format: key=value,key2=value2
func parseTemplateValues(templateValues []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, templateValue := range templateValues {
		// Check for multiple equals signs first
		if strings.Count(templateValue, "=") != 1 {
			return nil, fmt.Errorf("invalid template value format: %s (expected key=value)", templateValue)
		}

		// Split on equals sign
		parts := strings.SplitN(templateValue, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid template value format: %s (expected key=value)", templateValue)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Check for empty key (including whitespace-only keys)
		if key == "" {
			return nil, fmt.Errorf("empty key in template value: %s", templateValue)
		}

		// Try to parse as different types
		parsedValue, err := parseValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse value for key '%s': %w", key, err)
		}

		result[key] = parsedValue
	}

	return result, nil
}

// parseValue attempts to parse a string value into the most appropriate type
func parseValue(value string) (interface{}, error) {
	// Try to parse as boolean first
	switch strings.ToLower(value) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	}

	// Try to parse as number
	if strings.Contains(value, ".") {
		// Try float
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f, nil
		}
	} else {
		// Try int
		if i, err := strconv.Atoi(value); err == nil {
			return i, nil
		}
	}

	// If all else fails, treat as string
	return value, nil
}
