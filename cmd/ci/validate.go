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
)

var errWorkflowValidationFailed = errors.New("GitHub Actions workflow validation failed")

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
	command.Flags().String("format", validateFormatText, "Output format: text, rich, sarif")
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

//nolint:cyclop,funlen,gocognit,revive // The command coordinates mutually exclusive workflow, affected, and exclusion inputs.
func runCIValidateWithPaths(cmd *cobra.Command, args []string, paths []string, pathsProvided bool, additionalExcludes []string) error {
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed("format") {
		if value := strings.TrimSpace(os.Getenv("ATMOS_VALIDATE_FORMAT")); value != "" {
			format = value
		} else if atmosConfig, configErr := ciValidationConfigLoader(cmd); configErr == nil && atmosConfig.Validate.Format != "" {
			format = atmosConfig.Validate.Format
		}
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format != validateFormatText && format != validateFormatRich && format != validateFormatSARIF {
		return fmt.Errorf("unsupported format %q: expected text, rich, or sarif", format)
	}
	workflowPath, err := cmd.Flags().GetString("workflow-path")
	if err != nil {
		return err
	}
	workflowPath = strings.TrimSpace(workflowPath)
	excludes, err := cmd.Flags().GetStringSlice("exclude")
	if err != nil {
		return err
	}
	if _, err := validation.ExcludePaths(nil, excludes); err != nil {
		return err
	}
	excludes = append(excludes, additionalExcludes...)
	if _, err := validation.ExcludePaths(nil, excludes); err != nil {
		return err
	}
	if workflowPath != "" && len(args) > 0 {
		return errors.New("workflow-file arguments cannot be used with --workflow-path")
	}
	if pathsProvided {
		args = paths
		workflowPath = ""
	} else if affected, _ := cmd.Flags().GetBool("affected"); affected {
		if len(args) > 0 || workflowPath != "" {
			return errors.New("--affected cannot be used with workflow-file arguments or --workflow-path")
		}
		base, err := cmd.Flags().GetString("base")
		if err != nil {
			return err
		}
		affectedPaths, err := validation.AffectedFiles(base)
		if err != nil {
			return err
		}
		if actionlintConfigChanged(affectedPaths) {
			args = nil
		} else {
			args = affectedWorkflowFiles(affectedPaths)
			if len(args) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No affected GitHub Actions workflow files to validate.")
				return err
			}
		}
	}

	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	//nolint:nestif // Selection differs for an explicit file list and a workflow directory.
	if len(excludes) > 0 {
		if len(args) == 0 {
			path := workflowPath
			if path == "" {
				path = filepath.Join(root, ".github", "workflows")
			}
			args, err = workflowFilesExcluding(root, path, excludes)
			if err != nil {
				return err
			}
			workflowPath = ""
		} else {
			args, err = validation.ExcludePaths(args, excludes)
			if err != nil {
				return err
			}
		}
	}

	validator, ok := civalidate.Get(githubactions.ValidatorName)
	if !ok {
		return fmt.Errorf("GitHub Actions validator %q is not registered", githubactions.ValidatorName)
	}
	report, err := validator.Validate(cmd.Context(), civalidate.Request{
		Root:         root,
		Paths:        args,
		WorkflowPath: workflowPath,
	})
	if err != nil {
		return fmt.Errorf("validate GitHub Actions workflows: %w", err)
	}

	if err := writeCIValidationOutput(format, report, root, len(args) > 0, workflowPath); err != nil {
		return err
	}

	// SARIF output is deliberately an explicit, side-effect-free output mode.
	// Rich is a presentation choice, not a separate CI reporting channel.
	if format != validateFormatSARIF && ciValidationAnnotationsEnabled(cmd) {
		if err := cipkg.Annotate(report.ToAnnotations()); err != nil {
			log.Warn("Failed to publish GitHub Actions validation annotations", "error", err)
		}
	}

	if report.HasErrors() {
		if format == validateFormatRich {
			return errUtils.ExitCodeError{Code: 1, Silent: true}
		}
		return workflowValidationError(report)
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
		return fmt.Sprintf("Validated **%d** GitHub Actions workflow file(s) in %c%s%c.", report.FilesChecked, 96, workflowPath, 96)
	}
	return fmt.Sprintf("Validated **%d** GitHub Actions workflow file(s) in %c.github/workflows%c.", report.FilesChecked, 96, 96)
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
