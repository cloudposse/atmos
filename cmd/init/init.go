package initcmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	gen "github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/generator/ui"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
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
- basic: Minimal cloud-agnostic project layout (atmos.yaml, stacks, and components)
- simple: Basic Atmos project structure
- atmos: Complete atmos.yaml configuration only
- aws/app: AWS application SDLC repository (dev/staging/prod, native CI, emulator-enabled)
- aws/landing-zone: AWS landing zone (dev/staging/prod environments with a conventional baseline, emulator-enabled)
- gcp/landing-zone: GCP landing zone (GCS, Secret Manager, service account, emulator-enabled)
- azure/landing-zone: Azure landing zone (resource group and network baseline, emulator-enabled)

Run "atmos scaffold list" to see all templates, including remote ones.

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

		// Get flag values with proper precedence: flag > env > config > default.
		force := v.GetBool("force")
		update := v.GetBool("update")
		baseRef := v.GetString("base-ref")
		if update {
			baseRef = defaultBaseRef(baseRef)
		}
		sourceOverride := v.GetString("source-override")
		ref := v.GetString("ref")
		gitEnabled := v.GetBool("git") && !v.GetBool("no-git")

		// Interactive prompting requires both an interactive-capable flag
		// value AND a real terminal on both stdin and stdout: in CI or
		// piped contexts prompts are skipped automatically and defaults +
		// --set values are used.
		interactive := v.GetBool("interactive") && term.IsTTYSupportForStdout() && term.IsTTYSupportForStdin()

		// Get template values from --set flags.
		// Use viper so env vars (ATMOS_INIT_SET) and config-backed values are honoured.
		setFlags := v.GetStringSlice("set")
		templateValues := make(map[string]interface{})
		for _, flag := range setFlags {
			key, value, err := parseSetFlag(flag)
			if err != nil {
				return fmt.Errorf("%w: invalid --set value %q: %w", errUtils.ErrInitialization, flag, err)
			}
			templateValues[key] = value
		}

		return executeInit(cmd.Context(), &initOptions{
			templateName:   template,
			targetDir:      target,
			interactive:    interactive,
			force:          force,
			update:         update,
			baseRef:        baseRef,
			templateVars:   templateValues,
			sourceOverride: sourceOverride,
			ref:            ref,
			git:            gitEnabled,
		})
	},
}

var initParser *flags.StandardParser

func init() {
	// Create StandardParser for init command flags with ATMOS_INIT_* env vars.
	initParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Overwrite existing files"),
		flags.WithBoolFlag("update", "", false, "Update an existing target directory via a 3-way merge instead of failing"),
		flags.WithStringFlag("base-ref", "", "", "Git ref in the target directory to use as the 3-way merge base (used with --update; defaults to HEAD)"),
		flags.WithBoolFlag("interactive", "i", true, "Interactive mode for template selection and configuration (disabled automatically without a terminal)"),
		flags.WithStringSliceFlag("set", "", []string{}, "Set template values (can be used multiple times: --set key=value)"),
		flags.WithStringFlag("source-override", "", "", "Resolve catalog templates from this local base directory instead of their remote source (mainly for testing)"),
		flags.WithStringFlag("ref", "", "", "Git ref for a template repository source (sugar for ?ref=)"),
		flags.WithBoolFlag("git", "", true, "Initialize a git repository and create the initial commit"),
		flags.WithBoolFlag("no-git", "", false, "Do not initialize a git repository"),
		flags.WithEnvVars("force", "ATMOS_INIT_FORCE"),
		flags.WithEnvVars("update", "ATMOS_INIT_UPDATE"),
		flags.WithEnvVars("base-ref", "ATMOS_INIT_BASE_REF"),
		flags.WithEnvVars("interactive", "ATMOS_INIT_INTERACTIVE"),
		flags.WithEnvVars("set", "ATMOS_INIT_SET"),
		flags.WithEnvVars("source-override", "ATMOS_INIT_SOURCE_OVERRIDE", "ATMOS_SCAFFOLD_SOURCE_OVERRIDE"),
		flags.WithEnvVars("ref", "ATMOS_INIT_REF"),
		flags.WithEnvVars("git", "ATMOS_INIT_GIT"),
		flags.WithEnvVars("no-git", "ATMOS_INIT_NO_GIT"),
	)

	// Register flags on the command.
	initParser.RegisterFlags(initCmd)

	// Bind to Viper for precedence handling.
	if err := initParser.BindToViper(viper.GetViper()); err != nil {
		log.Debug("Failed to bind init flags to Viper", "error", err)
	}

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
// Init ships as experimental while the project-template catalog and
// update workflow mature; behavior may change between releases.
func (i *InitCommandProvider) IsExperimental() bool {
	return true
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
	templateName   string
	targetDir      string
	interactive    bool
	force          bool
	update         bool
	baseRef        string
	templateVars   map[string]interface{}
	sourceOverride string
	ref            string
	git            bool
}

// executeInit initializes a new Atmos project from a template.
// This logic was moved from internal/exec/init.go to keep command logic in cmd/.
func executeInit(_ context.Context, opts *initOptions) error {
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

	// Merge distributable catalog templates (e.g. aws/landing-zone).
	// They are advertised as stubs and fetched from their source on selection.
	if stubs, stubErr := templates.CatalogStubs(opts.sourceOverride); stubErr == nil {
		for name := range stubs {
			if _, exists := configs[name]; !exists {
				configs[name] = stubs[name]
			}
		}
	} else {
		log.Debug("Failed to load scaffold catalog", "error", stubErr)
	}

	// Select the template.
	selectedConfig, err := selectTemplate(opts.templateName, opts.interactive, initUI, configs, opts.ref)
	if err != nil {
		return err
	}

	// Hydrate catalog/remote stubs into a full template before generating.
	// cleanup removes any temporary download directory after generation.
	cleanup, err := source.Hydrate(&selectedConfig, opts.sourceOverride)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInitialization, err)
	}
	defer cleanup()

	finalTargetDir, err := runInitExecution(initUI, &selectedConfig, opts)
	if err != nil {
		return err
	}
	return maybeInitGeneratedProjectGit(finalTargetDir, &selectedConfig, opts)
}

func maybeInitGeneratedProjectGit(targetDir string, selectedConfig *templates.Configuration, opts *initOptions) error {
	if !opts.git || targetDir == "" {
		return nil
	}
	_, err := gen.InitGitRepository(gen.InitGitOptions{
		TargetPath:      targetDir,
		TemplateName:    selectedConfig.Name,
		TemplateVersion: selectedConfig.Version,
	})
	return err
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
func selectTemplate(templateName string, interactive bool, initUI *ui.InitUI, configs map[string]templates.Configuration, ref string) (templates.Configuration, error) {
	// If template name is provided, use it directly.
	if templateName != "" {
		config, exists := configs[templateName]
		if !exists {
			if source.IsTemplateSource(templateName) {
				return templates.Configuration{
					Name:   templateName,
					Source: source.WithRef(templateName, ref),
				}, nil
			}
			return templates.Configuration{}, fmt.Errorf("%w: template '%s' not found", errUtils.ErrInitTemplateNotFound, templateName)
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
func runInitExecution(initUI *ui.InitUI, selectedConfig *templates.Configuration, opts *initOptions) (string, error) {
	// If target directory is empty, use interactive flow.
	if opts.targetDir == "" {
		if !opts.interactive {
			return "", fmt.Errorf("%w: target directory is required in non-interactive mode", errUtils.ErrInitialization)
		}
		targetDir, err := initUI.ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, "", opts.force, opts.update, !opts.interactive, opts.baseRef, opts.templateVars)
		if offer, retryBaseRef := shouldOfferUpdate(err, opts); offer {
			if confirmed, cErr := initUI.ConfirmUpdateInstead(targetDir); cErr == nil && confirmed {
				return initUI.ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, targetDir, opts.force, true, !opts.interactive, retryBaseRef, opts.templateVars)
			}
		}
		return targetDir, err
	}

	// Target directory provided, use normal Execute.
	err := initUI.ExecuteWithBaseRef(selectedConfig, opts.targetDir, opts.force, opts.update, !opts.interactive, opts.baseRef, opts.templateVars)
	if offer, retryBaseRef := shouldOfferUpdate(err, opts); offer {
		if confirmed, cErr := initUI.ConfirmUpdateInstead(opts.targetDir); cErr == nil && confirmed {
			return opts.targetDir, initUI.ExecuteWithBaseRef(selectedConfig, opts.targetDir, opts.force, true, !opts.interactive, retryBaseRef, opts.templateVars)
		}
	}
	return opts.targetDir, err
}

// shouldOfferUpdate decides whether to offer a 3-way-merge update instead of
// failing outright on a non-empty target directory: only when the failure is
// exactly that, the caller isn't already using --force/--update, and a real
// terminal is available to prompt on. Returns the base ref to retry with
// (the caller's --base-ref, defaulting to HEAD) alongside the decision.
func shouldOfferUpdate(err error, opts *initOptions) (bool, string) {
	if err == nil || opts.force || opts.update || !opts.interactive {
		return false, ""
	}
	if !errors.Is(err, errUtils.ErrTargetDirectoryNotEmpty) {
		return false, ""
	}
	return true, defaultBaseRef(opts.baseRef)
}

// defaultBaseRef fills in HEAD as the 3-way-merge base ref when the caller
// didn't supply one. Without this, --update silently sets up no git storage
// at all (ExecuteWithDelimiters only calls SetupGitStorage when baseRef is
// non-empty) and every file fails with an opaque "three-way merge failed" --
// HEAD is the obvious default since `atmos init --git` always creates an
// initial commit.
func defaultBaseRef(baseRef string) string {
	if baseRef == "" {
		return "HEAD"
	}
	return baseRef
}
