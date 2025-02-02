package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listVendorCmd lists atmos vendor configurations
var listVendorCmd = &cobra.Command{
	Use:   "vendor",
	Short: "List all vendor configurations",
	Long:  "List vendor configurations in a tabular way, including component and vendor manifests",
	Example: "atmos list vendor\n" +
		"atmos list vendor --format json\n" +
		"atmos list vendor --format csv     # Uses comma (,) as delimiter\n" +
		"atmos list vendor --format tsv     # Uses tab (\\t) as delimiter\n" +
		"atmos list vendor --format csv --delimiter ';'    # Custom delimiter",
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()

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

		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), theme.Colors.Error)
			return
		}

		output, err := l.FilterAndListVendors(atmosConfig.Vendor.List, formatFlag, delimiterFlag)
		if err != nil {
			u.PrintMessageInColor(fmt.Sprintf("Error: %v"+"\n", err), theme.Colors.Warning)
			return
		}

		u.PrintMessageInColor(output, theme.Colors.Success)
	},
}

func init() {
	listVendorCmd.PersistentFlags().String("format", "", "Output format (table, json, csv, tsv)")
	listVendorCmd.PersistentFlags().String("delimiter", "\t", "Delimiter for csv/tsv output (default: tab for tsv, comma for csv)")
	listCmd.AddCommand(listVendorCmd)
}
