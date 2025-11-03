package cmd

import (
	"context"
	"fmt"
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
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

var (
	// defaultConfigFileNames determines the file names where the config is located.
	defaultConfigFileNames = []string{".editorconfig", ".editorconfig-checker.json", ".ecrc"}
	currentConfig          *config.Config
	cliConfig              config.Config
	configFilePaths        []string
)

var editorConfigParser = flags.NewEditorConfigOptionsBuilder().
	WithExclude().
	WithInit().
	WithIgnoreDefaults().
	WithDryRun().
	WithShowVersion().
	WithFormat("default").
	WithDisableTrimTrailingWhitespace().
	WithDisableEndOfLine().
	WithDisableInsertFinalNewline().
	WithDisableIndentation().
	WithDisableIndentSize().
	WithDisableMaxLineLength().
	Build()

var editorConfigCmd *cobra.Command = &cobra.Command{
	Use:   "editorconfig",
	Short: "Validate all files against the EditorConfig",
	Long:  "Validate all files against the project's EditorConfig rules",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Parse flags using EditorConfigOptions.
		opts, err := editorConfigParser.Parse(context.Background(), args)
		if err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}
		initializeConfig(opts)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		if len(args) > 0 {
			showUsageAndExit(cmd, args)
		}
		runMainLogic()
		return nil
	},
}

// initializeConfig sets up the editorconfig-checker configuration using parsed options.
func initializeConfig(opts *flags.EditorConfigOptions) {
	// Use atmos.yaml config paths if not explicitly provided via flags.
	if len(atmosConfig.Validate.EditorConfig.ConfigFilePaths) > 0 {
		configFilePaths = atmosConfig.Validate.EditorConfig.ConfigFilePaths
	} else {
		configFilePaths = defaultConfigFileNames
	}

	currentConfig = config.NewConfig(configFilePaths)

	// Apply init flag.
	if opts.Init || (!opts.Init && atmosConfig.Validate.EditorConfig.Init) {
		err := currentConfig.Save(version.Version)
		if err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}
	}

	if err := currentConfig.Parse(); err != nil {
		log.Trace("Failed to parse EditorConfig configuration", "error", err, "paths", configFilePaths)
	}

	// Apply exclude from flags or atmos.yaml.
	if opts.Exclude != "" {
		currentConfig.Exclude = append(currentConfig.Exclude, opts.Exclude)
	} else if len(atmosConfig.Validate.EditorConfig.Exclude) > 0 {
		excludeStr := strings.Join(atmosConfig.Validate.EditorConfig.Exclude, ",")
		currentConfig.Exclude = append(currentConfig.Exclude, excludeStr)
	}

	// Build cliConfig from opts and atmos.yaml precedence.
	cliConfig.IgnoreDefaults = opts.IgnoreDefaults || atmosConfig.Validate.EditorConfig.IgnoreDefaults
	cliConfig.DryRun = opts.DryRun || atmosConfig.Validate.EditorConfig.DryRun
	cliConfig.Verbose = opts.LogsLevel == u.LogLevelTrace

	// Handle format flag with validation.
	formatStr := opts.Format
	if formatStr == "default" && atmosConfig.Validate.EditorConfig.Format != "" {
		formatStr = atmosConfig.Validate.EditorConfig.Format
	}
	format := outputformat.OutputFormat(formatStr)
	if ok := format.IsValid(); !ok {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("%v is not a valid format choose from the following: %v", formatStr, outputformat.GetArgumentChoiceText()), "", "")
	}
	cliConfig.Format = format

	// Apply NoColor from GlobalFlags.
	cliConfig.NoColor = opts.NoColor || atmosConfig.Settings.Terminal.NoColor

	// Apply disable flags.
	cliConfig.Disable.TrimTrailingWhitespace = opts.DisableTrimTrailingWhitespace || atmosConfig.Validate.EditorConfig.DisableTrimTrailingWhitespace
	cliConfig.Disable.EndOfLine = opts.DisableEndOfLine || atmosConfig.Validate.EditorConfig.DisableEndOfLine
	cliConfig.Disable.InsertFinalNewline = opts.DisableInsertFinalNewline || atmosConfig.Validate.EditorConfig.DisableInsertFinalNewline
	cliConfig.Disable.Indentation = opts.DisableIndentation || atmosConfig.Validate.EditorConfig.DisableIndentation
	cliConfig.Disable.IndentSize = opts.DisableIndentSize || atmosConfig.Validate.EditorConfig.DisableIndentSize
	cliConfig.Disable.MaxLineLength = opts.DisableMaxLineLength || atmosConfig.Validate.EditorConfig.DisableMaxLineLength

	currentConfig.Merge(cliConfig)
}

// runMainLogic contains the main logic.
func runMainLogic() {
	config := *currentConfig
	log.Debug(config.String())
	log.Debug("Excluding", "regex", config.GetExcludesAsRegularExpression())

	if err := checkVersion(config); err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	filePaths, err := files.GetFiles(config)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	if config.DryRun {
		for _, file := range filePaths {
			log.Info(file)
		}
		return
	}

	errors := validation.ProcessValidation(filePaths, config)
	log.Debug("Files checked", "count", len(filePaths))
	errorCount := er.GetErrorCount(errors)
	if errorCount != 0 {
		er.PrintErrors(errors, config)
		errUtils.Exit(1)
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

func init() {
	// Register flags using the parser.
	editorConfigParser.RegisterFlags(editorConfigCmd)
	_ = editorConfigParser.BindToViper(viper.GetViper())

	// Add command to parent.
	validateCmd.AddCommand(editorConfigCmd)
}
