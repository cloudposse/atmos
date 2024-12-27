package cmd

import (
	"fmt"
	"os"

	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	er "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/error"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/files"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/logger"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/utils"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/validation"
	"github.com/spf13/cobra"
)

var (
	editorConfigVersion   = "v3.0.0"
	defaultConfigFilePath = ".ecrc"
	currentConfig         *config.Config
	cliConfig             config.Config
	configFilePath        string
	tmpExclude            string
)

var editorConfigCmd *cobra.Command = &cobra.Command{
	Use:   "editorconfig-checker",
	Short: "EditorConfig Checker CLI",
	Long:  "A command-line tool to check your files against the EditorConfig rules",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initializeConfig()
	},
	Run: func(cmd *cobra.Command, args []string) {
		runMainLogic()
	},
}

// initializeConfig breaks the initialization cycle by separating the config setup
func initializeConfig() {
	if configFilePath == "" {
		configFilePath = defaultConfigFilePath
	}

	var err error
	currentConfig, err = config.NewConfig(configFilePath)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	_ = currentConfig.Parse()

	if tmpExclude != "" {
		currentConfig.Exclude = append(currentConfig.Exclude, tmpExclude)
	}

	currentConfig.Merge(cliConfig)
}

// runMainLogic contains the main logic
func runMainLogic() {
	config := *currentConfig
	config.Logger.Debug(config.GetAsString())
	config.Logger.Verbose("Exclude Regexp: %s", config.GetExcludesAsRegularExpression())

	if utils.FileExists(config.Path) && config.Version != "" && config.Version != editorConfigVersion {
		config.Logger.Error("Version from config file is not the same as the version of the binary")
		config.Logger.Error("Binary: %s, Config: %s", editorConfigVersion, config.Version)
		os.Exit(1)
	}

	if handleReturnableFlags(config) {
		return
	}

	filePaths, err := files.GetFiles(config)
	if err != nil {
		config.Logger.Error(err.Error())
		os.Exit(1)
	}

	if config.DryRun {
		for _, file := range filePaths {
			config.Logger.Output(file)
		}
		os.Exit(0)
	}

	errors := validation.ProcessValidation(filePaths, config)
	errorCount := er.GetErrorCount(errors)

	if errorCount != 0 {
		er.PrintErrors(errors, config)
		config.Logger.Error(fmt.Sprintf("\n%d errors found", errorCount))
		os.Exit(1)
	}

	config.Logger.Verbose("%d files checked", len(filePaths))
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
	cmd.PersistentFlags().BoolVar(&cliConfig.IgnoreDefaults, "ignore-defaults", false, "Ignore default excludes")
	cmd.PersistentFlags().BoolVar(&cliConfig.DryRun, "dry-run", false, "Show which files would be checked")
	cmd.PersistentFlags().BoolVar(&cliConfig.ShowVersion, "version", false, "Print the version number")
	cmd.PersistentFlags().BoolVar(&cliConfig.Help, "help", false, "Print help information")
	cmd.PersistentFlags().StringVar(&cliConfig.Format, "format", "default", "Specify the output format: default, gcc")
	cmd.PersistentFlags().BoolVar(&cliConfig.Verbose, "verbose", false, "Print debugging information")
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
