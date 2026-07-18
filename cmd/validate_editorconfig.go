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
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	u "github.com/cloudposse/atmos/pkg/utils"
	validateReport "github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/version"
)

var (
	// defaultConfigFileNames determines the file names where the config is located
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
	if err := replaceAtmosConfigInConfig(cmd, atmosConfig); err != nil {
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

func replaceAtmosConfigInConfig(cmd *cobra.Command, atmosConfig schema.AtmosConfiguration) error {
	if !cmd.Flags().Changed("config") && len(atmosConfig.Validate.EditorConfig.ConfigFilePaths) > 0 {
		configFilePaths = atmosConfig.Validate.EditorConfig.ConfigFilePaths
	}
	if !cmd.Flags().Changed("exclude") && len(atmosConfig.Validate.EditorConfig.Exclude) > 0 {
		tmpExclude = strings.Join(atmosConfig.Validate.EditorConfig.Exclude, ",")
	}
	if !cmd.Flags().Changed("init") && atmosConfig.Validate.EditorConfig.Init {
		initEditorConfig = atmosConfig.Validate.EditorConfig.Init
	}
	if !cmd.Flags().Changed("ignore-defaults") && atmosConfig.Validate.EditorConfig.IgnoreDefaults {
		cliConfig.IgnoreDefaults = atmosConfig.Validate.EditorConfig.IgnoreDefaults
	}
	if !cmd.Flags().Changed("dry-run") && atmosConfig.Validate.EditorConfig.DryRun {
		cliConfig.DryRun = atmosConfig.Validate.EditorConfig.DryRun
	}
	requestedFormat := requestedEditorConfigFormat(cmd, atmosConfig)
	if requestedFormat != "" {
		isSARIF, err := configureEditorConfigFormat(requestedFormat)
		if err != nil {
			return err
		}
		editorConfigSARIF = isSARIF
	}
	// Set verbose mode if log level is Trace
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
	if !cmd.Flags().Changed("no-color") && atmosConfig.Settings.Terminal.NoColor {
		cliConfig.NoColor = atmosConfig.Settings.Terminal.NoColor
	} else if cmd.Flags().Changed("no-color") {
		cliConfig.NoColor, _ = cmd.Flags().GetBool("no-color")
	}
	if !cmd.Flags().Changed("disable-trim-trailing-whitespace") && atmosConfig.Validate.EditorConfig.DisableTrimTrailingWhitespace {
		cliConfig.Disable.TrimTrailingWhitespace = atmosConfig.Validate.EditorConfig.DisableTrimTrailingWhitespace
	}
	if !cmd.Flags().Changed("disable-end-of-line") && atmosConfig.Validate.EditorConfig.DisableEndOfLine {
		cliConfig.Disable.EndOfLine = atmosConfig.Validate.EditorConfig.DisableEndOfLine
	}
	if !cmd.Flags().Changed("disable-insert-final-newline") && atmosConfig.Validate.EditorConfig.DisableInsertFinalNewline {
		cliConfig.Disable.InsertFinalNewline = atmosConfig.Validate.EditorConfig.DisableInsertFinalNewline
	}
	if !cmd.Flags().Changed("disable-indentation") && atmosConfig.Validate.EditorConfig.DisableIndentation {
		cliConfig.Disable.Indentation = atmosConfig.Validate.EditorConfig.DisableIndentation
	}
	if !cmd.Flags().Changed("disable-indent-size") && atmosConfig.Validate.EditorConfig.DisableIndentSize {
		cliConfig.Disable.IndentSize = atmosConfig.Validate.EditorConfig.DisableIndentSize
	}
	if !cmd.Flags().Changed("disable-max-line-length") && atmosConfig.Validate.EditorConfig.DisableMaxLineLength {
		cliConfig.Disable.MaxLineLength = atmosConfig.Validate.EditorConfig.DisableMaxLineLength
	}

	return nil
}

func requestedEditorConfigFormat(cmd *cobra.Command, atmosConfig schema.AtmosConfiguration) string {
	if aggregateValidationFormat != "" {
		return aggregateValidationFormat
	}
	if cmd.Flags().Changed("format") {
		return format
	}
	if value := os.Getenv("ATMOS_VALIDATE_FORMAT"); value != "" {
		return value
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
		return false, fmt.Errorf("%v is not a valid format choose from the following: %v, sarif, rich", value, outputformat.GetArgumentChoiceText())
	}
	cliConfig.Format = upstreamFormat
	return false, nil
}

// runEditorConfig executes EditorConfig validation without terminating the process.
// It can therefore be composed by aggregate validators.
func runEditorConfig(cmd *cobra.Command) error {
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
	return runEditorConfigForFiles(cmd, selectedFiles, nil)
}

// runEditorConfigForFiles validates a subset of files when selectedFiles is
// non-nil. A nil list retains the established whole-project behavior.
//
//nolint:cyclop,funlen,gocognit,revive // This preserves the established format and spinner branches while adding file selection.
func runEditorConfigForFiles(cmd *cobra.Command, selectedFiles []string, excludes []string) error {
	if err := initializeConfig(cmd); err != nil {
		return err
	}

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
		validationErrors, err := validateEditorConfigForFiles(&config, selectedFiles, excludes)
		if err != nil && len(validationErrors) == 0 {
			ui.Error(fmt.Sprintf("Validation failed: %v", err))
			return err
		}
		report := editorConfigDiagnostics(validationErrors)
		emitEditorConfigCI(cmd, report)
		if editorConfigRich {
			root, rootErr := os.Getwd()
			if rootErr != nil {
				return rootErr
			}
			output := validateReport.Rich(report, validateReport.DefaultRichOptions(root))
			if output != "" {
				if _, writeErr := fmt.Fprintln(cmd.OutOrStdout(), output); writeErr != nil {
					return writeErr
				}
			} else if err == nil {
				if _, writeErr := fmt.Fprintln(cmd.OutOrStdout(), "✓ EditorConfig validation passed"); writeErr != nil {
					return writeErr
				}
			}
		} else {
			body, marshalErr := report.SARIF()
			if marshalErr != nil {
				ui.Error(fmt.Sprintf("Failed to render SARIF: %v", marshalErr))
				return marshalErr
			}
			if _, writeErr := cmd.OutOrStdout().Write(body); writeErr != nil {
				ui.Error(fmt.Sprintf("Failed to write SARIF: %v", writeErr))
				return writeErr
			}
		}
		if editorConfigRich && err != nil {
			return errUtils.ExitCodeError{Code: 1, Silent: true}
		}
		return err
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

func validateEditorConfig(config config.Config) ([]er.ValidationErrors, error) {
	return validateEditorConfigForFiles(&config, nil, nil)
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
	cmd.PersistentFlags().Bool("ci", false, "Enable CI mode for automated pipelines (annotations, SARIF upload)")
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
	if err := viper.BindPFlag("ci", editorConfigCmd.PersistentFlags().Lookup("ci")); err != nil {
		panic(err)
	}
	if err := viper.BindEnv("ci", "ATMOS_CI", "CI"); err != nil {
		panic(err)
	}
	// Add command
	validateCmd.AddCommand(editorConfigCmd)
}
