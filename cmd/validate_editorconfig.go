package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"

	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	er "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/error"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/files"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/utils"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/validation"
	"github.com/spf13/cobra"
)

var (
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
		initializeConfig(cmd)
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
func initializeConfig(cmd *cobra.Command) {
	replaceAtmosConfigInConfig(cmd, atmosConfig)

	if configFilePath == "" {
		configFilePath = defaultConfigFilePath
	}

	var err error
	currentConfig, err = config.NewConfig(configFilePath)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	if initEditorConfig {
		err := currentConfig.Save(version.Version)
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

func replaceAtmosConfigInConfig(cmd *cobra.Command, atmosConfig schema.AtmosConfiguration) {
	if !cmd.Flags().Changed("config") && atmosConfig.Validate.EditorConfig.ConfigFilePath != "" {
		configFilePath = atmosConfig.Validate.EditorConfig.ConfigFilePath
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
	if !cmd.Flags().Changed("format") && atmosConfig.Validate.EditorConfig.Format != "" {
		cliConfig.Format = atmosConfig.Validate.EditorConfig.Format
	}
	if !cmd.Flags().Changed("logs-level") && atmosConfig.Logs.Level == "trace" {
		cliConfig.Verbose = true
	} else if cmd.Flags().Changed("logs-level") {
		if v, err := cmd.Flags().GetString("logs-level"); err == nil && v == "trace" {
			cliConfig.Verbose = true
		}
	}
	if !cmd.Flags().Changed("no-color") && !atmosConfig.Validate.EditorConfig.Color {
		cliConfig.NoColor = !atmosConfig.Validate.EditorConfig.Color
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
}

// runMainLogic contains the main logic
func runMainLogic() {
	config := *currentConfig
	u.LogDebug(atmosConfig, config.GetAsString())
	u.LogTrace(atmosConfig, fmt.Sprintf("Exclude Regexp: %s", config.GetExcludesAsRegularExpression()))

	if err := checkVersion(config); err != nil {
		u.LogErrorAndExit(atmosConfig, err)
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
	u.PrintMessage("No errors found")
}

func checkVersion(config config.Config) error {
	if !utils.FileExists(config.Path) || config.Version == "" {
		return nil
	}
	if config.Version != version.Version {
		return fmt.Errorf("version mismatch: binary=%s, config=%s",
			version.Version, config.Version)
	}

	return nil
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