package ci

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cipkg "github.com/cloudposse/atmos/pkg/ci"
	civalidate "github.com/cloudposse/atmos/pkg/ci/validate"
	"github.com/cloudposse/atmos/pkg/ci/validate/githubactions"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/validation"
)

const (
	validateFormatText  = "text"
	validateFormatRich  = "rich"
	validateFormatSARIF = "sarif"

	// The ciFormatFlagName constant names the shared "format" flag, referenced
	// by both the direct cobra registration and the standard-parser env-var
	// wiring below.
	ciFormatFlagName = "format"
)

var errWorkflowValidationFailed = errors.New("GitHub Actions workflow validation failed")

// ciValidateFormatEnvParser wires the local "format" flag through the standard
// parser so ATMOS_VALIDATE_FORMAT follows the standard flag > env var > config
// > default precedence via pkg/flags, replacing a direct os.Getenv call. It is
// only used for BindFlagsToViper, which binds this command's existing "format"
// flag plus the environment variable.
var ciValidateFormatEnvParser = flags.NewStandardParser(
	flags.WithStringFlag(ciFormatFlagName, "", "", "Output format"),
	flags.WithEnvVars(ciFormatFlagName, "ATMOS_VALIDATE_FORMAT"),
)

// ciValidationConfigLoader is replaceable so command tests can isolate
// configuration loading from the validator and output behavior.
var ciValidationConfigLoader = func(cmd *cobra.Command) (*schema.AtmosConfiguration, error) {
	globalFlags := flags.ParseGlobalFlags(cmd, viper.GetViper())
	atmosConfig, err := cfg.InitCliConfig(buildConfigAndStacksInfo(&globalFlags), false)
	if err != nil {
		return nil, err
	}
	return &atmosConfig, nil
}

// NewValidateCommand creates the GitHub Actions validation command. It is
// mounted at both `atmos ci validate` and the validation-oriented alias
// `atmos validate ci`.
func NewValidateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "validate [workflow-file ...]",
		Short: "Validate GitHub Actions workflow files",
		Long: `Validate GitHub Actions workflows with Atmos's built-in actionlint integration.

Without workflow-file arguments, all YAML workflows in .github/workflows are
checked. Supplying one or more workflow files limits validation to those files.
Use --workflow-path to recursively validate a different workflow directory.
The command respects .github/actionlint.yaml and .github/actionlint.yml.`,
		Args: cobra.ArbitraryArgs,
		RunE: runCIValidate,
	}
	command.Flags().String(ciFormatFlagName, validateFormatText, "Output format: text, rich, sarif")
	command.Flags().String("workflow-path", "", "Path to a directory of GitHub Actions workflow files")
	command.Flags().Bool("affected", false, "Validate only workflow files affected since the Git merge-base")
	command.Flags().String("base", "", "Git base ref or SHA to compare against for affected validation")
	command.Flags().StringSlice("exclude", nil, "Exclude repository paths from validation (glob; can be repeated)")
	return command
}

// validateCmd is the `atmos ci validate` command instance.
var validateCmd = NewValidateCommand()

func runCIValidate(cmd *cobra.Command, args []string) error {
	return runCIValidateWithPaths(cmd, args, nil, false, nil)
}

// RunValidateFiles validates the provided workflow files. A nil paths list
// retains whole-directory validation and is used by aggregate validation when
// an actionlint configuration change can affect every workflow.
func RunValidateFiles(cmd *cobra.Command, paths []string) error {
	return runCIValidateWithPaths(cmd, nil, paths, true, nil)
}

// RunValidateFilesExcluding validates workflow files while excluding paths
// matched by repository-relative glob patterns.
func RunValidateFilesExcluding(cmd *cobra.Command, paths []string, excludes []string) error {
	return runCIValidateWithPaths(cmd, nil, paths, true, excludes)
}

// runCIValidateWithPaths coordinates the mutually exclusive workflow,
// affected, and exclusion inputs as a flat pipeline of named steps.
func runCIValidateWithPaths(cmd *cobra.Command, args []string, paths []string, pathsProvided bool, additionalExcludes []string) error {
	format, err := resolveCIValidateFormat(cmd)
	if err != nil {
		return err
	}

	workflowPath, excludes, err := resolveCIValidateFlags(cmd, args, additionalExcludes)
	if err != nil {
		return err
	}

	selection, err := resolveCIValidateSelection(cmd, args, paths, pathsProvided, workflowPath)
	if err != nil {
		return err
	}
	if selection.earlyExit {
		return nil
	}
	args, workflowPath = selection.args, selection.workflowPath

	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	args, workflowPath, err = applyCIValidateExcludes(root, args, workflowPath, excludes)
	if err != nil {
		return err
	}

	report, err := validateGitHubActionsWorkflows(cmd, root, args, workflowPath)
	if err != nil {
		return err
	}

	return finishCIValidate(cmd, format, &ciValidateOutcome{
		report:        report,
		root:          root,
		explicitPaths: len(args) > 0,
		workflowPath:  workflowPath,
	})
}

// resolveCIValidateFormat resolves the requested output format following the
// standard flag > env var > config > default precedence via pkg/flags'
// ciValidateFormatEnvParser, instead of a direct os.Getenv call.
func resolveCIValidateFormat(cmd *cobra.Command) (string, error) {
	format, err := cmd.Flags().GetString(ciFormatFlagName)
	if err != nil {
		return "", err
	}
	if !cmd.Flags().Changed(ciFormatFlagName) {
		if fallback := ciValidateFormatFallback(cmd); fallback != "" {
			format = fallback
		}
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format != validateFormatText && format != validateFormatRich && format != validateFormatSARIF {
		return "", fmt.Errorf("%w: %q", errUtils.ErrUnsupportedCIValidationFormat, format)
	}
	return format, nil
}

// ciValidateFormatFallback resolves the env var > atmos.yaml fallback chain
// used when the --format flag was not set explicitly, returning "" when
// neither source has a value.
func ciValidateFormatFallback(cmd *cobra.Command) string {
	if err := ciValidateFormatEnvParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
		return ""
	}
	if value := strings.TrimSpace(viper.GetString(ciFormatFlagName)); value != "" {
		return value
	}
	atmosConfig, err := ciValidationConfigLoader(cmd)
	if err != nil {
		return ""
	}
	return atmosConfig.Validate.Format
}

// resolveCIValidateFlags reads and validates the workflow-path and exclude
// flags, and rejects the mutually exclusive workflow-path/file-arguments
// combination.
func resolveCIValidateFlags(cmd *cobra.Command, args []string, additionalExcludes []string) (string, []string, error) {
	workflowPath, err := cmd.Flags().GetString("workflow-path")
	if err != nil {
		return "", nil, err
	}
	workflowPath = strings.TrimSpace(workflowPath)
	excludes, err := cmd.Flags().GetStringSlice("exclude")
	if err != nil {
		return "", nil, err
	}
	if _, err := validation.ExcludePaths(nil, excludes); err != nil {
		return "", nil, err
	}
	excludes = append(excludes, additionalExcludes...)
	if _, err := validation.ExcludePaths(nil, excludes); err != nil {
		return "", nil, err
	}
	if workflowPath != "" && len(args) > 0 {
		return "", nil, errUtils.ErrWorkflowArgsWithWorkflowPath
	}
	return workflowPath, excludes, nil
}

// ciValidateSelectionResult is the resolved workflow file (or directory)
// selection for one validate invocation. EarlyExit is true when there is
// nothing to validate and the caller should return without validating.
type ciValidateSelectionResult struct {
	args         []string
	workflowPath string
	earlyExit    bool
}

// resolveCIValidateSelection determines the workflow files (or directory) to
// validate for the mutually exclusive explicit-path, aggregate, and
// --affected selection modes. The result's earlyExit field is true when there
// is nothing to validate and the caller should return the accompanying error
// (nil, or a write error) without validating.
func resolveCIValidateSelection(cmd *cobra.Command, args []string, paths []string, pathsProvided bool, workflowPath string) (ciValidateSelectionResult, error) {
	if pathsProvided {
		return ciValidateSelectionResult{args: paths}, nil
	}
	affected, _ := cmd.Flags().GetBool("affected")
	if !affected {
		return ciValidateSelectionResult{args: args, workflowPath: workflowPath}, nil
	}
	if len(args) > 0 || workflowPath != "" {
		return ciValidateSelectionResult{}, errUtils.ErrAffectedWithFileArgsOrPath
	}
	base, err := cmd.Flags().GetString("base")
	if err != nil {
		return ciValidateSelectionResult{}, err
	}
	affectedPaths, err := validation.AffectedFiles(base)
	if err != nil {
		return ciValidateSelectionResult{}, err
	}
	if actionlintConfigChanged(affectedPaths) {
		return ciValidateSelectionResult{}, nil
	}
	resolvedArgs := affectedWorkflowFiles(affectedPaths)
	if len(resolvedArgs) == 0 {
		_, writeErr := fmt.Fprintln(cmd.OutOrStdout(), "No affected GitHub Actions workflow files to validate.")
		return ciValidateSelectionResult{earlyExit: true}, writeErr
	}
	return ciValidateSelectionResult{args: resolvedArgs}, nil
}

// applyCIValidateExcludes filters args by excludes, or expands workflowPath
// into an explicit file list when no args were selected yet, so the exclude
// globs apply uniformly to both selection styles.
func applyCIValidateExcludes(root string, args []string, workflowPath string, excludes []string) ([]string, string, error) {
	if len(excludes) == 0 {
		return args, workflowPath, nil
	}
	if len(args) > 0 {
		filtered, err := validation.ExcludePaths(args, excludes)
		if err != nil {
			return nil, "", err
		}
		return filtered, workflowPath, nil
	}
	path := workflowPath
	if path == "" {
		path = filepath.Join(root, ".github", "workflows")
	}
	filtered, err := workflowFilesExcluding(root, path, excludes)
	if err != nil {
		return nil, "", err
	}
	return filtered, "", nil
}

// validateGitHubActionsWorkflows runs the registered GitHub Actions validator
// against the resolved file selection.
func validateGitHubActionsWorkflows(cmd *cobra.Command, root string, args []string, workflowPath string) (validation.Report, error) {
	validator, ok := civalidate.Get(githubactions.ValidatorName)
	if !ok {
		return validation.Report{}, fmt.Errorf("%w: %q", errUtils.ErrCIValidatorNotRegistered, githubactions.ValidatorName)
	}
	report, err := validator.Validate(cmd.Context(), civalidate.Request{
		Root:         root,
		Paths:        args,
		WorkflowPath: workflowPath,
	})
	if err != nil {
		return validation.Report{}, fmt.Errorf("validate GitHub Actions workflows: %w", err)
	}
	return report, nil
}

// ciValidateOutcome bundles the validator's report with the context needed to
// render and report it, keeping finishCIValidate's argument count within
// this repo's function argument limit.
type ciValidateOutcome struct {
	report        validation.Report
	root          string
	explicitPaths bool
	workflowPath  string
}

// finishCIValidate writes the report, publishes CI annotations when enabled,
// and translates validation findings into the command's exit behavior.
func finishCIValidate(cmd *cobra.Command, format string, outcome *ciValidateOutcome) error {
	if err := writeCIValidationOutput(format, outcome.report, outcome.root, outcome.explicitPaths, outcome.workflowPath); err != nil {
		return err
	}

	// SARIF output is deliberately an explicit, side-effect-free output mode.
	// Rich is a presentation choice, not a separate CI reporting channel.
	if format != validateFormatSARIF && ciValidationAnnotationsEnabled(cmd) {
		if err := cipkg.Annotate(outcome.report.ToAnnotations()); err != nil {
			log.Warn("Failed to publish GitHub Actions validation annotations", "error", err)
		}
	}

	if outcome.report.HasErrors() {
		if format == validateFormatRich {
			return errUtils.ExitCodeError{Code: 1, Silent: true}
		}
		return workflowValidationError(outcome.report)
	}
	return nil
}

func workflowFilesExcluding(root string, workflowPath string, excludes []string) ([]string, error) {
	if !filepath.IsAbs(workflowPath) {
		workflowPath = filepath.Join(root, workflowPath)
	}

	files := make([]string, 0)
	err := filepath.Walk(workflowPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension != ".yaml" && extension != ".yml" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return validation.ExcludePaths(files, excludes)
}

func actionlintConfigChanged(paths []string) bool {
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(path))
		if path == ".github/actionlint.yaml" || path == ".github/actionlint.yml" {
			return true
		}
	}
	return false
}

func affectedWorkflowFiles(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(path))
		if strings.HasPrefix(path, ".github/workflows/") && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			if _, err := os.Stat(filepath.FromSlash(path)); err == nil {
				result = append(result, path)
			}
		}
	}
	return result
}

func writeCIValidationOutput(format string, report validation.Report, root string, explicitPaths bool, workflowPath string) error {
	if format == validateFormatSARIF {
		body, err := report.SARIF()
		if err != nil {
			return err
		}
		if err := data.Write(string(body)); err != nil {
			return fmt.Errorf("write SARIF output: %w", err)
		}
		return nil
	}
	if format == validateFormatRich {
		if !report.HasErrors() {
			ui.Success("No GitHub Actions workflow validation findings")
			return nil
		}
		output := validation.Rich(report, validation.DefaultRichOptions(root))
		if output == "" {
			return nil
		}
		ui.Writeln(output)
		return nil
	}
	if report.HasErrors() {
		return nil
	}
	ui.Success(validationSuccessMessage(report, explicitPaths, workflowPath))
	return nil
}

// workflowValidationError returns the validator's diagnostics as an Atmos-formatted error.
// Actionlint's native formatting is captured before it reaches the terminal, so Atmos owns the output.
func workflowValidationError(report validation.Report) error {
	return errUtils.Build(errWorkflowValidationFailed).
		WithExplanation("```text\n" + actionlintOutput(report) + "\n```").
		WithExitCode(1).
		Err()
}

func actionlintOutput(report validation.Report) string {
	diagnostics := strings.TrimSpace(report.RenderedDiagnostics)
	if diagnostics != "" {
		return diagnostics
	}
	return strings.TrimSpace(civalidate.Text(report))
}

func validationSuccessMessage(report validation.Report, explicitPaths bool, workflowPath string) string {
	if explicitPaths {
		return fmt.Sprintf("Validated **%d** GitHub Actions workflow file(s).", report.FilesChecked)
	}
	if workflowPath != "" {
		return fmt.Sprintf("Validated **%d** GitHub Actions workflow file(s) in `%s`.", report.FilesChecked, workflowPath)
	}
	return fmt.Sprintf("Validated **%d** GitHub Actions workflow file(s) in `.github/workflows`.", report.FilesChecked)
}

func ciValidationAnnotationsEnabled(cmd *cobra.Command) bool {
	provider := cipkg.Detect()
	if provider == nil || provider.Name() != githubactions.ValidatorName {
		return false
	}

	atmosConfig, err := ciValidationConfigLoader(cmd)
	if err != nil {
		// A local configuration error must not hide lint diagnostics. It also
		// means CI reporting cannot safely be considered enabled.
		log.Debug("Could not load Atmos configuration for CI validation annotations", "error", err)
		return false
	}
	if !cipkg.Enabled(atmosConfig) {
		log.Debug("CI validation annotations disabled because ci.enabled is false")
		return false
	}
	return cipkg.AnnotationsEnabled(atmosConfig)
}
