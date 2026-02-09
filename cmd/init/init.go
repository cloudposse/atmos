package init

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/generator/ui"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
)

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

		force, _ := cmd.Flags().GetBool("force")
		interactive, _ := cmd.Flags().GetBool("interactive")

		// Get template values from --set flags
		setFlags, _ := cmd.Flags().GetStringSlice("set")
		templateValues := make(map[string]interface{})
		for _, flag := range setFlags {
			key, value, err := parseSetFlag(flag)
			if err != nil {
				return fmt.Errorf("%w: invalid --set value %q: %w", errUtils.ErrInitialization, flag, err)
			}
			templateValues[key] = value
		}

		return executeInit(cmd.Context(), initOptions{
			templateName: template,
			targetDir:    target,
			interactive:  interactive,
			force:        force,
			templateVars: templateValues,
		})
	},
}

func init() {
	initCmd.Flags().BoolP("force", "f", false, "Overwrite existing files")
	initCmd.Flags().BoolP("interactive", "i", true, "Interactive mode for template selection and configuration")
	initCmd.Flags().StringSlice("set", []string{}, "Set template values (can be used multiple times: --set key=value)")

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
// Init command flags are defined in cobra directly.
func (i *InitCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
// Init command has no positional args builder.
func (i *InitCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
// Init command has no compatibility flags.
func (i *InitCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases for the init command.
func (i *InitCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (i *InitCommandProvider) IsExperimental() bool {
	return false
}

// parseSetFlag parses a --set flag in the format key=value.
// Returns an error if the flag is malformed (missing = or empty key).
func parseSetFlag(flag string) (string, string, error) {
	parts := strings.SplitN(flag, "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return "", "", errUtils.Build(errUtils.ErrInvalidFormat).
			WithExplanation("Invalid --set flag format").
			WithHint("Use key=value format (e.g., --set name=myproject)").
			Err()
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

// initOptions holds configuration for the init operation.
type initOptions struct {
	templateName string
	targetDir    string
	interactive  bool
	force        bool
	templateVars map[string]interface{}
}

// executeInit initializes a new Atmos project from a template.
// This logic was moved from internal/exec/init.go to keep command logic in cmd/.
func executeInit(_ context.Context, opts initOptions) error {
	// Convert to absolute path if provided.
	opts.targetDir = resolveTargetDir(opts.targetDir)

	// Create the UI instance.
	initUI, err := createInitUI()
	if err != nil {
		return err
	}

	// Get available template configurations.
	configs, err := templates.GetAvailableConfigurations()
	if err != nil {
		return fmt.Errorf("%w: failed to get available configurations: %w", errUtils.ErrInitialization, err)
	}

	// Select the template.
	selectedConfig, err := selectTemplate(opts.templateName, opts.interactive, initUI, configs)
	if err != nil {
		return err
	}

	// Execute with the selected template.
	return runInitExecution(initUI, &selectedConfig, opts)
}

// resolveTargetDir converts a target directory to an absolute path if provided.
func resolveTargetDir(targetDir string) string {
	if targetDir == "" {
		return ""
	}
	absPath, err := filepath.Abs(targetDir)
	if err != nil {
		return targetDir // Return original if resolution fails.
	}
	return absPath
}

// createInitUI creates the UI instance for init operations.
func createInitUI() (*ui.InitUI, error) {
	ioCtx, err := iolib.NewContext()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create I/O context: %w", errUtils.ErrInitialization, err)
	}
	termWriter := iolib.NewTerminalWriter(ioCtx)
	term := terminal.New(terminal.WithIO(termWriter))
	return ui.NewInitUI(ioCtx, term), nil
}

// selectTemplate handles template selection, either from argument or interactively.
func selectTemplate(templateName string, interactive bool, initUI *ui.InitUI, configs map[string]templates.Configuration) (templates.Configuration, error) {
	// If template name is provided, use it directly.
	if templateName != "" {
		config, exists := configs[templateName]
		if !exists {
			return templates.Configuration{}, fmt.Errorf("%w: template '%s' not found", errUtils.ErrScaffoldNotFound, templateName)
		}
		return config, nil
	}

	// Template name not provided - need interactive selection.
	if !interactive {
		return templates.Configuration{}, fmt.Errorf("%w: template name must be provided in non-interactive mode", errUtils.ErrInitialization)
	}

	// Interactive template selection.
	selectedName, err := initUI.PromptForTemplate("embeds", configs)
	if err != nil {
		return templates.Configuration{}, fmt.Errorf("%w: failed to prompt for template: %w", errUtils.ErrInitialization, err)
	}
	return configs[selectedName], nil
}

// runInitExecution executes the init with the selected template and target directory.
func runInitExecution(initUI *ui.InitUI, selectedConfig *templates.Configuration, opts initOptions) error {
	// If target directory is empty, use interactive flow.
	if opts.targetDir == "" {
		if !opts.interactive {
			return fmt.Errorf("%w: target directory is required in non-interactive mode", errUtils.ErrInitialization)
		}
		return initUI.ExecuteWithInteractiveFlow(selectedConfig, "", opts.force, false, !opts.interactive, opts.templateVars)
	}

	// Target directory provided, use normal Execute.
	return initUI.Execute(selectedConfig, opts.targetDir, opts.force, false, !opts.interactive, opts.templateVars)
}
