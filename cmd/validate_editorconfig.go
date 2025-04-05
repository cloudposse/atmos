package cmd

import (
	"fmt"
	"os"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"

	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	er "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/error"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/files"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/outputformat"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/utils"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/validation"
	"github.com/spf13/cobra"
)

var (
	// defaultConfigFileNames determines the file names where the config is located
	defaultConfigFileNames = []string{".editorconfig", ".editorconfig-checker.json", ".ecrc"}
	initEditorConfig       bool
	currentConfig          *config.Config
	cliConfig              config.Config
	configFilePaths        []string
	tmpExclude             string
	format                 string
)

var editorConfigCmd *cobra.Command = &cobra.Command{
	Use:   "editorconfig",
	Short: "Validate all files against the EditorConfig",
	Long:  "Validate all files against the project's EditorConfig rules",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initializeConfig(cmd)
	},
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args)
		if len(args) > 0 {
			showUsageAndExit(cmd, args)
		}
		runMainLogic()
	},
}

// initializeConfig breaks the initialization cycle by separating the config setup
func initializeConfig(cmd *cobra.Command) {
	replaceAtmosConfigInConfig(cmd, atmosConfig)

	configPaths := []string{}
	if cmd.Flags().Changed("config") {
		configFiles, err := cmd.Flags().GetStringSlice("config")
		if err != nil {
			log.Fatal(err)
		}
		configFilePaths = configFiles
	}
	if len(configFilePaths) == 0 {
		configPaths = append(configPaths, defaultConfigFileNames...)
	} else {
		configPaths = append(configPaths, configFilePaths...)
	}

	currentConfig = config.NewConfig(configPaths)

	if initEditorConfig {
		err := currentConfig.Save(version.Version)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	}

	_ = currentConfig.Parse()

	if tmpExclude != "" {
		currentConfig.Exclude = append(currentConfig.Exclude, tmpExclude)
	}

	currentConfig.Merge(cliConfig)
}

func replaceAtmosConfigInConfig(cmd *cobra.Command, atmosConfig schema.AtmosConfiguration) {
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
	if !cmd.Flags().Changed("format") && atmosConfig.Validate.EditorConfig.Format != "" {
		format := outputformat.OutputFormat(atmosConfig.Validate.EditorConfig.Format)
		if ok := format.IsValid(); !ok {
			u.LogErrorAndExit(fmt.Errorf("%v is not a valid format choose from the following: %v", atmosConfig.Validate.EditorConfig.Format, outputformat.GetArgumentChoiceText()))
		}
		cliConfig.Format = format
	} else if cmd.Flags().Changed("format") {
		format := outputformat.OutputFormat(format)
		if ok := format.IsValid(); !ok {
			u.LogErrorAndExit(fmt.Errorf("%v is not a valid format choose from the following: %v", atmosConfig.Validate.EditorConfig.Format, outputformat.GetArgumentChoiceText()))
		}
		cliConfig.Format = format
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
	u.LogDebug(config.String())
	u.LogTrace(fmt.Sprintf("Exclude Regexp: %s", config.GetExcludesAsRegularExpression()))

	if err := checkVersion(config); err != nil {
		u.LogErrorAndExit(err)
	}

	filePaths, err := files.GetFiles(config)
	if err != nil {
		u.LogErrorAndExit(err)
	}

	if config.DryRun {
		for _, file := range filePaths {
			u.LogInfo(file)
		}
		os.Exit(0)
	}

	errors := validation.ProcessValidation(filePaths, config)
	u.LogDebug(fmt.Sprintf("%d files checked", len(filePaths)))
	errorCount := er.GetErrorCount(errors)
	if errorCount != 0 {
		er.PrintErrors(errors, config)
		os.Exit(1)
	}
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
	cmd.PersistentFlags().StringVar(&tmpExclude, "exclude", "", "Regex to exclude files from checking")
	cmd.PersistentFlags().BoolVar(&initEditorConfig, "init", false, "Create an initial configuration")

	cmd.PersistentFlags().BoolVar(&cliConfig.IgnoreDefaults, "ignore-defaults", false, "Ignore default excludes")
	cmd.PersistentFlags().BoolVar(&cliConfig.DryRun, "dry-run", false, "Show which files would be checked")
	cmd.PersistentFlags().BoolVar(&cliConfig.ShowVersion, "version", false, "Print the version number")
	cmd.PersistentFlags().StringVar(&format, "format", "default", "Specify the output format: default, gcc")
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
