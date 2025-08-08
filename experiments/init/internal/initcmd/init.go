package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

	initCmd := &cobra.Command{
		Use:   "init <configuration> [target path]",
		Short: "Initialize configurations and examples",
		Long:  generateHelpText(),
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeInit(cmd, args, force, update, useDefaults, templateValues)
		},
	}

	// Add flags
	initCmd.Flags().BoolVarP(&force, "force", "f", false, "Force overwrite existing files")
	initCmd.Flags().BoolVarP(&update, "update", "u", false, "Attempt 3-way merge for existing files")
	initCmd.Flags().BoolVar(&useDefaults, "use-defaults", false, "Use default values without prompting")
	initCmd.Flags().StringArrayVarP(&templateValues, "values", "V", []string{}, "Set template values (format: key=value, can be specified multiple times)")
	initCmd.MarkFlagsMutuallyExclusive("force", "update")

	return initCmd
}

func executeInit(cmd *cobra.Command, args []string, force, update, useDefaults bool, templateValues []string) error {
	// Parse arguments - Cobra ensures we have at least 1 argument
	configName := args[0]
	targetPath := "."
	if len(args) == 2 {
		targetPath = args[1]
	}

	// Resolve target path
	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %w", err)
	}

	// Get available configurations
	configs, err := embeds.GetAvailableConfigurations()
	if err != nil {
		return fmt.Errorf("failed to get available configurations: %w", err)
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

	// Initialize the UI
	initUI := ui.NewInitUI()

	// Execute the initialization
	return initUI.Execute(config, absTargetPath, force, update, useDefaults, parsedTemplateValues)
}

// parseTemplateValues parses template values from command line arguments
// Format: key=value,key2=value2
func parseTemplateValues(templateValues []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, templateValue := range templateValues {
		// Split on equals sign
		parts := strings.SplitN(templateValue, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid template value format: %s (expected key=value)", templateValue)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

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
	// Try to parse as boolean
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
