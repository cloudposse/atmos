package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	l "github.com/cloudposse/atmos/pkg/list"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	f "github.com/cloudposse/atmos/pkg/list/format"
	listutils "github.com/cloudposse/atmos/pkg/list/utils"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	utils "github.com/cloudposse/atmos/pkg/utils"
)

var listSettingsParser = flags.NewStandardOptionsBuilder().
	WithProcessTemplates(true).
	WithProcessFunctions(true).
	Build()

// listSettingsCmd lists settings across stacks.
var listSettingsCmd = &cobra.Command{
	Use:   "settings [component]",
	Short: "List settings across stacks or for a specific component",
	Long:  "List settings configuration across all stacks or for a specific component",
	Example: "atmos list settings\n" +
		"atmos list settings c1\n" +
		"atmos list settings --query .terraform\n" +
		"atmos list settings --format json\n" +
		"atmos list settings --stack '*-dev-*'\n" +
		"atmos list settings --stack 'prod-*'",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		checkAtmosConfig()

		output, err := listSettings(cmd, args)
		if err != nil {
			return err
		}

		utils.PrintMessage(output)
		return nil
	},
}

func init() {
	fl.AddCommonListFlags(listSettingsCmd)

	// Register processing flags using builder pattern.
	listSettingsParser.RegisterFlags(listSettingsCmd)
	_ = listSettingsParser.BindToViper(viper.GetViper())

	AddStackCompletion(listSettingsCmd)
	listCmd.AddCommand(listSettingsCmd)
}

// setupSettingsOptions sets up the filter options for settings listing.
func setupSettingsOptions(commonFlags fl.CommonFlags, componentFilter string) *l.FilterOptions {
	return &l.FilterOptions{
		Component:       "settings",
		ComponentFilter: componentFilter,
		Query:           commonFlags.Query,
		IncludeAbstract: false,
		MaxColumns:      commonFlags.MaxColumns,
		FormatStr:       commonFlags.Format,
		Delimiter:       commonFlags.Delimiter,
		StackPattern:    commonFlags.Stack,
	}
}

// logNoSettingsFoundMessage logs an appropriate message when no settings are found.
func logNoSettingsFoundMessage(componentFilter string) {
	if componentFilter != "" {
		log.Info("No settings found", "component", componentFilter)
	} else {
		log.Info("No settings found")
	}
}

// SettingsParams contains all parameters needed for the list settings command.
type SettingsParams struct {
	CommonFlags     *fl.CommonFlags
	ProcessingFlags *fl.ProcessingFlags
	ComponentFilter string
}

// initSettingsParams initializes and returns the parameters needed for listing settings.
func initSettingsParams(cmd *cobra.Command, args []string) (*SettingsParams, error) {
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return nil, &listerrors.CommonFlagsError{Cause: err}
	}

	processingFlags := fl.GetProcessingFlags(cmd)

	if f.Format(commonFlags.Format) == f.FormatCSV && commonFlags.Delimiter == f.DefaultTSVDelimiter {
		commonFlags.Delimiter = f.DefaultCSVDelimiter
	}

	componentFilter := ""
	if len(args) > 0 {
		componentFilter = args[0]
	}

	return &SettingsParams{
		CommonFlags:     commonFlags,
		ProcessingFlags: processingFlags,
		ComponentFilter: componentFilter,
	}, nil
}

// getStacksMapForSettings initializes the Atmos config and returns the stacks map.
func getStacksMapForSettings(processingFlags *fl.ProcessingFlags, componentFilter string) (map[string]interface{}, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, &listerrors.InitConfigError{Cause: err}
	}

	// Check if component exists
	if componentFilter != "" {
		if !listutils.CheckComponentExists(&atmosConfig, componentFilter) {
			return nil, &listerrors.ComponentDefinitionNotFoundError{Component: componentFilter}
		}
	}

	// Execute describe stacks
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false,
		processingFlags.Templates, processingFlags.Functions, false, nil, nil)
	if err != nil {
		return nil, &listerrors.DescribeStacksError{Cause: err}
	}

	return stacksMap, nil
}

func listSettings(cmd *cobra.Command, args []string) (string, error) {
	// Initialize parameters
	params, err := initSettingsParams(cmd, args)
	if err != nil {
		return "", err
	}

	stacksMap, err := getStacksMapForSettings(params.ProcessingFlags, params.ComponentFilter)
	if err != nil {
		return "", err
	}

	log.Debug("Filtering settings",
		"query", params.CommonFlags.Query, "component", params.ComponentFilter,
		"maxColumns", params.CommonFlags.MaxColumns, "format", params.CommonFlags.Format,
		"stackPattern", params.CommonFlags.Stack, "processTemplates", params.ProcessingFlags.Templates,
		"processYamlFunctions", params.ProcessingFlags.Functions)

	filterOptions := setupSettingsOptions(*params.CommonFlags, params.ComponentFilter)
	output, err := l.FilterAndListValues(stacksMap, filterOptions)
	if err != nil {
		var noValuesErr *listerrors.NoValuesFoundError
		if errors.As(err, &noValuesErr) {
			logNoSettingsFoundMessage(params.ComponentFilter)
			return "", nil
		}
		return "", err
	}

	return output, nil
}
