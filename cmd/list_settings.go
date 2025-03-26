package cmd

import (
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/list/errors"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	f "github.com/cloudposse/atmos/pkg/list/format"
	u "github.com/cloudposse/atmos/pkg/list/utils"
	"github.com/cloudposse/atmos/pkg/schema"
	utils "github.com/cloudposse/atmos/pkg/utils"
)

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
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		output, err := listSettings(cmd, args)
		if err != nil {
			log.Error("failed to list settings", "error", err)
			return
		}

		utils.PrintMessage(output)
	},
}

func init() {
	fl.AddCommonListFlags(listSettingsCmd)

	// Add template and function processing flags
	listSettingsCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	listSettingsCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

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

// handleSettingsError handles specific settings-related errors.
func handleSettingsError(err error, componentFilter, query string) (string, error) {
	if u.IsNoValuesFoundError(err) {
		if componentFilter != "" {
			return "", &errors.NoSettingsFoundForComponentError{
				Component: componentFilter,
				Query:     query,
			}
		}
		return "", &errors.NoSettingsFoundError{Query: query}
	}
	return "", &errors.SettingsFilteringError{Cause: err}
}

func listSettings(cmd *cobra.Command, args []string) (string, error) {
	commonFlags, err := fl.GetCommonListFlags(cmd)
	if err != nil {
		return "", &errors.CommonFlagsError{Cause: err}
	}

	processingFlags := fl.GetProcessingFlags(cmd)

	if f.Format(commonFlags.Format) == f.FormatCSV && commonFlags.Delimiter == f.DefaultTSVDelimiter {
		commonFlags.Delimiter = f.DefaultCSVDelimiter
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return "", &errors.InitConfigError{Cause: err}
	}

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false,
		processingFlags.Templates, processingFlags.Functions, false, nil)
	if err != nil {
		return "", &errors.DescribeStacksError{Cause: err}
	}

	componentFilter := ""
	if len(args) > 0 {
		componentFilter = args[0]
	}

	log.Info("Filtering settings",
		"query", commonFlags.Query, "component", componentFilter,
		"maxColumns", commonFlags.MaxColumns, "format", commonFlags.Format,
		"stackPattern", commonFlags.Stack, "processTemplates", processingFlags.Templates,
		"processYamlFunctions", processingFlags.Functions)

	filterOptions := setupSettingsOptions(*commonFlags, componentFilter)
	output, err := l.FilterAndListValues(stacksMap, filterOptions)
	if err != nil {
		return handleSettingsError(err, componentFilter, commonFlags.Query)
	}

	return output, nil
}
