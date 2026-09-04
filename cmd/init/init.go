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
	"github.com/cloudposse/atmos/pkg/generator/merge"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/storage"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/generator/ui"
	"github.com/cloudposse/atmos/pkg/hooks"
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
		// Only pre-resolve here when target is already the real, final
		// target directory (i.e. it was given positionally). When target is
		// "" the interactive flow still has to prompt for one -- see
		// resolveInteractiveInitBaseRef, which resolves the base ref itself
		// once the actual directory is known. Resolving against "" here
		// would read .atmos/init/metadata.yaml from the wrong (empty/cwd)
		// path and permanently overwrite baseRef with "HEAD", discarding any
		// pin at the directory the user goes on to pick.
		if update && target != "" {
			resolvedBaseRef, err := defaultBaseRef(baseRef, target)
			if err != nil {
				return err
			}
			baseRef = resolvedBaseRef
		}
		sourceOverride := v.GetString("source-override")
		ref := v.GetString("ref")
		gitEnabled := v.GetBool("git") && !v.GetBool("no-git")
		mergeStrategy := v.GetString("merge-strategy")
		mergeDriver := v.GetString("merge-driver")
		skipHooks := hooks.NewSkipPredicate(hooks.ResolveSkipHooks(cmd))

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
			mergeStrategy:  mergeStrategy,
			mergeDriver:    mergeDriver,
			skipHooks:      skipHooks,
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
		flags.WithStringFlag("merge-driver", "", "auto", "Merge driver for --update: auto (YAML-aware for .yaml/.yml, text otherwise, default), text (force line-oriented text merge for every file)"),
		flags.WithValidValues("merge-driver", "auto", "text"),
		flags.WithStringFlag("merge-strategy", "", "", "Conflict resolution strategy for --update: manual (surface conflicts, default; theirs if --force is set), ours (keep your version), theirs (use the template's version)"),
		flags.WithValidValues("merge-strategy", "manual", "ours", "theirs"),
		// Skip scaffold hooks at runtime, mirroring `terraform`'s --skip-hooks
		// (see cmd/terraform/flags.go): --skip-hooks (no value) skips all
		// hooks for this invocation; --skip-hooks=name1,name2 skips only the
		// named hooks.
		flags.WithStringFlag("skip-hooks", "", "", "Skip scaffold hooks for this invocation. Use --skip-hooks (no value) to skip all, or --skip-hooks=name1,name2 to skip specific hooks by name"),
		flags.WithNoOptDefVal("skip-hooks", "*"),
		flags.WithEnvVars("force", "ATMOS_INIT_FORCE"),
		flags.WithEnvVars("update", "ATMOS_INIT_UPDATE"),
		flags.WithEnvVars("base-ref", "ATMOS_INIT_BASE_REF"),
		flags.WithEnvVars("interactive", "ATMOS_INIT_INTERACTIVE"),
		flags.WithEnvVars("set", "ATMOS_INIT_SET"),
		flags.WithEnvVars("source-override", "ATMOS_INIT_SOURCE_OVERRIDE", "ATMOS_SCAFFOLD_SOURCE_OVERRIDE"),
		flags.WithEnvVars("ref", "ATMOS_INIT_REF"),
		flags.WithEnvVars("git", "ATMOS_INIT_GIT"),
		flags.WithEnvVars("no-git", "ATMOS_INIT_NO_GIT"),
		flags.WithEnvVars("merge-driver", "ATMOS_INIT_MERGE_DRIVER"),
		flags.WithEnvVars("merge-strategy", "ATMOS_INIT_MERGE_STRATEGY"),
		flags.WithEnvVars("skip-hooks", "ATMOS_INIT_SKIP_HOOKS"),
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
	mergeStrategy  string
	mergeDriver    string
	skipHooks      func(string) bool
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

	conflictStrategy, err := merge.ResolveConflictStrategy(opts.mergeStrategy, opts.force, opts.update)
	if err != nil {
		return err
	}
	initUI.SetConflictStrategy(conflictStrategy)

	mergeDriver, err := merge.ParseDriver(opts.mergeDriver)
	if err != nil {
		return err
	}
	initUI.SetMergeDriver(mergeDriver)
	initUI.SetSkipHooks(opts.skipHooks)

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
	_, headSHA, err := gen.InitGitRepository(gen.InitGitOptions{
		TargetPath:      targetDir,
		TemplateName:    selectedConfig.Name,
		TemplateVersion: selectedConfig.Version,
	})
	if err != nil {
		return err
	}
	// No-op when headSHA is empty (targetDir was already a git repo; see
	// gen.PinInitialBaseRefForInit).
	return gen.PinInitialBaseRefForInit(
		targetDir, headSHA,
		gen.WithTemplateName(selectedConfig.Name),
		gen.WithTemplateVersion(selectedConfig.Version),
		gen.WithSource(selectedConfig.Source),
	)
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
	// If target directory is empty, use interactive flow; otherwise use normal Execute.
	if opts.targetDir == "" {
		return runInitInteractiveFlow(initUI, selectedConfig, opts)
	}
	return runInitTargetedFlow(initUI, selectedConfig, opts)
}

// runInitInteractiveFlow handles init when no target directory was provided,
// prompting the user for one (and optionally offering a 3-way-merge update
// instead of failing when it already exists and is non-empty).
func runInitInteractiveFlow(initUI *ui.InitUI, selectedConfig *templates.Configuration, opts *initOptions) (string, error) {
	if !opts.interactive {
		return "", fmt.Errorf("%w: target directory is required in non-interactive mode", errUtils.ErrInitialization)
	}

	resolved, err := resolveInteractiveInitBaseRef(initUI, selectedConfig, opts)
	if err != nil {
		return resolved.targetDir, err
	}

	finalTargetDir, err := initUI.ExecuteWithInteractiveFlowAndBaseRefResult(
		selectedConfig, resolved.targetDir, opts.force, opts.update, resolved.useDefaults, resolved.baseRef, resolved.templateValues,
	)
	offer, retryBaseRef, offerErr := shouldOfferUpdate(err, opts, finalTargetDir)
	if offerErr != nil {
		return finalTargetDir, offerErr
	}
	if offer {
		if confirmed, cErr := initUI.ConfirmUpdateInstead(finalTargetDir); cErr == nil && confirmed {
			return initUI.ExecuteWithInteractiveFlowAndBaseRefResult(
				selectedConfig, finalTargetDir, opts.force, true, resolved.useDefaults, retryBaseRef, resolved.templateValues,
			)
		}
	}
	return finalTargetDir, err
}

// interactiveInitBaseRef bundles resolveInteractiveInitBaseRef's results
// (grouped into a struct, rather than five separate return values, to stay
// under revive's function-result-limit).
type interactiveInitBaseRef struct {
	targetDir      string
	baseRef        string
	templateValues map[string]interface{}
	useDefaults    bool
}

// resolveInteractiveInitBaseRef resolves the --update merge base ref for the
// no-positional-target interactive flow, mirroring cmd/scaffold's
// resolveInteractiveBaseRef. --base-ref's default (the pinned ref from
// .atmos/init/metadata.yaml, see defaultBaseRef) can only be looked up once
// the real target directory is known, but in this flow that directory
// doesn't exist until the interactive prompt below picks one -- so for
// --update, resolve the target directory first (initUI.ResolveTargetPath
// runs the same prompt/setup-form logic
// ExecuteWithInteractiveFlowAndBaseRefResult would, and is a no-op once
// targetDir is non-empty), then resolve the base ref against it, and
// finally hand both back to the caller's
// ExecuteWithInteractiveFlowAndBaseRefResult call -- which skips prompting
// again since targetDir is already set.
//
// Without --update the base ref is unused (ExecuteWithDelimiters only sets
// up git storage when update is true), so this is a no-op passthrough that
// still lets the interactive flow prompt for the target itself.
func resolveInteractiveInitBaseRef(
	initUI *ui.InitUI,
	selectedConfig *templates.Configuration,
	opts *initOptions,
) (interactiveInitBaseRef, error) {
	if !opts.update {
		return interactiveInitBaseRef{baseRef: opts.baseRef, templateValues: opts.templateVars, useDefaults: !opts.interactive}, nil
	}

	targetDir, templateValues, useDefaults, err := initUI.ResolveTargetPath(selectedConfig, "", opts.update, !opts.interactive, opts.templateVars)
	if err != nil {
		return interactiveInitBaseRef{targetDir: targetDir}, err
	}

	baseRef, err := defaultBaseRef(opts.baseRef, targetDir)
	if err != nil {
		return interactiveInitBaseRef{targetDir: targetDir}, err
	}
	return interactiveInitBaseRef{targetDir: targetDir, baseRef: baseRef, templateValues: templateValues, useDefaults: useDefaults}, nil
}

// runInitTargetedFlow handles init when a target directory was provided
// (offering the same 3-way-merge update fallback as the interactive flow).
func runInitTargetedFlow(initUI *ui.InitUI, selectedConfig *templates.Configuration, opts *initOptions) (string, error) {
	err := initUI.ExecuteWithBaseRef(selectedConfig, opts.targetDir, opts.force, opts.update, !opts.interactive, opts.baseRef, opts.templateVars)
	offer, retryBaseRef, offerErr := shouldOfferUpdate(err, opts, opts.targetDir)
	if offerErr != nil {
		return opts.targetDir, offerErr
	}
	if offer {
		if confirmed, cErr := initUI.ConfirmUpdateInstead(opts.targetDir); cErr == nil && confirmed {
			return opts.targetDir, initUI.ExecuteWithBaseRef(selectedConfig, opts.targetDir, opts.force, true, !opts.interactive, retryBaseRef, opts.templateVars)
		}
	}
	return opts.targetDir, err
}

// shouldOfferUpdate decides whether to offer a 3-way-merge update instead of
// failing outright on a non-empty target directory: only when the failure is
// exactly that, the caller isn't already using --force/--update, and a real
// terminal is available to prompt on. TargetDir must be the actual, final
// target directory generation just ran against (not opts.targetDir, which is
// the raw positional arg and can be "" when the interactive flow picked the
// real directory itself -- see resolveInteractiveInitBaseRef). Returns the
// base ref to retry with (the caller's --base-ref, defaulting to HEAD or a
// pinned metadata ref) alongside the decision.
func shouldOfferUpdate(err error, opts *initOptions, targetDir string) (offer bool, baseRef string, resolveErr error) {
	if err == nil || opts.force || opts.update || !opts.interactive {
		return false, "", nil
	}
	if !errors.Is(err, errUtils.ErrTargetDirectoryNotEmpty) {
		return false, "", nil
	}
	resolvedBaseRef, resolveErr := defaultBaseRef(opts.baseRef, targetDir)
	if resolveErr != nil {
		return false, "", resolveErr
	}
	return true, resolvedBaseRef, nil
}

// defaultBaseRef resolves init's --update base ref against this target's own
// pinned metadata (.atmos/init/metadata.yaml, written by
// gen.PinInitialBaseRefForInit). See gen.ResolveDefaultBaseRef's doc for the
// full rationale -- that function is shared with cmd/scaffold's equivalent
// so the two commands' base-ref resolution can't drift apart again (as it
// did before: this command shipped with the same silent-overwrite bug
// cmd/scaffold fixed, because the fix lived only in cmd/scaffold and was
// never ported here).
func defaultBaseRef(baseRef, targetDir string) (string, error) {
	return gen.ResolveDefaultBaseRef(baseRef, targetDir, storage.InitMetadataPath(targetDir))
}
