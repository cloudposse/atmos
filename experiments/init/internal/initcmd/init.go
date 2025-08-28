package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/experiments/init/embeds"
	"github.com/cloudposse/atmos/experiments/init/internal/config"
	"github.com/cloudposse/atmos/experiments/init/internal/ui"
	"github.com/cloudposse/atmos/experiments/init/internal/utils"
	"github.com/spf13/cobra"
)

// NewInitCmd creates the init command
func NewInitCmd() *cobra.Command {
	var force, update, useDefaults bool
	var templateValues []string
	var threshold int

	initCmd := &cobra.Command{
		Use:   "init [configuration] [target path]",
		Short: "Initialize configurations and examples",
		Long: func() string {
			// Get available configurations dynamically
			configs, err := embeds.GetAvailableConfigurations()
			if err != nil {
				// Fallback to static help if we can't get configurations
				return `The 'atmos init' command initializes configurations and examples.

Available configurations include:
- default: Initialize a typical project for atmos
- atmos.yaml: Initialize a local atmos CLI configuration file
- .editorconfig: Initialize a local Editor Config file
- .gitignore: Initialize a recommended Git ignore file
- examples/demo-stacks: Demonstration of using Atmos stacks
- examples/demo-localstack: Demonstration of using Atmos with localstack
- rich-project: Rich project template with comprehensive configuration options

Examples:
- Initialize a typical project for atmos
  $ atmos init

- Initialize an 'atmos.yaml' CLI configuration file
  $ atmos init atmos.yaml

- Initialize with specific target path
  $ atmos init atmos.yaml /tmp/atmos.yaml

- Force overwrite existing files
  $ atmos init default --force

- Update existing files with 3-way merge
  $ atmos init default --update

- Set template values via command line
  $ atmos init rich-project --values author=John --values year=2024 --values license=MIT

- Use default values without prompting
  $ atmos init rich-project --use-defaults`
			}

			// Build dynamic help text
			var helpText strings.Builder
			helpText.WriteString("The 'atmos init' command initializes configurations and examples.\n\n")
			helpText.WriteString("Available configurations:\n")

			// Sort configurations for consistent ordering
			var configKeys []string
			for key := range configs {
				configKeys = append(configKeys, key)
			}
			sort.Strings(configKeys)

			// Add each configuration with its description
			for _, key := range configKeys {
				config := configs[key]
				helpText.WriteString(fmt.Sprintf("- %s: %s\n", key, config.Description))
			}

			helpText.WriteString("\nExamples:\n")
			helpText.WriteString("- Initialize a typical project for atmos\n")
			helpText.WriteString("  $ atmos init\n\n")
			helpText.WriteString("- Initialize an 'atmos.yaml' CLI configuration file\n")
			helpText.WriteString("  $ atmos init atmos.yaml\n\n")
			helpText.WriteString("- Initialize with specific target path\n")
			helpText.WriteString("  $ atmos init atmos.yaml /tmp/atmos.yaml\n\n")
			helpText.WriteString("- Force overwrite existing files\n")
			helpText.WriteString("  $ atmos init default --force\n\n")
			helpText.WriteString("- Update existing files with 3-way merge\n")
			helpText.WriteString("  $ atmos init default --update\n\n")
			helpText.WriteString("- Set template values via command line\n")
			helpText.WriteString("  $ atmos init rich-project --values author=John --values year=2024 --values license=MIT\n\n")
			helpText.WriteString("- Use default values without prompting\n")
			helpText.WriteString("  $ atmos init rich-project --use-defaults")

			return helpText.String()
		}(),
		Args: cobra.RangeArgs(0, 2),
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

func executeInit(cmd *cobra.Command, args []string, force, update, useDefaults bool, templateValues []string, threshold int) error {
	// Get available configurations first
	configs, err := embeds.GetAvailableConfigurations()
	if err != nil {
		return fmt.Errorf("failed to get available configurations: %w", err)
	}

	var configName, targetPath string

	// Initialize the UI
	initUI := ui.NewInitUI()
	if threshold > 0 {
		initUI.SetThreshold(threshold)
	}

	// Handle interactive mode when no arguments provided
	if len(args) == 0 {
		selectedConfig, err := initUI.PromptForTemplate("embeds", configs)
		if err != nil {
			return fmt.Errorf("failed to prompt for configuration: %w", err)
		}
		configName = selectedConfig
	} else {
		// Parse arguments when provided
		configName = args[0]
		targetPath = "."
		if len(args) == 2 {
			targetPath = args[1]
		}
	}

	// If no configuration specified, prompt for one
	if configName == "" {
		selectedConfig, err := initUI.PromptForTemplate("embeds", configs)
		if err != nil {
			return fmt.Errorf("failed to prompt for configuration: %w", err)
		}
		configName = selectedConfig
	}

	// Get the selected configuration
	embedsConfig, exists := configs[configName]
	if !exists {
		return fmt.Errorf("configuration '%s' not found", configName)
	}

	// Parse template values from command line
	parsedTemplateValues, err := utils.ParseTemplateValues(templateValues)
	if err != nil {
		return fmt.Errorf("failed to parse template values: %w", err)
	}

	// If no target path was provided (interactive mode), prompt for it after setup
	if targetPath == "" {
		// For templates with scaffold configuration, we need to run setup first to get proper values
		if embeds.HasScaffoldConfig(embedsConfig.Files) {
			// Create a temporary directory for setup
			tempDir, err := os.MkdirTemp("", "atmos-setup-*")
			if err != nil {
				return fmt.Errorf("failed to create temporary directory: %w", err)
			}
			defer os.RemoveAll(tempDir)

			// Load the scaffold configuration
			var scaffoldConfigFile *embeds.File
			for i := range embedsConfig.Files {
				if embedsConfig.Files[i].Path == config.ScaffoldConfigFileName {
					scaffoldConfigFile = &embedsConfig.Files[i]
					break
				}
			}

			if scaffoldConfigFile == nil {
				return fmt.Errorf("%s not found in configuration", config.ScaffoldConfigFileName)
			}

			// Load the scaffold configuration from content
			scaffoldConfig, err := config.LoadScaffoldConfigFromContent(scaffoldConfigFile.Content)
			if err != nil {
				return fmt.Errorf("failed to load scaffold configuration: %w", err)
			}

			// Run setup to get configuration values
			mergedValues, _, err := initUI.RunSetupForm(scaffoldConfig, tempDir, useDefaults, parsedTemplateValues)
			if err != nil {
				return fmt.Errorf("failed to run setup form: %w", err)
			}

			// Now prompt for target directory with evaluated template
			targetPath, err = initUI.PromptForTargetDirectory(embedsConfig, mergedValues)
			if err != nil {
				return fmt.Errorf("failed to prompt for target directory: %w", err)
			}
		} else {
			// For simple templates, prompt directly
			targetPath, err = initUI.PromptForTargetDirectory(embedsConfig, nil)
			if err != nil {
				return fmt.Errorf("failed to prompt for target directory: %w", err)
			}
		}
	}

	// Resolve target path
	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %w", err)
	}

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(absTargetPath, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Execute the initialization
	return initUI.Execute(embedsConfig, absTargetPath, force, update, useDefaults, parsedTemplateValues)
}
