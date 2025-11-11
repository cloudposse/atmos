package scaffold

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/generator/setup"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/project/config"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
)

//go:embed scaffold-schema.json
var scaffoldSchemaData string

var scaffoldGenerateParser *flags.StandardParser

// Valid prompt types for scaffold configuration.
var validPromptTypes = []string{"input", "select", "confirm", "multiselect"}

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

		v := viper.GetViper()
		if err := scaffoldGenerateParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values with proper precedence: flag > env > config > default
		force := v.GetBool("force")
		dryRun := v.GetBool("dry-run")

		// Parse string map from --set flags
		templateValuesMap := flags.ParseStringMap(v, "set")

		// Validate --set flags were properly formatted
		// Get raw flag values to check for malformed entries
		if cmd.Flags().Changed("set") {
			rawSetFlags, _ := cmd.Flags().GetStringSlice("set")
			for _, entry := range rawSetFlags {
				if !strings.Contains(entry, "=") {
					log.Warn("Malformed --set flag ignored (missing '='): " + entry)
				} else {
					parts := strings.SplitN(entry, "=", 2)
					if strings.TrimSpace(parts[0]) == "" {
						log.Warn("Malformed --set flag ignored (empty key): " + entry)
					}
				}
			}
		}

		// Convert map[string]string to map[string]interface{} for template engine
		templateValues := make(map[string]interface{})
		for k, val := range templateValuesMap {
			templateValues[k] = val
		}

		return executeScaffoldGenerate(
			cmd,
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
		return executeScaffoldList(cmd)
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
	// Create StandardParser for generate subcommand flags
	scaffoldGenerateParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Overwrite existing files"),
		flags.WithBoolFlag("dry-run", "", false, "Preview changes without writing files"),
		flags.WithStringMapFlag("set", "", map[string]string{}, "Set template values (can be used multiple times: --set key=value)"),
		flags.WithEnvVars("force", "ATMOS_SCAFFOLD_FORCE"),
		flags.WithEnvVars("dry-run", "ATMOS_SCAFFOLD_DRY_RUN"),
		flags.WithEnvVars("set", "ATMOS_SCAFFOLD_SET"),
	)

	// Register flags to generate subcommand
	scaffoldGenerateParser.RegisterFlags(scaffoldGenerateCmd)

	// Bind to Viper for precedence handling
	_ = scaffoldGenerateParser.BindToViper(viper.GetViper())

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

// GetFlagsBuilder returns the flags builder for this command.
func (s *ScaffoldCommandProvider) GetFlagsBuilder() flags.Builder {
	return scaffoldGenerateParser
}

// GetPositionalArgsBuilder returns nil as this command doesn't use positional args builder.
func (s *ScaffoldCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns nil as this command doesn't need compatibility flags.
func (s *ScaffoldCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// executeScaffoldGenerate generates code from a scaffold template.
// This logic was moved from internal/exec/scaffold.go to keep command logic in cmd/.
func executeScaffoldGenerate(
	cmd *cobra.Command,
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
			return errUtils.Build(errUtils.ErrResolveTargetDirectory).
				WithExplanationf("Cannot resolve target directory path: `%s`", targetDir).
				WithHint("Ensure the path is valid").
				WithHint("Check that the parent directory exists and is accessible").
				WithContext("target_dir", targetDir).
				WithExitCode(2).
				Err()
		}
	}

	// Create generator context
	genCtx, err := setup.NewGeneratorContext()
	if err != nil {
		return errUtils.Build(errUtils.ErrCreateGeneratorContext).
			WithExplanation("Failed to initialize generator context").
			WithHint("Check terminal capabilities and I/O permissions").
			WithHint("Try running with `ATMOS_LOGS_LEVEL=Debug` for more details").
			WithExitCode(1).
			Err()
	}

	scaffoldUI := genCtx.UI

	// Load embedded templates first
	configs, err := templates.GetAvailableConfigurations()
	if err != nil {
		return errUtils.Build(errUtils.ErrLoadScaffoldTemplates).
			WithExplanation("Failed to load available scaffold templates").
			WithHint("Run `atmos scaffold list` to see available templates").
			WithHint("Check that embedded templates are included in the binary").
			WithContext("templates_loaded", len(configs)).
			WithExitCode(1).
			Err()
	}

	// Load and merge scaffold templates from atmos.yaml
	scaffoldSection, err := config.ReadAtmosScaffoldSection(".")
	if err != nil {
		return errUtils.Build(errUtils.ErrReadScaffoldConfig).
			WithExplanation("Failed to read `scaffold` section from `atmos.yaml`").
			WithHint("Check the `scaffold` section syntax in `atmos.yaml`").
			WithHint("Run `atmos validate config` to verify configuration syntax").
			WithContext("config_file", "atmos.yaml").
			WithContext("section", "scaffold").
			WithExitCode(2).
			Err()
	}

	// Merge configured templates (they take precedence over embedded)
	if templatesData, ok := scaffoldSection["templates"]; ok {
		if templatesMap, ok := templatesData.(map[string]interface{}); ok {
			for templateName, templateData := range templatesMap {
				// Convert atmos.yaml scaffold template to Configuration
				config, err := convertScaffoldTemplateToConfiguration(templateName, templateData)
				if err != nil {
					// Log error but continue with other templates
					atmosui.Warning(fmt.Sprintf("Failed to load scaffold template '%s': %v", templateName, err))
					continue
				}
				// Configured templates override embedded templates
				configs[templateName] = config
			}
		}
	}

	// Handle template selection
	var selectedConfig templates.Configuration
	if templateName == "" {
		// Interactive template selection
		selectedName, err := scaffoldUI.PromptForTemplate("scaffold", configs)
		if err != nil {
			return errUtils.Build(errUtils.ErrPromptFailed).
				WithExplanation("Interactive template selection failed").
				WithHint("Interactive prompts require a TTY (terminal)").
				WithHint("Use non-interactive mode: `atmos scaffold generate <template> <target>`").
				WithHint("Or set `ATMOS_FORCE_TTY=true` if running in a compatible environment").
				WithContext("mode", "interactive").
				WithExitCode(1).
				Err()
		}
		selectedConfig = configs[selectedName]
	} else {
		// Use specified template
		config, exists := configs[templateName]
		if !exists {
			// Get list of available templates for better error message
			availableTemplates := make([]string, 0, len(configs))
			for name := range configs {
				availableTemplates = append(availableTemplates, name)
			}
			return errUtils.Build(errUtils.ErrScaffoldNotFound).
				WithExplanationf("Scaffold template `%s` not found", templateName).
				WithHint("Run `atmos scaffold list` to see available templates").
				WithHint("Check the `scaffold.templates` section in your `atmos.yaml`").
				WithHint("Verify the template name is spelled correctly").
				WithContext("template", templateName).
				WithContext("available_templates", strings.Join(availableTemplates, ", ")).
				WithExitCode(2).
				Err()
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
			return errUtils.Build(errUtils.ErrTargetDirRequired).
				WithExplanation("Target directory is required when using `--dry-run` flag").
				WithHint("Specify target directory: `atmos scaffold generate <template> <target>`").
				WithHint("Or remove `--dry-run` flag to use interactive mode").
				WithContext("flag", "dry-run").
				WithExitCode(2).
				Err()
		}
	}

	// Target directory provided, use normal Execute
	return scaffoldUI.Execute(selectedConfig, targetDir, force, update, useDefaults, templateVars)
}

// executeScaffoldList lists all available scaffold templates configured in atmos.yaml.
// This logic was moved from internal/exec/scaffold.go to keep command logic in cmd/.
func executeScaffoldList(cmd *cobra.Command) error {
	// Read scaffold section from atmos.yaml
	scaffoldSection, err := config.ReadAtmosScaffoldSection(".")
	if err != nil {
		return errUtils.Build(errUtils.ErrReadScaffoldConfig).
			WithExplanation("Failed to read `scaffold` section from `atmos.yaml`").
			WithHint("Check the `scaffold` section syntax in `atmos.yaml`").
			WithHint("Run `atmos validate config` to check for errors").
			WithContext("config_file", "atmos.yaml").
			WithExitCode(2).
			Err()
	}

	// Get the templates section
	templatesData, ok := scaffoldSection["templates"]
	if !ok {
		if err := atmosui.Info("No scaffold templates configured in atmos.yaml."); err != nil {
			return err
		}
		if err := atmosui.Info("Add a 'scaffold.templates' section to your atmos.yaml to configure available templates."); err != nil {
			return err
		}
		return nil
	}

	templatesMap, ok := templatesData.(map[string]interface{})
	if !ok {
		return errUtils.Build(errUtils.ErrInvalidScaffoldConfig).
			WithExplanation("The `scaffold.templates` section is not a valid configuration").
			WithHint("The `scaffold.templates` section must be a map of template names to configurations").
			WithExample("```yaml\nscaffold:\n  templates:\n    my-template:\n      description: My template\n      source: ./path\n```").
			WithExitCode(2).
			Err()
	}

	// Check if there are any templates
	if len(templatesMap) == 0 {
		if err := atmosui.Info("No scaffold templates configured in atmos.yaml."); err != nil {
			return err
		}
		if err := atmosui.Info("Add templates to the 'scaffold.templates' section to get started."); err != nil {
			return err
		}
		return nil
	}

	// Create generator context
	genCtx, err := setup.NewGeneratorContext()
	if err != nil {
		return errUtils.Build(errUtils.ErrCreateGeneratorContext).
			WithExplanation("Failed to initialize generator context").
			WithHint("Check terminal capabilities and I/O permissions").
			WithExitCode(1).
			Err()
	}

	uiInstance := genCtx.UI
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
			return errors.Join(errUtils.ErrScaffoldValidation, err)
		}
		scaffoldPaths = paths
	} else {
		// Validate all scaffolds in current directory
		paths, err := findScaffoldFiles(".")
		if err != nil {
			return errors.Join(errUtils.ErrScaffoldValidation, err)
		}
		scaffoldPaths = paths
	}

	if len(scaffoldPaths) == 0 {
		if err := atmosui.Info("No scaffold.yaml files found to validate"); err != nil {
			return err
		}
		return nil
	}

	// Validate each scaffold file
	validCount := 0
	errorCount := 0

	for _, scaffoldPath := range scaffoldPaths {
		if err := atmosui.Infof("Validating %s", scaffoldPath); err != nil {
			return err
		}

		if err := validateScaffoldFile(scaffoldPath); err != nil {
			if writeErr := atmosui.Errorf("%s: %v", scaffoldPath, err); writeErr != nil {
				return writeErr
			}
			errorCount++
		} else {
			if err := atmosui.Successf("%s: valid", scaffoldPath); err != nil {
				return err
			}
			validCount++
		}
	}

	// Print summary
	if err := atmosui.Writeln(""); err != nil {
		return err
	}
	if err := atmosui.Writeln("Validation Summary:"); err != nil {
		return err
	}
	if err := atmosui.Successf("Valid files: %d", validCount); err != nil {
		return err
	}
	if errorCount > 0 {
		if err := atmosui.Errorf("Invalid files: %d", errorCount); err != nil {
			return err
		}
		return errUtils.Build(errUtils.ErrScaffoldValidation).
			WithExplanationf("%d scaffold file(s) failed validation", errorCount).
			WithHint("Review the validation errors above and fix the issues").
			WithHint("Check that all required fields are present: `name`, `prompts`").
			WithHint("Verify prompt types are valid: `input`, `select`, `confirm`, `multiselect`").
			WithContext("valid_count", validCount).
			WithContext("error_count", errorCount).
			WithExitCode(1).
			Err()
	}

	if err := atmosui.Success("All scaffold files are valid"); err != nil {
		return err
	}
	return nil
}

// findScaffoldFiles finds scaffold.yaml files in the given path.
func findScaffoldFiles(path string) ([]string, error) {
	var scaffoldPaths []string

	// Check if path is a file
	info, err := os.Stat(path)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldFileNotFound).
			WithExplanationf("Cannot access path: `%s`", path).
			WithHint("Check that the file or directory exists").
			WithHint("Verify you have read permissions").
			WithContext("path", path).
			WithExitCode(2).
			Err()
	}

	if !info.IsDir() {
		// Single file - check if it's scaffold.yaml
		if strings.HasSuffix(path, "scaffold.yaml") || strings.HasSuffix(path, "scaffold.yml") {
			scaffoldPaths = append(scaffoldPaths, path)
		} else {
			return nil, errUtils.Build(errUtils.ErrInvalidScaffoldFile).
				WithExplanationf("File must be named `scaffold.yaml` or `scaffold.yml`: `%s`", path).
				WithHint("Rename the file to `scaffold.yaml`").
				WithContext("path", path).
				WithExitCode(2).
				Err()
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
			return nil, errUtils.Build(errUtils.ErrScaffoldDirectoryRead).
				WithExplanationf("Cannot read directory: `%s`", path).
				WithHint("Check directory permissions").
				WithHint("Verify the path is a valid directory").
				WithContext("path", path).
				WithExitCode(2).
				Err()
		}
	}

	return scaffoldPaths, nil
}

// validateScaffoldFile validates a single scaffold.yaml file.
func validateScaffoldFile(scaffoldPath string) error {
	// Read scaffold file
	scaffoldData, err := os.ReadFile(scaffoldPath)
	if err != nil {
		return errUtils.Build(errUtils.ErrScaffoldReadFile).
			WithExplanationf("Cannot read file: `%s`", scaffoldPath).
			WithHint("Check file permissions").
			WithHint("Verify the file exists").
			WithContext("path", scaffoldPath).
			WithExitCode(2).
			Err()
	}

	// Parse scaffold configuration to ensure it's valid YAML
	var config ScaffoldConfig
	if err := yaml.Unmarshal(scaffoldData, &config); err != nil {
		return errUtils.Build(errUtils.ErrScaffoldParseYAML).
			WithExplanationf("Invalid YAML syntax in: `%s`", scaffoldPath).
			WithHint("Check for syntax errors (indentation, quotes, colons)").
			WithHint("Use a YAML validator: https://www.yamllint.com/").
			WithExample("```yaml\nname: my-scaffold\ndescription: My scaffold template\nprompts:\n  - name: project_name\n    type: input\n```").
			WithContext("path", scaffoldPath).
			WithExitCode(2).
			Err()
	}

	// Basic validation
	if config.Name == "" {
		return errUtils.Build(errUtils.ErrScaffoldMissingName).
			WithExplanation("Scaffold name is required").
			WithHint("Add a `name` field to your `scaffold.yaml`").
			WithExample("```yaml\nname: my-scaffold\ndescription: My template\n```").
			WithContext("path", scaffoldPath).
			WithExitCode(2).
			Err()
	}

	// Validate prompts
	for i, prompt := range config.Prompts {
		if prompt.Name == "" {
			return errUtils.Build(errUtils.ErrScaffoldInvalidPrompt).
				WithExplanationf("Prompt #%d is missing the `name` field", i+1).
				WithHint("Each prompt must have a `name` field").
				WithExample("```yaml\nprompts:\n  - name: project_name\n    type: input\n    description: Project name\n```").
				WithContext("prompt_index", i).
				WithContext("path", scaffoldPath).
				WithExitCode(2).
				Err()
		}
		if prompt.Type == "" {
			return errUtils.Build(errUtils.ErrScaffoldInvalidPrompt).
				WithExplanationf("Prompt #%d (`%s`) is missing the `type` field", i+1, prompt.Name).
				WithExplanationf("Valid types: `%s`", strings.Join(validPromptTypes, ", ")).
				WithExample("```yaml\nprompts:\n  - name: project_name\n    type: input  # Must be one of: input, select, confirm, multiselect\n```").
				WithContext("prompt_index", i).
				WithContext("prompt_name", prompt.Name).
				WithContext("path", scaffoldPath).
				WithExitCode(2).
				Err()
		}
		// Validate prompt type using validPromptTypes constant
		valid := false
		for _, validType := range validPromptTypes {
			if prompt.Type == validType {
				valid = true
				break
			}
		}
		if !valid {
			return errUtils.Build(errUtils.ErrScaffoldInvalidPrompt).
				WithExplanationf("Prompt #%d (`%s`) has invalid type: `%s`", i+1, prompt.Name, prompt.Type).
				WithExplanationf("Must be one of: `%s`", strings.Join(validPromptTypes, ", ")).
				WithExample("```yaml\nprompts:\n  - name: environment\n    type: select  # Valid: input, select, confirm, multiselect\n    options:\n      - dev\n      - staging\n      - prod\n```").
				WithContext("prompt_index", i).
				WithContext("prompt_name", prompt.Name).
				WithContext("invalid_type", prompt.Type).
				WithContext("valid_types", strings.Join(validPromptTypes, ", ")).
				WithContext("path", scaffoldPath).
				WithExitCode(2).
				Err()
		}
	}

	return nil
}

// convertScaffoldTemplateToConfiguration converts an atmos.yaml scaffold template entry to a templates.Configuration.
func convertScaffoldTemplateToConfiguration(name string, templateData interface{}) (templates.Configuration, error) {
	templateMap, ok := templateData.(map[string]interface{})
	if !ok {
		return templates.Configuration{}, errUtils.Build(errUtils.ErrInvalidTemplateData).
			WithExplanationf("Template `%s` has invalid structure in `atmos.yaml`", name).
			WithHint("Each template must be a map with configuration keys").
			WithExample("```yaml\nscaffold:\n  templates:\n    my-template:\n      description: My template\n      source: ./path/to/template\n      target_dir: ./output\n```").
			WithContext("template_name", name).
			WithExitCode(2).
			Err()
	}

	config := templates.Configuration{
		Name:        name,
		Description: fmt.Sprintf("Scaffold template: %s", name),
		TemplateID:  name,
	}

	// Extract description if provided
	if desc, ok := templateMap["description"].(string); ok {
		config.Description = desc
	}

	// Extract source (for remote templates)
	if source, ok := templateMap["source"].(string); ok {
		// For remote templates, we would need to fetch and process them
		// For now, create a placeholder that indicates this is a remote template
		config.Description = fmt.Sprintf("%s (source: %s)", config.Description, source)
	}

	// Extract target_dir if provided
	if targetDir, ok := templateMap["target_dir"].(string); ok {
		config.TargetDir = targetDir
	}

	// Note: We don't load actual files here since they might be remote or require
	// additional processing. The actual template processing will be handled by
	// the generator when the template is selected.
	return config, nil
}
