package cmd

import (
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"

	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	er "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/error"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/files"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/utils"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/validation"
	"github.com/spf13/cobra"
)

var (
	editorConfigVersion   = "v3.0.3"
	defaultConfigFilePath = ".editorconfig"
	initEditorConfig      bool
	currentConfig         *config.Config
	cliConfig             config.Config
	configFilePath        string
	tmpExclude            string
)

var editorConfigCmd *cobra.Command = &cobra.Command{
	Use:   "editorconfig",
	Short: "Validate all files against the EditorConfig",
	Long:  "Validate all files against the project's EditorConfig rules",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initializeConfig()
	},
	Run: func(cmd *cobra.Command, args []string) {
		if cliConfig.Help {
			cmd.Help()
			os.Exit(0)
		}
		runMainLogic()
	},
}

// initializeConfig breaks the initialization cycle by separating the config setup
func initializeConfig() {
	replaceAtmosConfigInConfig(atmosConfig)

	u.LogInfo(atmosConfig, fmt.Sprintf("EditorConfig Checker CLI Version: %s", editorConfigVersion))
	if configFilePath == "" {
		configFilePath = defaultConfigFilePath
	}

	var err error
	currentConfig, err = config.NewConfig(configFilePath)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	if initEditorConfig {
		err := currentConfig.Save(editorConfigVersion)
		if err != nil {
			u.LogErrorAndExit(atmosConfig, err)
		}
	}

	_ = currentConfig.Parse()

	if tmpExclude != "" {
		currentConfig.Exclude = append(currentConfig.Exclude, tmpExclude)
	}

	currentConfig.Merge(cliConfig)
}

func replaceAtmosConfigInConfig(atmosConfig schema.AtmosConfiguration) {
	if atmosConfig.Validate.EditorConfig.ConfigFilePath != "" {
		configFilePath = atmosConfig.Validate.EditorConfig.ConfigFilePath
	}
	if atmosConfig.Validate.EditorConfig.Exclude != "" {
		tmpExclude = atmosConfig.Validate.EditorConfig.Exclude
	}
	if atmosConfig.Validate.EditorConfig.Init {
		initEditorConfig = atmosConfig.Validate.EditorConfig.Init
	}
	if atmosConfig.Validate.EditorConfig.IgnoreDefaults {
		cliConfig.IgnoreDefaults = atmosConfig.Validate.EditorConfig.IgnoreDefaults
	}
	if atmosConfig.Validate.EditorConfig.DryRun {
		cliConfig.DryRun = atmosConfig.Validate.EditorConfig.DryRun
	}
	if atmosConfig.Validate.EditorConfig.Format != "" {
		cliConfig.Format = atmosConfig.Validate.EditorConfig.Format
	}
	if atmosConfig.Logs.Level == "trace" {
		cliConfig.Verbose = true
	}
	if atmosConfig.Validate.EditorConfig.NoColor {
		cliConfig.NoColor = atmosConfig.Validate.EditorConfig.NoColor
	}
	if atmosConfig.Validate.EditorConfig.Disable.TrimTrailingWhitespace {
		cliConfig.Disable.TrimTrailingWhitespace = atmosConfig.Validate.EditorConfig.Disable.TrimTrailingWhitespace
	}
	if atmosConfig.Validate.EditorConfig.Disable.EndOfLine {
		cliConfig.Disable.EndOfLine = atmosConfig.Validate.EditorConfig.Disable.EndOfLine
	}
	if atmosConfig.Validate.EditorConfig.Disable.InsertFinalNewline {
		cliConfig.Disable.InsertFinalNewline = atmosConfig.Validate.EditorConfig.Disable.InsertFinalNewline
	}
	if atmosConfig.Validate.EditorConfig.Disable.Indentation {
		cliConfig.Disable.Indentation = atmosConfig.Validate.EditorConfig.Disable.Indentation
	}
	if atmosConfig.Validate.EditorConfig.Disable.IndentSize {
		cliConfig.Disable.IndentSize = atmosConfig.Validate.EditorConfig.Disable.IndentSize
	}
	if atmosConfig.Validate.EditorConfig.Disable.MaxLineLength {
		cliConfig.Disable.MaxLineLength = atmosConfig.Validate.EditorConfig.Disable.MaxLineLength
	}
}

// runMainLogic contains the main logic
func runMainLogic() {
	config := *currentConfig
	u.LogDebug(atmosConfig, config.GetAsString())
	u.LogTrace(atmosConfig, fmt.Sprintf("Exclude Regexp: %s", config.GetExcludesAsRegularExpression()))

	if err := checkVersion(config); err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	if handleReturnableFlags(config) {
		return
	}

	filePaths, err := files.GetFiles(config)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	if config.DryRun {
		for _, file := range filePaths {
			u.LogInfo(atmosConfig, file)
		}
		os.Exit(0)
	}

	errors := validation.ProcessValidation(filePaths, config)
	errorCount := er.GetErrorCount(errors)

	if errorCount != 0 {
		er.PrintErrors(errors, config)
		u.LogErrorAndExit(atmosConfig, fmt.Errorf("\n%d errors found", errorCount))
	}

	u.LogDebug(atmosConfig, fmt.Sprintf("%d files checked", len(filePaths)))
	u.LogInfo(atmosConfig, "No errors found")
}

func checkVersion(config config.Config) error {
	if !utils.FileExists(config.Path) || config.Version == "" {
		return nil
	}
	if config.Version != editorConfigVersion {
		return fmt.Errorf("version mismatch: binary=%s, config=%s",
			editorConfigVersion, config.Version)
	}

	return nil
}

// handleReturnableFlags handles early termination flags
func handleReturnableFlags(config config.Config) bool {
	if config.ShowVersion {
		config.Logger.Output(editorConfigVersion)
		return true
	}
	if config.Help {
		config.Logger.Output("USAGE:")
		return true
	}
	return false
}

// addPersistentFlags adds flags to the root command
func addPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&configFilePath, "config", "", "Path to the configuration file")
	cmd.PersistentFlags().StringVar(&tmpExclude, "exclude", "", "Regex to exclude files from checking")
	cmd.PersistentFlags().BoolVar(&initEditorConfig, "init", false, "creates an initial configuration")

	cmd.PersistentFlags().BoolVar(&cliConfig.IgnoreDefaults, "ignore-defaults", false, "Ignore default excludes")
	cmd.PersistentFlags().BoolVar(&cliConfig.DryRun, "dry-run", false, "Show which files would be checked")
	cmd.PersistentFlags().BoolVar(&cliConfig.ShowVersion, "version", false, "Print the version number")
	cmd.PersistentFlags().StringVar(&cliConfig.Format, "format", "default", "Specify the output format: default, gcc")
	cmd.PersistentFlags().BoolVar(&cliConfig.NoColor, "no-color", false, "Don't print colors")
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
	// Add command
	validateCmd.AddCommand(editorConfigCmd)
}
