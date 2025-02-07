package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
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
		"atmos list values vpc --format csv",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		flags := cmd.Flags()

		queryFlag, err := flags.GetString("query")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'query' flag: %v", err), theme.Colors.Error)
			return
		}

		abstractFlag, err := flags.GetBool("abstract")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'abstract' flag: %v", err), theme.Colors.Error)
			return
		}

		maxColumnsFlag, err := flags.GetInt("max-columns")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'max-columns' flag: %v", err), theme.Colors.Error)
			return
		}

		formatFlag, err := flags.GetString("format")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'format' flag: %v", err), theme.Colors.Error)
			return
		}

		delimiterFlag, err := flags.GetString("delimiter")
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error getting the 'delimiter' flag: %v", err), theme.Colors.Error)
			return
		}

		// Set appropriate default delimiter based on format
		if formatFlag == l.FormatCSV && delimiterFlag == l.DefaultTSVDelimiter {
			delimiterFlag = l.DefaultCSVDelimiter
		}

		component := args[0]
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), theme.Colors.Error)
			return
		}

		// Get all stacks
		stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), theme.Colors.Error)
			return
		}

		output, err := l.FilterAndListValues(stacksMap, component, queryFlag, abstractFlag, maxColumnsFlag, formatFlag, delimiterFlag)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error: %v"+"\n", err), theme.Colors.Warning)
			return
		}

		u.PrintMessageInColor(output, theme.Colors.Success)
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
		cmd.PersistentFlags().String("format", "", "Output format (table, json, csv, tsv)")
		cmd.PersistentFlags().String("delimiter", "\t", "Delimiter for csv/tsv output (default: tab for tsv, comma for csv)")
	}

	commonFlags(listValuesCmd)
	commonFlags(listVarsCmd)

	listCmd.AddCommand(listValuesCmd)
	listCmd.AddCommand(listVarsCmd)
}
