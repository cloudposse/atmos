package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	er "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/error"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/files"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/outputformat"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/utils"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/validation"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	u "github.com/cloudposse/atmos/pkg/utils"
	validateReport "github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/version"
)

var (
	// DefaultConfigFileNames determines the file names where the config is located.
	defaultConfigFileNames  = []string{".editorconfig", ".editorconfig-checker.json", ".ecrc"}
	initEditorConfig        bool
	currentConfig           *config.Config
	cliConfig               config.Config
	configFilePaths         []string
	tmpExclude              string
	format                  string
	editorConfigSARIF       bool
	editorConfigRich        bool
	editorConfigAnnotate    = ci.Annotate
	editorConfigReportSARIF = ci.ReportSARIF
)

// ciFlagsParser wires the shared "ci" flag through the standard parser so it
// follows the standard flag > env var > config > default precedence via
// pkg/flags, replacing direct viper.BindPFlag/BindEnv calls.
var ciFlagsParser = flags.NewStandardParser(
	flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (annotations, SARIF upload)"),
	flags.WithEnvVars("ci", "ATMOS_CI", "CI"),
)

var editorConfigCmd *cobra.Command = &cobra.Command{
	Use:   "editorconfig",
	Short: "Validate all files against the EditorConfig",
	Long:  "Validate all files against the project's EditorConfig rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		if len(args) > 0 {
			showUsageAndExit(cmd, args)
		}
		return runEditorConfigCommand(cmd)
	},
}

// runEditorConfigCommand preserves the established standalone behavior for
// upstream-rendered formats. Aggregate validation calls runEditorConfig
// directly so it can collect the failure without terminating the process.
func runEditorConfigCommand(cmd *cobra.Command) error {
	err := runEditorConfig(cmd)
	if !errors.Is(err, errUtils.ErrEditorConfigValidationFailed) {
		return err
	}
	if editorConfigSARIF || editorConfigRich {
		return errUtils.ExitCodeError{Code: 1, Silent: true}
	}
	errUtils.Exit(1)
	return nil
}

// parseConfigPaths extracts config file paths from command flags.
// Returns the paths specified via --config flag, or default paths if not specified.
func parseConfigPaths(cmd *cobra.Command) []string {
	if cmd.Flags().Changed("config") {
		configFlag := cmd.Flags().Lookup("config")
		if configFlag != nil {
			return strings.Split(configFlag.Value.String(), ",")
		}
	}
	return defaultConfigFileNames
}

// initializeConfig breaks the initialization cycle by separating the config setup.
func initializeConfig(cmd *cobra.Command) error {
	editorConfigSARIF = false
	editorConfigRich = false
	configFilePaths = nil
	if err := replaceAtmosConfigInConfig(cmd, &atmosConfig); err != nil {
		return err
	}

	configPaths := parseConfigPaths(cmd)
	if !cmd.Flags().Changed("config") && len(configFilePaths) > 0 {
		configPaths = configFilePaths
	}
	configFilePaths = configPaths

	currentConfig = config.NewConfig(configPaths)

	if initEditorConfig {
		err := currentConfig.Save(version.Version)
		if err != nil {
			return err
		}
	}

	if err := currentConfig.Parse(); err != nil {
		log.Trace("Failed to parse EditorConfig configuration", "error", err, "paths", configPaths)
	}

	if tmpExclude != "" {
		currentConfig.Exclude = append(currentConfig.Exclude, tmpExclude)
	}

	currentConfig.Merge(cliConfig)

	return nil
}

func replaceAtmosConfigInConfig(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) error {
	applyEditorConfigPathOverrides(cmd, atmosConfig)
	if err := applyEditorConfigFormatOverride(cmd, atmosConfig); err != nil {
		return err
	}
	applyEditorConfigVerbosityOverride(cmd, atmosConfig)
	applyEditorConfigColorOverride(cmd, atmosConfig)
	applyEditorConfigDisableOverrides(cmd, atmosConfig)

	return nil
}

// applyEditorConfigPathOverrides applies atmos.yaml fallbacks for the
// EditorConfig command's file-discovery flags (--config, --init,
// --ignore-defaults, --dry-run) whenever the corresponding flag was not set
// explicitly on the command line.
func applyEditorConfigPathOverrides(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) {
	if !cmd.Flags().Changed("config") && len(atmosConfig.Validate.EditorConfig.ConfigFilePaths) > 0 {
		configFilePaths = atmosConfig.Validate.EditorConfig.ConfigFilePaths
	}
	// atmosConfig.Validate.EditorConfig.Exclude is glob-matched via
	// runEditorConfig's excludes parameter (pkg/validation.ExcludePaths), not
	// fed into tmpExclude here: this command's own --exclude flag stays a
	// regex (editorconfig-checker's native, pre-existing contract), while the
	// atmos.yaml config field uses glob -- the syntax most users reach for,
	// and the same one every other `validate ... --exclude` flag in this repo
	// already speaks.
	if !cmd.Flags().Changed("init") && atmosConfig.Validate.EditorConfig.Init {
		initEditorConfig = atmosConfig.Validate.EditorConfig.Init
	}
	if !cmd.Flags().Changed("ignore-defaults") && atmosConfig.Validate.EditorConfig.IgnoreDefaults {
		cliConfig.IgnoreDefaults = atmosConfig.Validate.EditorConfig.IgnoreDefaults
	}
	if !cmd.Flags().Changed("dry-run") && atmosConfig.Validate.EditorConfig.DryRun {
		cliConfig.DryRun = atmosConfig.Validate.EditorConfig.DryRun
	}
}

// applyEditorConfigFormatOverride resolves the requested output format (flag >
// env var > atmos.yaml EditorConfig format > atmos.yaml Validate format) and
// configures the vendored checker's format plus the SARIF/rich output
// switches.
func applyEditorConfigFormatOverride(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) error {
	requestedFormat := requestedEditorConfigFormat(cmd, atmosConfig)
	if requestedFormat == "" {
		return nil
	}
	isSARIF, err := configureEditorConfigFormat(requestedFormat)
	if err != nil {
		return err
	}
	editorConfigSARIF = isSARIF
	return nil
}

// applyEditorConfigVerbosityOverride enables verbose output when the log
// level -- from either a flag or atmos.yaml -- is Trace.
func applyEditorConfigVerbosityOverride(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) {
	traceFromConfig := !cmd.Flags().Changed("logs-level") && atmosConfig.Logs.Level == u.LogLevelTrace
	traceFromFlag := false
	if cmd.Flags().Changed("logs-level") {
		if v, err := cmd.Flags().GetString("logs-level"); err == nil {
			if parsedLevel, parseErr := log.ParseLogLevel(v); parseErr == nil {
				traceFromFlag = parsedLevel == u.LogLevelTrace
			}
		}
	}
	if traceFromConfig || traceFromFlag {
		cliConfig.Verbose = true
	}
}

// applyEditorConfigColorOverride resolves --no-color from either the flag or
// atmos.yaml's terminal settings.
func applyEditorConfigColorOverride(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) {
	if !cmd.Flags().Changed("no-color") && atmosConfig.Settings.Terminal.NoColor {
		cliConfig.NoColor = atmosConfig.Settings.Terminal.NoColor
	} else if cmd.Flags().Changed("no-color") {
		cliConfig.NoColor, _ = cmd.Flags().GetBool("no-color")
	}
}

// editorConfigDisableOverride pairs a --disable-* flag name with the
// atmos.yaml source field and vendored-checker target field it feeds when the
// flag was not set explicitly.
type editorConfigDisableOverride struct {
	flagName string
	source   *bool
	target   *bool
}

// applyEditorConfigDisableOverrides applies atmos.yaml fallbacks for each
// --disable-* flag whenever the corresponding flag was not set explicitly.
func applyEditorConfigDisableOverrides(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) {
	overrides := []editorConfigDisableOverride{
		{"disable-trim-trailing-whitespace", &atmosConfig.Validate.EditorConfig.DisableTrimTrailingWhitespace, &cliConfig.Disable.TrimTrailingWhitespace},
		{"disable-end-of-line", &atmosConfig.Validate.EditorConfig.DisableEndOfLine, &cliConfig.Disable.EndOfLine},
		{"disable-insert-final-newline", &atmosConfig.Validate.EditorConfig.DisableInsertFinalNewline, &cliConfig.Disable.InsertFinalNewline},
		{"disable-indentation", &atmosConfig.Validate.EditorConfig.DisableIndentation, &cliConfig.Disable.Indentation},
		{"disable-indent-size", &atmosConfig.Validate.EditorConfig.DisableIndentSize, &cliConfig.Disable.IndentSize},
		{"disable-max-line-length", &atmosConfig.Validate.EditorConfig.DisableMaxLineLength, &cliConfig.Disable.MaxLineLength},
	}
	for _, override := range overrides {
		if !cmd.Flags().Changed(override.flagName) && *override.source {
			*override.target = *override.source
		}
	}
}

// requestedEditorConfigFormat resolves the requested output format following
// the standard flag > env var > config > default precedence via pkg/flags'
// validateFormatEnvParser (shared with the other validate commands), instead
// of a direct os.Getenv call.
func requestedEditorConfigFormat(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) string {
	if aggregateValidationFormat != "" {
		return aggregateValidationFormat
	}
	if cmd.Flags().Changed("format") {
		return format
	}
	if err := validateFormatEnvParser.BindFlagsToViper(cmd, viper.GetViper()); err == nil {
		if value := strings.TrimSpace(viper.GetString("format")); value != "" {
			return value
		}
	}
	if value := atmosConfig.Validate.EditorConfig.Format; value != "" {
		return value
	}
	return atmosConfig.Validate.Format
}

func configureEditorConfigFormat(value string) (bool, error) {
	editorConfigRich = false
	if strings.EqualFold(value, "sarif") {
		// The vendored checker does not know SARIF. Keep its config on a valid
		// value while Atmos renders the collected diagnostics itself.
		cliConfig.Format = outputformat.Default
		return true, nil
	}
	if strings.EqualFold(value, "rich") {
		// Rich is also Atmos-owned so it never reaches the vendored renderer.
		cliConfig.Format = outputformat.Default
		editorConfigRich = true
		return false, nil
	}

	upstreamFormat := outputformat.OutputFormat(value)
	if !upstreamFormat.IsValid() {
		return false, fmt.Errorf("%w: %v is not a valid format choose from the following: %v, sarif, rich", errUtils.ErrEditorConfigInvalidFormat, value, outputformat.GetArgumentChoiceText())
	}
	cliConfig.Format = upstreamFormat
	return false, nil
}

// runEditorConfig executes EditorConfig validation without terminating the process.
// It can therefore be composed by aggregate validators.
func runEditorConfig(cmd *cobra.Command) error {
	// Re-bind on every invocation (not just at init) so the "ci" flag/env
	// wiring survives a viper instance being reset between invocations, the
	// same pattern used by other standard-parser-based commands (e.g.
	// terraform apply/plan bind their parser inside RunE).
	if err := ciFlagsParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
		return err
	}
	affectedFiles, affected, err := validationAffectedFiles(cmd)
	if err != nil {
		return err
	}
	selectedFiles, validateAll := affectedEditorConfigFiles(affectedFiles)
	if affected && !validateAll && len(selectedFiles) == 0 {
		return validationNoAffectedFiles(cmd, "EditorConfig")
	}
	if !affected || validateAll {
		selectedFiles = nil
	}
	return runEditorConfigForFiles(cmd, selectedFiles, atmosConfig.Validate.EditorConfig.Exclude)
}

// runEditorConfigForFiles validates a subset of files when selectedFiles is
// non-nil. A nil list retains the established whole-project behavior.
//
//nolint:funlen,gocognit,revive // This preserves the established format and spinner branches while adding file selection.
func runEditorConfigForFiles(cmd *cobra.Command, selectedFiles []string, excludes []string) error {
	if err := initializeConfig(cmd); err != nil {
		return err
	}

	// atmosConfig.Validate.EditorConfig.Exclude (atmos.yaml) always applies,
	// regardless of entry point. The standalone `atmos validate editorconfig`
	// command already passes it in explicitly; the aggregate `atmos validate
	// --affected` path (cmd/validate_all.go's buildEditorConfigValidationTask)
	// only passes the CLI-level --exclude flags, so without this merge
	// atmos.yaml's excludes are silently dropped for every path except the
	// standalone command.
	excludes = append(append([]string{}, excludes...), atmosConfig.Validate.EditorConfig.Exclude...)

	config := *currentConfig
	log.Debug(config.String())
	log.Debug("Excluding", "regex", config.GetExcludesAsRegularExpression())

	if err := checkVersion(config); err != nil {
		return err
	}

	// Dry-run mode - just list files without spinner.
	if config.DryRun {
		filePaths, err := files.GetFiles(config)
		if err != nil {
			return err
		}
		filePaths, err = filterEditorConfigFiles(filePaths, selectedFiles, excludes)
		if err != nil {
			return err
		}
		for _, file := range filePaths {
			log.Info(file)
		}
		return nil
	}

	if editorConfigSARIF || editorConfigRich {
		return runEditorConfigStructuredOutput(cmd, &config, selectedFiles, excludes)
	}

	var filePaths []string
	var validationErrors []er.ValidationErrors

	err := spinner.ExecWithSpinner(
		"Validating EditorConfig...",
		"EditorConfig validation passed",
		func() error {
			var validationErr error
			filePaths, validationErr = files.GetFiles(config)
			if validationErr != nil {
				return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrEditorConfigGetFiles, validationErr)
			}
			filePaths, validationErr = filterEditorConfigFiles(filePaths, selectedFiles, excludes)
			if validationErr != nil {
				return validationErr
			}
			validationErrors = validation.ProcessValidation(filePaths, config)
			log.Debug("Files checked", "count", len(filePaths))
			if er.GetErrorCount(validationErrors) != 0 {
				return errUtils.ErrEditorConfigValidationFailed
			}
			return nil
		},
	)
	report := editorConfigDiagnostics(validationErrors)
	emitEditorConfigCI(cmd, report)
	if err != nil {
		if len(validationErrors) > 0 {
			er.PrintErrors(validationErrors, config)
		} else {
			ui.Error(fmt.Sprintf("Validation failed: %v", err))
		}
		return err
	}

	return nil
}

// runEditorConfigStructuredOutput handles the rich and SARIF output modes for
// EditorConfig validation, extracted from runEditorConfigForFiles to keep that
// function a flat pipeline.
func runEditorConfigStructuredOutput(cmd *cobra.Command, config *config.Config, selectedFiles, excludes []string) error {
	validationErrors, err := validateEditorConfigForFiles(config, selectedFiles, excludes)
	if err != nil && len(validationErrors) == 0 {
		ui.Error(fmt.Sprintf("Validation failed: %v", err))
		return err
	}
	report := editorConfigDiagnostics(validationErrors)
	emitEditorConfigCI(cmd, report)

	if editorConfigRich {
		if writeErr := writeEditorConfigRichOutput(cmd, report, err); writeErr != nil {
			return writeErr
		}
	} else if writeErr := writeEditorConfigSARIFOutput(cmd, report); writeErr != nil {
		return writeErr
	}

	if editorConfigRich && err != nil {
		return errUtils.ExitCodeError{Code: 1, Silent: true}
	}
	return err
}

// writeEditorConfigRichOutput renders the collected report as source-excerpt
// diagnostics, or a success message when there is nothing to show and
// validationErr is nil.
func writeEditorConfigRichOutput(cmd *cobra.Command, report validateReport.Report, validationErr error) error {
	root, rootErr := os.Getwd()
	if rootErr != nil {
		return rootErr
	}
	output := validateReport.Rich(report, validateReport.DefaultRichOptions(root))
	if output != "" {
		_, writeErr := fmt.Fprintln(cmd.OutOrStdout(), output)
		return writeErr
	}
	if validationErr == nil {
		_, writeErr := fmt.Fprintln(cmd.OutOrStdout(), "✓ EditorConfig validation passed")
		return writeErr
	}
	return nil
}

// writeEditorConfigSARIFOutput renders the collected report as a SARIF document.
func writeEditorConfigSARIFOutput(cmd *cobra.Command, report validateReport.Report) error {
	body, marshalErr := report.SARIF()
	if marshalErr != nil {
		ui.Error(fmt.Sprintf("Failed to render SARIF: %v", marshalErr))
		return marshalErr
	}
	if _, writeErr := cmd.OutOrStdout().Write(body); writeErr != nil {
		ui.Error(fmt.Sprintf("Failed to write SARIF: %v", writeErr))
		return writeErr
	}
	return nil
}

func validateEditorConfigForFiles(config *config.Config, selectedFiles []string, excludes []string) ([]er.ValidationErrors, error) {
	filePaths, err := files.GetFiles(*config)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrEditorConfigGetFiles, err)
	}
	filePaths, err = filterEditorConfigFiles(filePaths, selectedFiles, excludes)
	if err != nil {
		return nil, err
	}
	validationErrors := validation.ProcessValidation(filePaths, *config)
	log.Debug("Files checked", "count", len(filePaths))
	if er.GetErrorCount(validationErrors) != 0 {
		return validationErrors, errUtils.ErrEditorConfigValidationFailed
	}
	return validationErrors, nil
}

func filterEditorConfigFiles(filePaths []string, selectedFiles []string, excludes []string) ([]string, error) {
	selected := make(map[string]struct{}, len(selectedFiles))
	for _, file := range selectedFiles {
		absolute, err := filepath.Abs(filepath.FromSlash(file))
		if err == nil {
			selected[filepath.Clean(absolute)] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(filePaths))
	for _, file := range filePaths {
		remaining, err := validateReport.ExcludePaths([]string{file}, excludes)
		if err != nil {
			return nil, err
		}
		if len(remaining) == 0 {
			continue
		}
		if selectedFiles == nil {
			filtered = append(filtered, file)
			continue
		}
		absolute, err := filepath.Abs(file)
		if err == nil {
			if _, ok := selected[filepath.Clean(absolute)]; ok {
				filtered = append(filtered, file)
			}
		}
	}
	return filtered, nil
}

func editorConfigDiagnostics(validationErrors []er.ValidationErrors) validateReport.Report {
	diagnostics := make([]validateReport.Diagnostic, 0, er.GetErrorCount(validationErrors))
	for _, fileErrors := range validationErrors {
		path := fileErrors.FilePath
		if relativePath, err := files.GetRelativePath(path); err == nil {
			path = relativePath
		}
		for _, validationError := range fileErrors.Errors {
			line := validationError.LineNumber
			if line < 0 {
				line = 0
			}
			diagnostics = append(diagnostics, validateReport.Diagnostic{
				Source:   "editorconfig",
				RuleID:   "editorconfig",
				Severity: validateReport.SeverityError,
				Message:  validationError.Message.Error(),
				File:     path,
				Line:     line,
				EndLine:  line + validationError.AdditionalIdenticalErrorCount,
			})
		}
	}
	return validateReport.Report{Diagnostics: diagnostics}
}

func emitEditorConfigCI(cmd *cobra.Command, report validateReport.Report) {
	if !ci.ModeEnabled(cmd) {
		return
	}
	if ci.AnnotationsEnabled(&atmosConfig) && cliConfig.Format != outputformat.GithubActions {
		if err := editorConfigAnnotate(report.ToAnnotations()); err != nil {
			log.Debug("Failed to emit EditorConfig CI annotations", "error", err)
		}
	}
	if ci.ResultsEnabled(&atmosConfig) {
		body, err := report.SARIF()
		if err != nil {
			log.Debug("Failed to render EditorConfig SARIF for CI upload", "error", err)
			return
		}
		if err := editorConfigReportSARIF(cmd.Context(), ci.SARIFReport{Body: body, Category: "validate-editorconfig"}); err != nil {
			log.Debug("Failed to upload EditorConfig SARIF", "error", err)
		}
	}
}

func checkVersion(config config.Config) error {
	if !utils.FileExists(config.Path) || config.Version == "" {
		return nil
	}
	if config.Version != version.Version {
		return fmt.Errorf("%w: binary=%s, config=%s",
			errUtils.ErrEditorConfigVersionMismatch, version.Version, config.Version)
	}

	return nil
}

// addPersistentFlags adds flags to the root command.
func addPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&tmpExclude, "exclude", "", "Regex to exclude files from checking")
	cmd.PersistentFlags().BoolVar(&initEditorConfig, "init", false, "Create an initial configuration")

	cmd.PersistentFlags().BoolVar(&cliConfig.IgnoreDefaults, "ignore-defaults", false, "Ignore default excludes")
	cmd.PersistentFlags().BoolVar(&cliConfig.DryRun, "dry-run", false, "Show which files would be checked")
	cmd.PersistentFlags().BoolVar(&cliConfig.ShowVersion, "version", false, "Print the version number")
	cmd.PersistentFlags().StringVar(&format, "format", "default", "Specify the output format: default, gcc, codeclimate, github-actions, sarif, rich")
	cmd.PersistentFlags().BoolVar(&cliConfig.Disable.TrimTrailingWhitespace, "disable-trim-trailing-whitespace", false, "Disable trailing whitespace check")
	cmd.PersistentFlags().BoolVar(&cliConfig.Disable.EndOfLine, "disable-end-of-line", false, "Disable end-of-line check")
	cmd.PersistentFlags().BoolVar(&cliConfig.Disable.InsertFinalNewline, "disable-insert-final-newline", false, "Disable final newline check")
	cmd.PersistentFlags().BoolVar(&cliConfig.Disable.Indentation, "disable-indentation", false, "Disable indentation check")
	cmd.PersistentFlags().BoolVar(&cliConfig.Disable.IndentSize, "disable-indent-size", false, "Disable indent size check")
	cmd.PersistentFlags().BoolVar(&cliConfig.Disable.MaxLineLength, "disable-max-line-length", false, "Disable max line length check")
}

func init() {
	// Add flags
	addPersistentFlags(editorConfigCmd)
	addAffectedValidationFlags(editorConfigCmd)
	ciFlagsParser.RegisterPersistentFlags(editorConfigCmd)
	if err := ciFlagsParser.BindFlagsToViper(editorConfigCmd, viper.GetViper()); err != nil {
		panic(err)
	}
	// Add command
	validateCmd.AddCommand(editorConfigCmd)
}
