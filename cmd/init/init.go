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
			key, value := parseSetFlag(flag)
			if key != "" {
				templateValues[key] = value
			}
		}

		return executeInit(
			cmd.Context(),
			template,
			target,
			interactive,
			force,
			templateValues,
		)
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

// parseSetFlag parses a --set flag in the format key=value.
func parseSetFlag(flag string) (string, string) {
	parts := strings.SplitN(flag, "=", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// executeInit initializes a new Atmos project from a template.
// This logic was moved from internal/exec/init.go to keep command logic in cmd/.
func executeInit(
	ctx context.Context,
	templateName string,
	targetDir string,
	interactive bool,
	force bool,
	templateVars map[string]interface{},
) error {
	// Convert to absolute path if provided
	if targetDir != "" {
		var err error
		targetDir, err = filepath.Abs(targetDir)
		if err != nil {
			return fmt.Errorf("%w: failed to resolve target directory: %w", errUtils.ErrInitialization, err)
		}
	}

	// Create the UI instance
	// Create I/O context for this command
	ioCtx, err := iolib.NewContext()
	if err != nil {
		return fmt.Errorf("failed to create I/O context: %w", err)
	}

	// Create terminal writer for I/O
	termWriter := iolib.NewTerminalWriter(ioCtx)
	term := terminal.New(terminal.WithIO(termWriter))

	initUI := ui.NewInitUI(ioCtx, term)

	// Get available template configurations
	configs, err := templates.GetAvailableConfigurations()
	if err != nil {
		return fmt.Errorf("%w: failed to get available configurations: %w", errUtils.ErrInitialization, err)
	}

	// Handle template selection
	var selectedConfig templates.Configuration
	if templateName == "" {
		// Check if interactive mode is disabled
		if !interactive {
			return fmt.Errorf("%w: template name must be provided in non-interactive mode", errUtils.ErrInitialization)
		}

		// Interactive template selection
		selectedName, err := initUI.PromptForTemplate("embeds", configs)
		if err != nil {
			return fmt.Errorf("%w: failed to prompt for template: %w", errUtils.ErrInitialization, err)
		}
		selectedConfig = configs[selectedName]
	} else {
		// Use specified template
		config, exists := configs[templateName]
		if !exists {
			return fmt.Errorf("%w: template '%s' not found", errUtils.ErrScaffoldNotFound, templateName)
		}
		selectedConfig = config
	}

	// Handle empty target directory based on interactive mode
	if targetDir == "" {
		if interactive {
			// Interactive mode: use ExecuteWithInteractiveFlow which will prompt for target directory
			return initUI.ExecuteWithInteractiveFlow(selectedConfig, "", force, false, !interactive, templateVars)
		} else {
			// Non-interactive mode: target directory is required
			return fmt.Errorf("%w: target directory is required in non-interactive mode", errUtils.ErrInitialization)
		}
	}

	// Target directory provided, use normal Execute
	return initUI.Execute(selectedConfig, targetDir, force, false, !interactive, templateVars)
}
