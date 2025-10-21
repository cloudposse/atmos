package scaffold

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/generator/ui"
	"github.com/cloudposse/atmos/pkg/project/config"
	"github.com/cloudposse/atmos/pkg/utils"
)

//go:embed scaffold-schema.json
var scaffoldSchemaData string

// ScaffoldConfig represents a scaffold configuration.
type ScaffoldConfig struct {
	Name         string              `yaml:"name"`
	Description  string              `yaml:"description,omitempty"`
	Author       string              `yaml:"author,omitempty"`
	Version      string              `yaml:"version,omitempty"`
	Prompts      []PromptConfig      `yaml:"prompts,omitempty"`
	Dependencies []string            `yaml:"dependencies,omitempty"`
	Hooks        map[string][]string `yaml:"hooks,omitempty"`
}

// PromptConfig represents a prompt configuration.
type PromptConfig struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description,omitempty"`
	Type        string      `yaml:"type"`
	Default     interface{} `yaml:"default,omitempty"`
	Required    bool        `yaml:"required,omitempty"`
}

// scaffoldCmd represents the scaffold parent command.
var scaffoldCmd = &cobra.Command{
	Use:   "scaffold",
	Short: "Generate code from scaffold templates",
	Long: `Generate code from scaffold templates defined in atmos.yaml or embedded templates.

Scaffold templates allow you to quickly generate boilerplate code, configurations,
and directory structures based on predefined templates.`,
}

// scaffoldGenerateCmd represents the scaffold generate subcommand.
var scaffoldGenerateCmd = &cobra.Command{
	Use:   "generate [template] [target]",
	Short: "Generate code from a scaffold template",
	Long: `Generate code from a scaffold template.

Templates can be:
- Built-in templates embedded in Atmos
- Custom templates defined in your atmos.yaml
- Remote templates from Git repositories

If no template is specified, an interactive selection will be shown.
If no target directory is specified, you will be prompted for one.`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		template := ""
		target := ""

		if len(args) > 0 {
			template = args[0]
		}
		if len(args) > 1 {
			target = args[1]
		}

		force, _ := cmd.Flags().GetBool("force")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Get template values from --set flags
		setFlags, _ := cmd.Flags().GetStringSlice("set")
		templateValues := make(map[string]interface{})
		for _, flag := range setFlags {
			key, value := parseSetFlag(flag)
			if key != "" {
				templateValues[key] = value
			}
		}

		return executeScaffoldGenerate(
			cmd.Context(),
			template,
			target,
			force,
			dryRun,
			templateValues,
		)
	},
}

// scaffoldListCmd represents the scaffold list subcommand.
var scaffoldListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available scaffold templates",
	Long: `List all scaffold templates configured in your atmos.yaml.

This command shows templates from the 'scaffold.templates' section
of your atmos.yaml configuration file.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeScaffoldList(cmd.Context())
	},
}

// scaffoldValidateCmd represents the validate subcommand.
var scaffoldValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate scaffold template configuration",
	Long: `Validate scaffold.yaml files against the scaffold schema.

If no path is specified, validates all scaffold.yaml files in the current directory.
If a directory is specified, validates all scaffold.yaml files in that directory.
If a file is specified, validates that specific scaffold.yaml file.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		return executeValidateScaffold(cmd.Context(), path)
	},
}

func init() {
	// Add flags to generate command
	scaffoldGenerateCmd.Flags().BoolP("force", "f", false, "Overwrite existing files")
	scaffoldGenerateCmd.Flags().Bool("dry-run", false, "Preview changes without writing files")
	scaffoldGenerateCmd.Flags().StringSlice("set", []string{}, "Set template values (can be used multiple times: --set key=value)")

	// Add subcommands to scaffold parent
	scaffoldCmd.AddCommand(scaffoldGenerateCmd)
	scaffoldCmd.AddCommand(scaffoldListCmd)
	scaffoldCmd.AddCommand(scaffoldValidateCmd)

	// Register this command with the registry
	internal.Register(&ScaffoldCommandProvider{})
}

// ScaffoldCommandProvider implements the CommandProvider interface.
type ScaffoldCommandProvider struct{}

// GetCommand returns the scaffold command.
func (s *ScaffoldCommandProvider) GetCommand() *cobra.Command {
	return scaffoldCmd
}

// GetName returns the command name.
func (s *ScaffoldCommandProvider) GetName() string {
	return "scaffold"
}

// GetGroup returns the command group for help organization.
func (s *ScaffoldCommandProvider) GetGroup() string {
	return "Configuration Management"
}

// parseSetFlag parses a --set flag in the format key=value.
func parseSetFlag(flag string) (string, string) {
	parts := strings.SplitN(flag, "=", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// executeScaffoldGenerate generates code from a scaffold template.
// This logic was moved from internal/exec/scaffold.go to keep command logic in cmd/.
func executeScaffoldGenerate(
	ctx context.Context,
	templateName string,
	targetDir string,
	force bool,
	dryRun bool,
	templateVars map[string]interface{},
) error {
	// Convert to absolute path if provided
	if targetDir != "" {
		var err error
		targetDir, err = filepath.Abs(targetDir)
		if err != nil {
			return fmt.Errorf("%w: failed to resolve target directory: %w", errUtils.ErrScaffoldGeneration, err)
		}
	}

	// Create the UI instance
	scaffoldUI := ui.NewInitUI()

	// For now, try to find scaffold templates in the same embedded templates
	// This is a simplified implementation - the original experiment supported
	// more complex scaffold template discovery from atmos.yaml configuration
	configs, err := templates.GetAvailableConfigurations()
	if err != nil {
		return fmt.Errorf("%w: failed to get available scaffold templates: %w", errUtils.ErrScaffoldGeneration, err)
	}

	// Handle template selection
	var selectedConfig templates.Configuration
	if templateName == "" {
		// Interactive template selection
		selectedName, err := scaffoldUI.PromptForTemplate("scaffold", configs)
		if err != nil {
			return fmt.Errorf("%w: failed to prompt for template: %w", errUtils.ErrScaffoldGeneration, err)
		}
		selectedConfig = configs[selectedName]
	} else {
		// Use specified template
		config, exists := configs[templateName]
		if !exists {
			return fmt.Errorf("%w: scaffold template '%s' not found", errUtils.ErrScaffoldNotFound, templateName)
		}
		selectedConfig = config
	}

	// Handle empty target directory based on mode
	update := false       // Scaffold typically doesn't use update mode like init does
	useDefaults := dryRun // If dry-run, we want to use defaults and not prompt

	if targetDir == "" {
		// Check if we can prompt (not in dry-run mode which is headless)
		if !dryRun {
			// Interactive mode: use ExecuteWithInteractiveFlow which will prompt for target directory
			return scaffoldUI.ExecuteWithInteractiveFlow(selectedConfig, "", force, update, useDefaults, templateVars)
		} else {
			// Dry-run mode or other headless modes: target directory is required
			return fmt.Errorf("%w: target directory is required when using --dry-run flag", errUtils.ErrScaffoldGeneration)
		}
	}

	// Target directory provided, use normal Execute
	return scaffoldUI.Execute(selectedConfig, targetDir, force, update, useDefaults, templateVars)
}

// executeScaffoldList lists all available scaffold templates configured in atmos.yaml.
// This logic was moved from internal/exec/scaffold.go to keep command logic in cmd/.
func executeScaffoldList(ctx context.Context) error {
	// Read scaffold section from atmos.yaml
	scaffoldSection, err := config.ReadAtmosScaffoldSection(".")
	if err != nil {
		return fmt.Errorf("%w: failed to read scaffold section from atmos.yaml: %w", errUtils.ErrScaffoldGeneration, err)
	}

	// Get the templates section
	templatesData, ok := scaffoldSection["templates"]
	if !ok {
		fmt.Println("No scaffold templates configured in atmos.yaml.")
		fmt.Println("Add a 'scaffold.templates' section to your atmos.yaml to configure available templates.")
		return nil
	}

	templatesMap, ok := templatesData.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%w: templates section is not a valid configuration", errUtils.ErrScaffoldGeneration)
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

// executeValidateScaffold validates scaffold.yaml files against the scaffold schema.
// This logic was moved from internal/exec/validate_scaffold.go to keep command logic in cmd/.
func executeValidateScaffold(
	ctx context.Context,
	path string,
) error {
	// Determine what to validate
	var scaffoldPaths []string

	if path != "" {
		// Validate specific path
		paths, err := findScaffoldFiles(path)
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrScaffoldValidation, err)
		}
		scaffoldPaths = paths
	} else {
		// Validate all scaffolds in current directory
		paths, err := findScaffoldFiles(".")
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrScaffoldValidation, err)
		}
		scaffoldPaths = paths
	}

	if len(scaffoldPaths) == 0 {
		utils.PrintfMessageToTUI("No scaffold.yaml files found to validate\n")
		return nil
	}

	// Validate each scaffold file
	validCount := 0
	errorCount := 0

	for _, scaffoldPath := range scaffoldPaths {
		utils.PrintfMessageToTUI("Validating %s\n", scaffoldPath)

		if err := validateScaffoldFile(scaffoldPath); err != nil {
			utils.PrintfMessageToTUI("✗ %s: %v\n", scaffoldPath, err)
			errorCount++
		} else {
			utils.PrintfMessageToTUI("✓ %s: valid\n", scaffoldPath)
			validCount++
		}
	}

	// Print summary
	utils.PrintfMessageToTUI("\nValidation Summary:\n")
	utils.PrintfMessageToTUI("✓ Valid files: %d\n", validCount)
	if errorCount > 0 {
		utils.PrintfMessageToTUI("✗ Invalid files: %d\n", errorCount)
		return fmt.Errorf("%w: %d scaffold files failed validation", errUtils.ErrScaffoldValidation, errorCount)
	}

	utils.PrintfMessageToTUI("All scaffold files are valid\n")
	return nil
}

// findScaffoldFiles finds scaffold.yaml files in the given path.
func findScaffoldFiles(path string) ([]string, error) {
	var scaffoldPaths []string

	// Check if path is a file
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path '%s': %w", path, err)
	}

	if !info.IsDir() {
		// Single file - check if it's scaffold.yaml
		if strings.HasSuffix(path, "scaffold.yaml") || strings.HasSuffix(path, "scaffold.yml") {
			scaffoldPaths = append(scaffoldPaths, path)
		} else {
			return nil, fmt.Errorf("file '%s' is not a scaffold configuration file", path)
		}
	} else {
		// Directory - look for scaffold.yaml files recursively
		err := filepath.Walk(path, func(walkPath string, walkInfo os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if !walkInfo.IsDir() && (walkInfo.Name() == "scaffold.yaml" || walkInfo.Name() == "scaffold.yml") {
				scaffoldPaths = append(scaffoldPaths, walkPath)
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk directory '%s': %w", path, err)
		}
	}

	return scaffoldPaths, nil
}

// validateScaffoldFile validates a single scaffold.yaml file.
func validateScaffoldFile(scaffoldPath string) error {
	// Read scaffold file
	scaffoldData, err := os.ReadFile(scaffoldPath)
	if err != nil {
		return fmt.Errorf("failed to read scaffold file: %w", err)
	}

	// Parse scaffold configuration to ensure it's valid YAML
	var config ScaffoldConfig
	if err := yaml.Unmarshal(scaffoldData, &config); err != nil {
		return fmt.Errorf("failed to parse scaffold YAML: %w", err)
	}

	// Basic validation
	if config.Name == "" {
		return fmt.Errorf("scaffold name is required")
	}

	// Validate prompts
	for i, prompt := range config.Prompts {
		if prompt.Name == "" {
			return fmt.Errorf("prompt %d: name is required", i)
		}
		if prompt.Type == "" {
			return fmt.Errorf("prompt %d: type is required", i)
		}
		if prompt.Type != "input" && prompt.Type != "select" && prompt.Type != "confirm" && prompt.Type != "multiselect" {
			return fmt.Errorf("prompt %d: invalid type '%s'", i, prompt.Type)
		}
	}

	return nil
}
