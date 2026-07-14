//nolint:revive // file-length-limit: scaffold command has many interconnected functions
package scaffold

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	gen "github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/merge"
	"github.com/cloudposse/atmos/pkg/generator/setup"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/project/config"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendor"
)

var scaffoldGenerateParser *flags.StandardParser

// validateSetFlag validates a --set flag entry in the format "key=value".
// Returns an error if the entry is malformed.
func validateSetFlag(entry string) error {
	if !strings.Contains(entry, "=") {
		return errUtils.Build(errUtils.ErrInvalidFlag).
			WithExplanationf("Malformed --set flag (missing '='): %s", entry).
			WithHint("Use the format: --set key=value").
			Err()
	}

	parts := strings.SplitN(entry, "=", 2)
	if strings.TrimSpace(parts[0]) == "" {
		return errUtils.Build(errUtils.ErrInvalidFlag).
			WithExplanationf("Malformed --set flag (empty key): %s", entry).
			WithHint("Use the format: --set key=value").
			Err()
	}

	return nil
}

// parseSetFlag parses a --set flag entry in the format "key=value".
// Returns the key and value, or an error if the entry is malformed.
// Both key and value have leading/trailing whitespace trimmed.
// This behavior matches flags.parseKeyValuePair for consistency.
func parseSetFlag(entry string) (key string, value string, err error) {
	if err := validateSetFlag(entry); err != nil {
		return "", "", err
	}

	parts := strings.SplitN(entry, "=", 2)
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
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
		update := v.GetBool("update")
		baseRef := v.GetString("base-ref")
		if update {
			baseRef = defaultBaseRef(baseRef)
		}
		dryRun := v.GetBool("dry-run")
		sourceOverride := v.GetString("scaffold-source-override")
		ref := v.GetString("ref")
		gitEnabled := v.GetBool("git") && !v.GetBool("no-git")
		mergeStrategy := v.GetString("merge-strategy")

		// Interactive prompting requires both an interactive-capable flag
		// value AND a real terminal on both stdin and stdout: in CI or
		// piped contexts the form is skipped automatically and defaults +
		// --set values are used.
		interactive := v.GetBool("interactive") && term.IsTTYSupportForStdout() && term.IsTTYSupportForStdin()
		useDefaults := dryRun || v.GetBool("defaults") || !interactive

		// Parse string map from --set flags
		templateValuesMap := flags.ParseStringMap(v, "set")

		// Validate --set flags were properly formatted
		// Get raw flag values to check for malformed entries
		if cmd.Flags().Changed("set") {
			// --set is registered as StringArray (pkg/flags), not StringSlice, so
			// values with commas aren't split; GetStringSlice would fail its type
			// assertion here and silently no-op this whole validation loop.
			rawSetFlags, err := cmd.Flags().GetStringArray("set")
			if err != nil {
				return err
			}
			for _, entry := range rawSetFlags {
				if err := validateSetFlag(entry); err != nil {
					return err
				}
			}
		}

		// Convert map[string]string to map[string]interface{} for template engine
		templateValues := make(map[string]interface{})
		for k, val := range templateValuesMap {
			templateValues[k] = val
		}

		return executeScaffoldGenerate(&scaffoldGenerateOptions{
			templateName:   template,
			targetDir:      target,
			force:          force,
			update:         update,
			baseRef:        baseRef,
			dryRun:         dryRun,
			interactive:    interactive,
			useDefaults:    useDefaults,
			templateValues: templateValues,
			sourceOverride: sourceOverride,
			ref:            ref,
			git:            gitEnabled,
			mergeStrategy:  mergeStrategy,
		})
	},
}

// scaffoldGenerateOptions holds the resolved inputs for scaffold generation.
type scaffoldGenerateOptions struct {
	templateName   string
	targetDir      string
	force          bool
	update         bool
	baseRef        string
	dryRun         bool
	interactive    bool
	useDefaults    bool
	templateValues map[string]interface{}
	sourceOverride string
	ref            string
	git            bool
	mergeStrategy  string
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
	// Create StandardParser for generate subcommand flags.
	scaffoldGenerateParser = flags.NewStandardParser(
		flags.WithBoolFlag("force", "f", false, "Overwrite existing files"),
		flags.WithBoolFlag("update", "", false, "Update an existing target directory via a 3-way merge instead of failing"),
		flags.WithStringFlag("base-ref", "", "", "Git ref in the target directory to use as the 3-way merge base (used with --update; defaults to HEAD)"),
		flags.WithBoolFlag("dry-run", "", false, "Preview changes without writing files"),
		flags.WithBoolFlag("interactive", "i", true, "Prompt for field values (disabled automatically without a terminal)"),
		flags.WithBoolFlag("defaults", "", false, "Use field defaults and --set values without prompting"),
		flags.WithStringMapFlag("set", "", map[string]string{}, "Set template values (can be used multiple times: --set key=value)"),
		flags.WithStringFlag("scaffold-source-override", "", "", "Resolve catalog templates from this local base directory instead of their remote source (mainly for testing)"),
		flags.WithStringFlag("ref", "", "", "Git ref for a template repository source (sugar for ?ref=)"),
		flags.WithBoolFlag("git", "", false, "Initialize a git repository and create the initial commit"),
		flags.WithBoolFlag("no-git", "", false, "Do not initialize a git repository"),
		flags.WithStringFlag("merge-strategy", "", "manual", "Conflict resolution strategy for --update: manual (surface conflicts, default), ours (keep your version), theirs (use the template's version)"),
		flags.WithValidValues("merge-strategy", "manual", "ours", "theirs"),
		flags.WithEnvVars("force", "ATMOS_SCAFFOLD_FORCE"),
		flags.WithEnvVars("update", "ATMOS_SCAFFOLD_UPDATE"),
		flags.WithEnvVars("base-ref", "ATMOS_SCAFFOLD_BASE_REF"),
		flags.WithEnvVars("dry-run", "ATMOS_SCAFFOLD_DRY_RUN"),
		flags.WithEnvVars("interactive", "ATMOS_SCAFFOLD_INTERACTIVE"),
		flags.WithEnvVars("defaults", "ATMOS_SCAFFOLD_DEFAULTS"),
		flags.WithEnvVars("set", "ATMOS_SCAFFOLD_SET"),
		flags.WithEnvVars("scaffold-source-override", "ATMOS_SCAFFOLD_SOURCE_OVERRIDE"),
		flags.WithEnvVars("ref", "ATMOS_SCAFFOLD_REF"),
		flags.WithEnvVars("git", "ATMOS_SCAFFOLD_GIT"),
		flags.WithEnvVars("no-git", "ATMOS_SCAFFOLD_NO_GIT"),
		flags.WithEnvVars("merge-strategy", "ATMOS_SCAFFOLD_MERGE_STRATEGY"),
	)

	// Register flags to generate subcommand.
	scaffoldGenerateParser.RegisterFlags(scaffoldGenerateCmd)

	// Bind to Viper for precedence handling.
	if err := scaffoldGenerateParser.BindToViper(viper.GetViper()); err != nil {
		log.Debug("Failed to bind scaffold flags to Viper", "error", err)
	}

	// Add subcommands to scaffold parent.
	scaffoldCmd.AddCommand(scaffoldGenerateCmd)
	scaffoldCmd.AddCommand(scaffoldListCmd)
	scaffoldCmd.AddCommand(scaffoldValidateCmd)

	// Register this command with the registry.
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

// GetAliases returns command aliases (none for scaffold).
func (s *ScaffoldCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
// Scaffold ships as experimental while the template schema and update
// workflow mature; behavior may change between releases.
func (s *ScaffoldCommandProvider) IsExperimental() bool {
	return true
}

// GetFlagsBuilder returns nil since the parent scaffold command has no flags.
// Flags (--force, --dry-run, --set) belong to the generate subcommand.
func (s *ScaffoldCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
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
func executeScaffoldGenerate(opts *scaffoldGenerateOptions) error {
	// Convert to absolute path
	absTargetDir, err := resolveTargetDirectory(opts.targetDir)
	if err != nil {
		return err
	}

	// Load all available templates
	configs, _, scaffoldUI, err := loadScaffoldTemplates(opts.sourceOverride)
	if err != nil {
		return err
	}

	conflictStrategy, err := merge.ParseConflictStrategy(opts.mergeStrategy)
	if err != nil {
		return err
	}
	scaffoldUI.SetConflictStrategy(conflictStrategy)

	// Select template (interactive or by name)
	selectedConfig, err := selectGenerateTemplate(opts, configs, scaffoldUI)
	if err != nil {
		return err
	}

	// Catalog/remote templates are advertised as stubs without files; fetch the
	// selected one into a full template before generating. cleanup removes any
	// temporary download directory once generation completes.
	cleanup, err := source.Hydrate(&selectedConfig, opts.sourceOverride)
	if err != nil {
		return err
	}
	defer cleanup()

	// If dry-run mode, render preview and return without writing files.
	if opts.dryRun {
		if absTargetDir == "" {
			return errUtils.Build(errUtils.ErrTargetDirRequired).
				WithExplanation("Target directory is required when using `--dry-run` flag").
				WithHint("Specify target directory: `atmos scaffold generate <template> <target>`").
				WithHint("Or remove `--dry-run` flag to use interactive mode").
				WithContext("flag", "dry-run").
				WithExitCode(2).
				Err()
		}
		// With --update, a path-only preview can't show real merge/conflict
		// status. Drive the real generation path with dry-run enabled on the
		// processor instead: rendering, git base load, and the 3-way merge all
		// still run (so genuine conflicts are reported), but no file is written.
		if opts.update {
			scaffoldUI.SetDryRun(true)
			return executeTemplateGeneration(&selectedConfig, absTargetDir, opts, scaffoldUI)
		}
		return renderDryRunPreview(&selectedConfig, absTargetDir, opts.templateValues)
	}

	// Execute template generation.
	return executeTemplateGeneration(&selectedConfig, absTargetDir, opts, scaffoldUI)
}

// selectGenerateTemplate selects the template for generation, refusing to
// open an interactive picker when interactivity is unavailable.
func selectGenerateTemplate(
	opts *scaffoldGenerateOptions,
	configs map[string]templates.Configuration,
	scaffoldUI ScaffoldUI,
) (templates.Configuration, error) {
	if opts.templateName == "" && !opts.interactive {
		return templates.Configuration{}, errUtils.Build(errUtils.ErrTemplateNameRequired).
			WithExplanation("A template name is required in non-interactive mode").
			WithHint("Specify the template: `atmos scaffold generate <template> <target>`").
			WithHint("Run `atmos scaffold list` to see available templates").
			WithExitCode(2).
			Err()
	}
	if opts.templateName != "" {
		if cfg, ok := configs[opts.templateName]; ok {
			return cfg, nil
		}
		if source.IsTemplateSource(opts.templateName) {
			return templates.Configuration{
				Name:   opts.templateName,
				Source: source.WithRef(opts.templateName, opts.ref),
			}, nil
		}
	}
	return selectTemplate(opts.templateName, configs, scaffoldUI)
}

// resolveTargetDirectory converts target directory to absolute path.
func resolveTargetDirectory(targetDir string) (string, error) {
	if targetDir == "" {
		return "", nil
	}

	absPath, err := filepath.Abs(targetDir)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrResolveTargetDirectory).
			WithCause(err).
			WithExplanationf("Cannot resolve target directory path: `%s`", targetDir).
			WithHint("Ensure the path is valid").
			WithHint("Check that the parent directory exists and is accessible").
			WithContext("target_dir", targetDir).
			WithExitCode(2).
			Err()
	}
	return absPath, nil
}

// loadScaffoldTemplates loads all available scaffold templates from embedded and atmos.yaml.
// Returns configs, origins (map[name]source where source is "embedded" or "atmos.yaml"), UI, and error.
func loadScaffoldTemplates(sourceOverride string) (map[string]templates.Configuration, map[string]string, ScaffoldUI, error) {
	// Create generator context
	genCtx, err := setup.NewGeneratorContext()
	if err != nil {
		return nil, nil, nil, errUtils.Build(errUtils.ErrCreateGeneratorContext).
			WithExplanation("Failed to initialize generator context").
			WithHint("Check terminal capabilities and I/O permissions").
			WithHint("Try running with `ATMOS_LOGS_LEVEL=Debug` for more details").
			WithExitCode(1).
			Err()
	}

	// Load embedded templates
	configs, err := templates.GetAvailableConfigurations()
	if err != nil {
		return nil, nil, nil, errUtils.Build(errUtils.ErrLoadScaffoldTemplates).
			WithExplanation("Failed to load available scaffold templates").
			WithHint("Run `atmos scaffold list` to see available templates").
			WithHint("Check that embedded templates are included in the binary").
			WithContext("templates_loaded", len(configs)).
			WithExitCode(1).
			Err()
	}

	// Track origins - embedded templates first
	origins := make(map[string]string)
	for name := range configs {
		origins[name] = "embedded"
	}

	// Merge distributable catalog templates (advertised as stubs, fetched on
	// selection). Embedded templates take precedence over catalog entries.
	if stubs, cerr := templates.CatalogStubs(sourceOverride); cerr == nil {
		for name := range stubs {
			if _, exists := configs[name]; !exists {
				configs[name] = stubs[name]
				origins[name] = "catalog"
			}
		}
	} else {
		log.Debug("Failed to load scaffold catalog", "error", cerr)
	}

	// Merge with configured templates from atmos.yaml (these override the above).
	if err := mergeConfiguredTemplates(configs, origins); err != nil {
		return nil, nil, nil, err
	}

	return configs, origins, genCtx.UI, nil
}

// mergeConfiguredTemplates merges scaffold templates from atmos.yaml into the configs map.
// It also updates the origins map to track which templates came from atmos.yaml.
func mergeConfiguredTemplates(configs map[string]templates.Configuration, origins map[string]string) error {
	defer perf.Track(nil, "scaffold.mergeConfiguredTemplates")()

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

	templatesData, ok := scaffoldSection["templates"]
	if !ok {
		return nil // No templates configured, that's fine
	}

	templatesMap, ok := templatesData.(map[string]interface{})
	if !ok {
		return errUtils.Build(errUtils.ErrInvalidScaffoldConfig).
			WithExplanation("The `scaffold.templates` section is not a valid configuration").
			WithHint("The `scaffold.templates` section must be a map of template names to configurations").
			WithExample("```yaml\nscaffold:\n  templates:\n    my-template:\n      description: My template\n      source: ./path\n```").
			WithContext("config_file", "atmos.yaml").
			WithExitCode(2).
			Err()
	}

	for templateName, templateData := range templatesMap {
		cfg, err := convertScaffoldTemplateToConfiguration(templateName, templateData)
		if err != nil {
			// Log error but continue with other templates.
			atmosui.Warning(fmt.Sprintf("Failed to load scaffold template '%s': %v", templateName, err))
			continue
		}
		// Configured templates override embedded templates
		configs[templateName] = cfg
		// Track origin as atmos.yaml (overriding any embedded origin)
		origins[templateName] = "atmos.yaml"
	}

	return nil
}

// selectTemplate selects a template either interactively or by name.
func selectTemplate(
	templateName string,
	configs map[string]templates.Configuration,
	scaffoldUI ScaffoldUI,
) (templates.Configuration, error) {
	if templateName == "" {
		return selectTemplateInteractive(configs, scaffoldUI)
	}
	return selectTemplateByName(templateName, configs)
}

// selectTemplateInteractive prompts the user to select a template.
func selectTemplateInteractive(
	configs map[string]templates.Configuration,
	scaffoldUI ScaffoldUI,
) (templates.Configuration, error) {
	selectedName, err := scaffoldUI.PromptForTemplate("scaffold", configs)
	if err != nil {
		return templates.Configuration{}, errUtils.Build(errUtils.ErrPromptFailed).
			WithExplanation("Interactive template selection failed").
			WithHint("Interactive prompts require a TTY (terminal)").
			WithHint("Use non-interactive mode: `atmos scaffold generate <template> <target>`").
			WithHint("Or set `ATMOS_FORCE_TTY=true` if running in a compatible environment").
			WithContext("mode", "interactive").
			WithExitCode(1).
			Err()
	}
	return configs[selectedName], nil
}

// selectTemplateByName selects a template by name from available configs.
func selectTemplateByName(
	templateName string,
	configs map[string]templates.Configuration,
) (templates.Configuration, error) {
	cfg, exists := configs[templateName]
	if !exists {
		availableTemplates := make([]string, 0, len(configs))
		for name := range configs {
			availableTemplates = append(availableTemplates, name)
		}
		return templates.Configuration{}, errUtils.Build(errUtils.ErrScaffoldNotFound).
			WithExplanationf("Scaffold template `%s` not found", templateName).
			WithHint("Run `atmos scaffold list` to see available templates").
			WithHint("Check the `scaffold.templates` section in your `atmos.yaml`").
			WithHint("Verify the template name is spelled correctly").
			WithContext("template", templateName).
			WithContext("available_templates", strings.Join(availableTemplates, ", ")).
			WithExitCode(2).
			Err()
	}
	return cfg, nil
}

// executeTemplateGeneration executes the template generation with the selected configuration.
func executeTemplateGeneration(
	selectedConfig *templates.Configuration,
	targetDir string,
	opts *scaffoldGenerateOptions,
	scaffoldUI ScaffoldUI,
) error {
	if targetDir == "" {
		finalTargetDir, err := executeTemplateWithoutTargetDir(selectedConfig, opts, scaffoldUI)
		if err != nil {
			return err
		}
		return maybeInitGeneratedGitRepository(finalTargetDir, selectedConfig, opts)
	}

	// Target directory provided, use normal Execute.
	err := scaffoldUI.ExecuteWithBaseRef(selectedConfig, targetDir, opts.force, opts.update, opts.useDefaults, opts.baseRef, opts.templateValues)
	if offer, retryBaseRef := shouldOfferScaffoldUpdate(err, opts); offer {
		if confirmed, cErr := scaffoldUI.ConfirmUpdateInstead(targetDir); cErr == nil && confirmed {
			err = scaffoldUI.ExecuteWithBaseRef(selectedConfig, targetDir, opts.force, true, opts.useDefaults, retryBaseRef, opts.templateValues)
		}
	}
	if err != nil {
		return err
	}
	return maybeInitGeneratedGitRepository(targetDir, selectedConfig, opts)
}

// shouldOfferScaffoldUpdate mirrors cmd/init's shouldOfferUpdate: offer a
// 3-way-merge update instead of failing outright on a non-empty target
// directory, only when not already using --force/--update, not in dry-run,
// and a real terminal is available to prompt on. Returns the base ref to
// retry with (the caller's --base-ref, defaulting to HEAD) alongside the
// decision.
func shouldOfferScaffoldUpdate(err error, opts *scaffoldGenerateOptions) (bool, string) {
	if err == nil || opts.force || opts.update || !opts.interactive || opts.dryRun {
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
// HEAD is the obvious default since `atmos init/scaffold --git` always
// creates an initial commit.
func defaultBaseRef(baseRef string) string {
	if baseRef == "" {
		return "HEAD"
	}
	return baseRef
}

func maybeInitGeneratedGitRepository(targetDir string, selectedConfig *templates.Configuration, opts *scaffoldGenerateOptions) error {
	if opts.git && !opts.dryRun {
		_, err := gen.InitGitRepository(gen.InitGitOptions{
			TargetPath:      targetDir,
			TemplateName:    selectedConfig.Name,
			TemplateVersion: selectedConfig.Version,
		})
		return err
	}
	return nil
}

// renderDryRunPreview renders a preview of template files without writing to disk.
func renderDryRunPreview(
	selectedConfig *templates.Configuration,
	targetDir string,
	templateVars map[string]interface{},
) error {
	renderDryRunHeader(selectedConfig, targetDir)

	mergedValues, err := loadDryRunValues(selectedConfig, templateVars)
	if err != nil {
		return err
	}

	renderDryRunFileList(selectedConfig, targetDir, mergedValues)
	return nil
}

// renderDryRunHeader renders the header information for dry-run mode.
func renderDryRunHeader(selectedConfig *templates.Configuration, targetDir string) {
	atmosui.Info("Dry-run mode: No files will be written")
	atmosui.Writef("\nTemplate: %s\n", selectedConfig.Name)
	if selectedConfig.Description != "" {
		atmosui.Writef("Description: %s\n", selectedConfig.Description)
	}
	atmosui.Writef("Target directory: %s\n\n", targetDir)
}

// loadDryRunValues loads configuration values for dry-run preview using defaults.
func loadDryRunValues(selectedConfig *templates.Configuration, templateVars map[string]interface{}) (map[string]interface{}, error) {
	// Create a copy to avoid mutating the caller's map.
	mergedValues := make(map[string]interface{}, len(templateVars))
	for k, v := range templateVars {
		mergedValues[k] = v
	}

	if !templates.HasScaffoldConfig(selectedConfig.Files) {
		return mergedValues, nil
	}

	scaffoldConfigFile := findScaffoldConfigFile(selectedConfig.Files)
	if scaffoldConfigFile == nil {
		return mergedValues, nil
	}

	scaffoldConfig, err := config.LoadScaffoldConfigFromContent(scaffoldConfigFile.Content)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldParseYAML).
			WithCause(err).
			WithExplanation("Failed to load scaffold configuration for dry-run preview").
			WithHint("Check the `scaffold.yaml` syntax in your template").
			WithHint("Run `atmos scaffold validate` to check for errors").
			WithExitCode(2).
			Err()
	}

	// Merge with defaults from scaffold config, preserving declared order.
	for i := range scaffoldConfig.Spec.Fields {
		field := &scaffoldConfig.Spec.Fields[i]
		if _, exists := mergedValues[field.Name]; !exists && field.Default != nil {
			mergedValues[field.Name] = field.Default
		}
	}

	return mergedValues, nil
}

// findScaffoldConfigFile finds the scaffold.yaml file in the configuration files.
func findScaffoldConfigFile(files []templates.File) *templates.File {
	for i := range files {
		if files[i].Path == config.ScaffoldConfigFileName {
			return &files[i]
		}
	}
	return nil
}

// renderDryRunFileList renders the list of files that would be generated.
func renderDryRunFileList(selectedConfig *templates.Configuration, targetDir string, mergedValues map[string]interface{}) {
	atmosui.Write("Files that would be generated:\n\n")

	// Reuse the same template processor that real generation uses so dry-run
	// previews render nested fields (e.g. `{{ .Config.project_name }}`) and
	// template functions exactly as the generated paths will.
	processor := engine.NewProcessor()

	fileCount := 0
	for _, file := range selectedConfig.Files {
		if file.Path == config.ScaffoldConfigFileName {
			continue
		}

		renderedPath := renderFilePath(processor, file.Path, mergedValues)
		printFilePath(targetDir, renderedPath)
		fileCount++
	}

	atmosui.Writef("\nTotal: %d files would be generated\n", fileCount)
}

// renderFilePath renders a file path template using the generation engine so the
// dry-run preview matches the paths produced during real generation. On any
// templating error it falls back to the raw path, since a preview must not fail.
func renderFilePath(processor *engine.Processor, path string, values map[string]interface{}) string {
	rendered, err := processor.ProcessTemplate(path, path, nil, values)
	if err != nil {
		return path
	}
	return rendered
}

// printFilePath prints a file path with proper formatting.
func printFilePath(targetDir, renderedPath string) {
	if targetDir != "" {
		fullPath := filepath.Join(targetDir, renderedPath)
		atmosui.Writef("  • %s\n", fullPath)
		return
	}
	atmosui.Writef("  • %s\n", renderedPath)
}

// executeTemplateWithoutTargetDir handles template execution when no target directory is provided.
func executeTemplateWithoutTargetDir(
	selectedConfig *templates.Configuration,
	opts *scaffoldGenerateOptions,
	scaffoldUI ScaffoldUI,
) (string, error) {
	if opts.interactive && !opts.dryRun {
		// Interactive mode: use ExecuteWithInteractiveFlow which will prompt for target directory.
		targetDir, err := scaffoldUI.ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, "", opts.force, opts.update, opts.useDefaults, opts.baseRef, opts.templateValues)
		if offer, retryBaseRef := shouldOfferScaffoldUpdate(err, opts); offer {
			if confirmed, cErr := scaffoldUI.ConfirmUpdateInstead(targetDir); cErr == nil && confirmed {
				return scaffoldUI.ExecuteWithInteractiveFlowAndBaseRefResult(selectedConfig, targetDir, opts.force, true, opts.useDefaults, retryBaseRef, opts.templateValues)
			}
		}
		return targetDir, err
	}

	// Without a terminal (or in dry-run mode) the target cannot be prompted for.
	return "", errUtils.Build(errUtils.ErrTargetDirRequired).
		WithExplanation("Target directory is required in non-interactive mode").
		WithHint("Specify target directory: `atmos scaffold generate <template> <target>`").
		WithContext("interactive", opts.interactive).
		WithContext("dry_run", opts.dryRun).
		WithExitCode(2).
		Err()
}

// executeScaffoldList lists all available scaffold templates (embedded and configured).
// This logic was moved from internal/exec/scaffold.go to keep command logic in cmd/.
func executeScaffoldList(_ *cobra.Command) error {
	// Load all available templates (embedded + catalog + atmos.yaml).
	configs, origins, scaffoldUI, err := loadScaffoldTemplates("")
	if err != nil {
		return err
	}

	// Check if there are any templates.
	if len(configs) == 0 {
		atmosui.Info("No scaffold templates available.")
		atmosui.Info("Add templates to the 'scaffold.templates' section in atmos.yaml to get started.")
		return nil
	}

	// Build table data from templates.Configuration map, sorted by template
	// name for deterministic output. Row order must match the table columns:
	// Template, Source, Version, Description.
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)

	header := []string{"Template", "Source", "Version", "Description"}
	var rows [][]string
	for _, name := range names {
		// Use tracked origin instead of inferring from TargetDir.
		source := origins[name]
		if source == "" {
			source = config.SourceEmbedded // Fallback for safety.
		}
		version := configs[name].Version
		if version == "" {
			version = "-"
		}
		description := configs[name].Description
		if description == "" {
			description = "-"
		}
		rows = append(rows, []string{name, source, version, description})
	}

	scaffoldUI.DisplayTemplateTable(header, rows)

	return nil
}

// executeValidateScaffold validates scaffold.yaml files against the scaffold schema.
// This logic was moved from internal/exec/validate_scaffold.go to keep command logic in cmd/.
func executeValidateScaffold(
	_ context.Context,
	path string,
) error {
	// Find scaffold files to validate
	scaffoldPaths, err := determineScaffoldPathsToValidate(path)
	if err != nil {
		return err
	}

	if len(scaffoldPaths) == 0 {
		atmosui.Info("No scaffold.yaml files found to validate")
		return nil
	}

	// Validate all scaffold files.
	validCount, errorCount := validateAllScaffoldFiles(scaffoldPaths)

	// Print summary and return result.
	return printValidationSummary(validCount, errorCount)
}

// determineScaffoldPathsToValidate finds scaffold files based on the provided path.
func determineScaffoldPathsToValidate(path string) ([]string, error) {
	searchPath := path
	if searchPath == "" {
		searchPath = "."
	}

	paths, err := findScaffoldFiles(searchPath)
	if err != nil {
		return nil, errors.Join(errUtils.ErrScaffoldValidation, err)
	}

	return paths, nil
}

// validateAllScaffoldFiles validates each scaffold file and returns counts.
func validateAllScaffoldFiles(scaffoldPaths []string) (validCount int, errorCount int) {
	for _, scaffoldPath := range scaffoldPaths {
		atmosui.Infof("Validating %s", scaffoldPath)

		if validationErr := validateScaffoldFile(scaffoldPath); validationErr != nil {
			atmosui.Errorf("%s: %v", scaffoldPath, validationErr)
			errorCount++
		} else {
			atmosui.Successf("%s: valid", scaffoldPath)
			validCount++
		}
	}

	return validCount, errorCount
}

// printValidationSummary prints the validation summary and returns an error if validation failed.
func printValidationSummary(validCount int, errorCount int) error {
	atmosui.Writeln("")
	atmosui.Writeln("Validation Summary:")
	atmosui.Successf("Valid files: %d", validCount)

	if errorCount > 0 {
		atmosui.Errorf("Invalid files: %d", errorCount)
		return errUtils.Build(errUtils.ErrScaffoldValidation).
			WithExplanationf("%d scaffold file(s) failed validation", errorCount).
			WithHint("Review the validation errors above and fix the issues: every scaffold.yaml is an `AtmosScaffoldConfig` manifest with `apiVersion`, `kind`, and `metadata.name`").
			WithHint("Questionnaire fields live under `spec.fields`; valid field types are `input`, `select`, `confirm`, and `multiselect`").
			WithContext("valid_count", validCount).
			WithContext("error_count", errorCount).
			WithExitCode(1).
			Err()
	}

	atmosui.Success("All scaffold files are valid")
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
		return validateSingleScaffoldFile(path, scaffoldPaths)
	}

	return findScaffoldFilesInDirectory(path, scaffoldPaths)
}

// validateSingleScaffoldFile validates a single scaffold.yaml file.
func validateSingleScaffoldFile(path string, scaffoldPaths []string) ([]string, error) {
	base := filepath.Base(path)
	if base == "scaffold.yaml" || base == "scaffold.yml" {
		scaffoldPaths = append(scaffoldPaths, path)
		return scaffoldPaths, nil
	}

	return nil, errUtils.Build(errUtils.ErrInvalidScaffoldFile).
		WithExplanationf("File must be named `scaffold.yaml` or `scaffold.yml`: `%s`", path).
		WithHint("Rename the file to `scaffold.yaml`").
		WithContext("path", path).
		WithExitCode(2).
		Err()
}

// findScaffoldFilesInDirectory recursively finds scaffold.yaml files in a directory.
func findScaffoldFilesInDirectory(path string, scaffoldPaths []string) ([]string, error) {
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

	return scaffoldPaths, nil
}

// validateScaffoldFile validates a single scaffold.yaml file against the
// AtmosScaffoldConfig manifest schema.
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

	// Validate against the generated AtmosScaffoldConfig JSON Schema,
	// including the manifest envelope (apiVersion, kind, metadata).
	if err := manifest.Validate(config.ScaffoldKind, scaffoldData); err != nil {
		return errUtils.Build(errUtils.ErrScaffoldValidation).
			WithCause(err).
			WithExplanationf("Invalid scaffold manifest: `%s`", scaffoldPath).
			WithExample(scaffoldManifestExample).
			WithContext("path", scaffoldPath).
			WithExitCode(2).
			Err()
	}

	return nil
}

// scaffoldManifestExample is a minimal valid AtmosScaffoldConfig manifest
// shown in validation error messages.
const scaffoldManifestExample = "```yaml\napiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: my-scaffold\n  description: My scaffold template\nspec:\n  fields:\n    - name: project_name\n      type: input\n      default: my-project\n```"

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

	source, _ := templateMap["source"].(string)
	if source == "" {
		return templates.Configuration{}, errUtils.Build(errUtils.ErrInvalidTemplateData).
			WithExplanationf("Template `%s` has no `source` in `atmos.yaml`", name).
			WithHint("Set `source` to a local directory containing the template files").
			WithExample("```yaml\nscaffold:\n  templates:\n    my-template:\n      description: My template\n      source: ./scaffolds/my-template\n```").
			WithContext("template_name", name).
			WithExitCode(2).
			Err()
	}

	// Remote sources (git/https/s3) are advertised as stubs and fetched lazily
	// at generation time by the source package; local sources load eagerly so
	// `atmos scaffold list` can show their version and description.
	if !vendor.IsLocalPath(source) && !vendor.IsFileURI(source) {
		stub := templates.Configuration{Name: name, Source: source}
		if desc, ok := templateMap["description"].(string); ok && desc != "" {
			stub.Description = desc
		}
		return stub, nil
	}

	// Load the template files from the local source directory. The path is
	// resolved relative to the current directory (where atmos.yaml lives).
	cfg, err := templates.LoadConfigurationFromDir(name, source)
	if err != nil {
		return templates.Configuration{}, err
	}

	// An explicit description in atmos.yaml overrides the template's own metadata.
	if desc, ok := templateMap["description"].(string); ok && desc != "" {
		cfg.Description = desc
	}

	// Extract target_dir if provided
	if targetDir, ok := templateMap["target_dir"].(string); ok {
		cfg.TargetDir = targetDir
	}

	return *cfg, nil
}
