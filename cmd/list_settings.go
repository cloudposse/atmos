package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	list "github.com/cloudposse/atmos/pkg/list"
	l "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listSettingsCmd lists settings across stacks
var listSettingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "List settings across stacks",
	Long:  "List settings configuration across all stacks",
	Example: "atmos list settings\n" +
		"atmos list settings --query .terraform\n" +
		"atmos list settings --format json\n" +
		"atmos list settings --stack '*-dev-*'\n" +
		"atmos list settings --stack 'prod-*'",
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		// Initialize logger from CLI config
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing CLI config: %v\n", err)
			return
		}

		logger, err := l.NewLoggerFromCliConfig(atmosConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
			return
		}

		flags := cmd.Flags()

		queryFlag, err := flags.GetString("query")
		if err != nil {
			logger.Error(fmt.Errorf("failed to get query flag: %v", err))
			return
		}

		maxColumnsFlag, err := flags.GetInt("max-columns")
		if err != nil {
			logger.Error(fmt.Errorf("failed to get max-columns flag: %v", err))
			return
		}

		formatFlag, err := flags.GetString("format")
		if err != nil {
			logger.Error(fmt.Errorf("failed to get format flag: %v", err))
			return
		}

		delimiterFlag, err := flags.GetString("delimiter")
		if err != nil {
			logger.Error(fmt.Errorf("failed to get delimiter flag: %v", err))
			return
		}

		stackPattern, err := flags.GetString("stack")
		if err != nil {
			logger.Error(fmt.Errorf("failed to get stack pattern flag: %v", err))
			return
		}

		// Set appropriate default delimiter based on format
		if formatFlag == list.FormatCSV && delimiterFlag == list.DefaultTSVDelimiter {
			delimiterFlag = list.DefaultCSVDelimiter
		}

		// Get all stacks
		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
		if err != nil {
			logger.Error(fmt.Errorf("failed to describe stacks: %v", err))
			return
		}

		// Use .settings as the default query if none provided
		if queryFlag == "" {
			queryFlag = ".settings"
		}

		output, err := list.FilterAndListValues(stacksMap, "", queryFlag, false, maxColumnsFlag, formatFlag, delimiterFlag, stackPattern)
		if err != nil {
			// Check if this is a 'no values found' error
			if list.IsNoValuesFoundError(err) {
				logger.Error(err)
			} else {
				logger.Warning(fmt.Sprintf("Failed to filter and list settings: %v", err))
			}
			return
		}

		logger.Info(output)
	},
}

func init() {
	// Add flags
	listSettingsCmd.PersistentFlags().String("query", "", "JMESPath query to filter settings (default: .settings)")
	listSettingsCmd.PersistentFlags().Int("max-columns", 10, "Maximum number of columns to display")
	listSettingsCmd.PersistentFlags().String("format", "", "Output format (table, json, yaml, csv, tsv)")
	listSettingsCmd.PersistentFlags().String("delimiter", "\t", "Delimiter for csv/tsv output (default: tab for tsv, comma for csv)")
	listSettingsCmd.PersistentFlags().String("stack", "", "Stack pattern to filter (supports glob patterns, e.g., '*-dev-*', 'prod-*')")

	// Add stack pattern completion
	AddStackCompletion(listSettingsCmd)

	// Add command to list command
	listCmd.AddCommand(listSettingsCmd)
}
