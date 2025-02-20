package cmd

import (
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	list "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listMetadataCmd lists metadata across stacks
var listMetadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "List metadata across stacks",
	Long:  "List metadata information across all stacks",
	Example: "atmos list metadata\n" +
		"atmos list metadata --query .component\n" +
		"atmos list metadata --format json\n" +
		"atmos list metadata --stack '*-dev-*'\n" +
		"atmos list metadata --stack 'prod-*'",
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			log.Error("failed to initialize CLI config", "error", err)
			return
		}

		flags := cmd.Flags()

		queryFlag, err := flags.GetString("query")
		if err != nil {
			log.Error("failed to get query flag", "error", err)
			return
		}

		maxColumnsFlag, err := flags.GetInt("max-columns")
		if err != nil {
			log.Error("failed to get max-columns flag", "error", err)
			return
		}

		formatFlag, err := flags.GetString("format")
		if err != nil {
			log.Error("failed to get format flag", "error", err)
			return
		}

		delimiterFlag, err := flags.GetString("delimiter")
		if err != nil {
			log.Error("failed to get delimiter flag", "error", err)
			return
		}

		stackPattern, err := flags.GetString("stack")
		if err != nil {
			log.Error("failed to get stack pattern flag", "error", err)
			return
		}

		// Set appropriate default delimiter based on format
		if formatFlag == list.FormatCSV && delimiterFlag == list.DefaultTSVDelimiter {
			delimiterFlag = list.DefaultCSVDelimiter
		}

		// Get all stacks
		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
		if err != nil {
			log.Error("failed to describe stacks", "error", err)
			return
		}

		// Use .metadata as the default query if none provided
		if queryFlag == "" {
			queryFlag = ".metadata"
		}

		output, err := list.FilterAndListValues(stacksMap, "", queryFlag, false, maxColumnsFlag, formatFlag, delimiterFlag, stackPattern)
		if err != nil {
			// Check if this is a 'no values found' error
			if list.IsNoValuesFoundError(err) {
				log.Error("no values found", "error", err)
			} else {
				log.Warn("failed to filter and list metadata", "error", err)
			}
			return
		}

		log.Info(output)
	},
}

func init() {
	// Add flags
	listMetadataCmd.PersistentFlags().String("query", "", "JMESPath query to filter metadata (default: .metadata)")
	listMetadataCmd.PersistentFlags().Int("max-columns", 10, "Maximum number of columns to display")
	listMetadataCmd.PersistentFlags().String("format", "", "Output format (table, json, yaml, csv, tsv)")
	listMetadataCmd.PersistentFlags().String("delimiter", "\t", "Delimiter for csv/tsv output (default: tab for tsv, comma for csv)")
	listMetadataCmd.PersistentFlags().String("stack", "", "Stack pattern to filter (supports glob patterns, e.g., '*-dev-*', 'prod-*')")

	// Add stack pattern completion
	AddStackCompletion(listMetadataCmd)

	// Add command to list command
	listCmd.AddCommand(listMetadataCmd)
}
