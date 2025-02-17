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
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listValuesCmd lists component values across stacks
var listValuesCmd = &cobra.Command{
	Use:   "values [component]",
	Short: "List component values across stacks",
	Long:  "List values for a component across all stacks where it is used",
	Example: "atmos list values vpc\n" +
		"atmos list values vpc --query .vars\n" +
		"atmos list values vpc --abstract\n" +
		"atmos list values vpc --max-columns 5\n" +
		"atmos list values vpc --format json\n" +
		"atmos list values vpc --format yaml\n" +
		"atmos list values vpc --format csv\n" +
		"atmos list values vpc --stack '*-dev-*'\n" +
		"atmos list values vpc --stack 'prod-*'",
	Args: cobra.ExactArgs(1),
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

		abstractFlag, err := flags.GetBool("abstract")
		if err != nil {
			logger.Error(fmt.Errorf("failed to get abstract flag: %v", err))
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

		// Set appropriate default delimiter based on format
		if formatFlag == list.FormatCSV && delimiterFlag == list.DefaultTSVDelimiter {
			delimiterFlag = list.DefaultCSVDelimiter
		}

		component := args[0]

		// Get stack pattern
		stackPattern, err := flags.GetString("stack")
		if err != nil {
			logger.Error(fmt.Errorf("failed to get stack pattern flag: %v", err))
			return
		}

		// Get all stacks
		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
		if err != nil {
			logger.Error(fmt.Errorf("failed to describe stacks: %v", err))
			return
		}

		output, err := list.FilterAndListValues(stacksMap, component, queryFlag, abstractFlag, maxColumnsFlag, formatFlag, delimiterFlag, stackPattern)
		if err != nil {
			// Check if this is a 'no values found' error
			if list.IsNoValuesFoundError(err) {
				logger.Error(err)
			} else {
				logger.Warning(fmt.Sprintf("Failed to filter and list values: %v", err))
			}
			return
		}

		logger.Info(output)
	},
}

// listVarsCmd is an alias for 'list values --query .vars'
var listVarsCmd = &cobra.Command{
	Use:   "vars [component]",
	Short: "List component vars across stacks (alias for 'list values --query .vars')",
	Long:  "List vars for a component across all stacks where it is used",
	Example: "atmos list vars vpc\n" +
		"atmos list vars vpc --abstract\n" +
		"atmos list vars vpc --max-columns 5\n" +
		"atmos list vars vpc --format json\n" +
		"atmos list vars vpc --format yaml\n" +
		"atmos list vars vpc --format csv",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Set the query flag to .vars
		if err := cmd.Flags().Set("query", ".vars"); err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error setting query flag: %v", err), theme.Colors.Error)
			return
		}
		// Run the values command
		listValuesCmd.Run(cmd, args)
	},
}

func init() {
	// Flags for both commands
	commonFlags := func(cmd *cobra.Command) {
		cmd.PersistentFlags().String("query", "", "JMESPath query to filter values")
		cmd.PersistentFlags().Bool("abstract", false, "Include abstract components")
		cmd.PersistentFlags().Int("max-columns", 10, "Maximum number of columns to display")
		cmd.PersistentFlags().String("format", "", "Output format (table, json, yaml, csv, tsv)")
		cmd.PersistentFlags().String("delimiter", "\t", "Delimiter for csv/tsv output (default: tab for tsv, comma for csv)")
		cmd.PersistentFlags().String("stack", "", "Stack pattern to filter (supports glob patterns, e.g., '*-dev-*', 'prod-*')")
		// Add stack pattern completion
		AddStackCompletion(cmd)
	}

	commonFlags(listValuesCmd)
	commonFlags(listVarsCmd)

	listCmd.AddCommand(listValuesCmd)
	listCmd.AddCommand(listVarsCmd)
}
