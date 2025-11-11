package init

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/generator/setup"
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

var initParser *flags.StandardParser

// initCmd represents the init command.
var initCmd = &cobra.Command{
	Use:   "init [template] [target]",
	Short: "Initialize a new Atmos project from a template",
	Long: `Initialize a new Atmos project from built-in templates.

This command helps you quickly scaffold a new Atmos project with
best-practice configurations and directory structures.

Available templates:
- simple: Basic Atmos project structure
- atmos: Complete atmos.yaml configuration only

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
		if err := initParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values with proper precedence: flag > env > config > default
		force := v.GetBool("force")
		interactive := v.GetBool("interactive")
		update := v.GetBool("update")
		baseRef := v.GetString("base-ref")

		// Parse string map from --set flags
		templateValuesMap := flags.ParseStringMap(v, "set")

		// Convert map[string]string to map[string]interface{} for template engine
		templateValues := make(map[string]interface{})
		for k, val := range templateValuesMap {
			templateValues[k] = val
		}

		return executeInit(
			cmd.Context(),
			template,
			target,
			interactive,
			force,
			update,
			baseRef,
			templateValues,
		)
	},
}

func init() {
	// Create StandardParser with init-specific flags
	initParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Overwrite existing files"),
		flags.WithBoolFlag("interactive", "i", true, "Interactive mode for template selection and configuration"),
		flags.WithBoolFlag("update", "u", false, "Update existing project from template (preserves customizations via 3-way merge)"),
		flags.WithStringMapFlag("set", "", map[string]string{}, "Set template values (can be used multiple times: --set key=value)"),
		flags.WithStringFlag("base-ref", "", "main", "Git reference to use as base for future updates (branch, tag, or commit hash)"),
		flags.WithEnvVars("force", "ATMOS_INIT_FORCE"),
		flags.WithEnvVars("interactive", "ATMOS_INIT_INTERACTIVE"),
		flags.WithEnvVars("update", "ATMOS_INIT_UPDATE"),
		flags.WithEnvVars("set", "ATMOS_INIT_SET"),
		flags.WithEnvVars("base-ref", "ATMOS_INIT_BASE_REF"),
	)

	// Register flags with command
	initParser.RegisterFlags(initCmd)

	// Bind to Viper for precedence handling
	_ = initParser.BindToViper(viper.GetViper())

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&InitCommandProvider{})
}

// InitCommandProvider implements the CommandProvider interface.
type InitCommandProvider struct{}

// GetCommand returns the init command.
func (i *InitCommandProvider) GetCommand() *cobra.Command {
	return initCmd
}

// GetName returns the command name.
func (i *InitCommandProvider) GetName() string {
	return "init"
}

// GetGroup returns the command group for help organization.
func (i *InitCommandProvider) GetGroup() string {
	return "Configuration Management"
}

// GetFlagsBuilder returns the flags builder for this command.
func (i *InitCommandProvider) GetFlagsBuilder() flags.Builder {
	return initParser
}

// GetPositionalArgsBuilder returns nil as this command doesn't use positional args builder.
func (i *InitCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns nil as this command doesn't need compatibility flags.
func (i *InitCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// executeInit initializes a new Atmos project from a template.
// This logic was moved from internal/exec/init.go to keep command logic in cmd/.
func executeInit(
	ctx context.Context,
	templateName string,
	targetDir string,
	interactive bool,
	force bool,
	update bool,
	baseRef string,
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

	initUI := genCtx.UI

	// Get available template configurations
	configs, err := templates.GetAvailableConfigurations()
	if err != nil {
		return errUtils.Build(errUtils.ErrLoadInitTemplates).
			WithExplanation("Failed to load available init templates").
			WithHint("Check that embedded templates are included in the binary").
			WithHint("Try rebuilding Atmos: `make build`").
			WithExitCode(1).
			Err()
	}

	// Handle template selection
	var selectedConfig templates.Configuration
	if templateName == "" {
		// Check if interactive mode is disabled
		if !interactive {
			return errUtils.Build(errUtils.ErrTemplateNameRequired).
				WithExplanation("Template name is required in non-interactive mode").
				WithHint("Specify template name: `atmos init <template>`").
				WithHint("Or use interactive mode: `atmos init --interactive`").
				WithHint("Available templates: `simple`, `atmos`").
				WithExitCode(2).
				Err()
		}

		// Interactive template selection
		selectedName, err := initUI.PromptForTemplate("embeds", configs)
		if err != nil {
			return errUtils.Build(errUtils.ErrPromptFailed).
				WithExplanation("Interactive template selection failed").
				WithHint("Interactive prompts require a TTY (terminal)").
				WithHint("Use non-interactive mode: `atmos init <template> <target>`").
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
			return errUtils.Build(errUtils.ErrScaffoldNotFound).
				WithExplanationf("Template `%s` not found", templateName).
				WithHint("Available templates: `simple`, `atmos`").
				WithHint("Use interactive mode to browse: `atmos init`").
				WithContext("template", templateName).
				WithContext("available_templates", "simple, atmos").
				WithExitCode(2).
				Err()
		}
		selectedConfig = config
	}

	// Handle empty target directory based on interactive mode
	if targetDir == "" {
		if interactive {
			// Interactive mode: use ExecuteWithInteractiveFlow which will prompt for target directory
			return initUI.ExecuteWithInteractiveFlowAndBaseRef(selectedConfig, "", force, update, !interactive, baseRef, templateVars)
		} else {
			// Non-interactive mode: target directory is required
			return errUtils.Build(errUtils.ErrTargetDirRequired).
				WithExplanation("Target directory is required in non-interactive mode").
				WithHint("Specify target directory: `atmos init <template> <target>`").
				WithHint("Or use interactive mode: `atmos init --interactive`").
				WithExitCode(2).
				Err()
		}
	}

	// Target directory provided, use normal Execute
	return initUI.ExecuteWithBaseRef(selectedConfig, targetDir, force, update, !interactive, baseRef, templateVars)
}
