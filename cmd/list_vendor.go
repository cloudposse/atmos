package cmd

import (
	"fmt"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listVendorCmd lists vendor configurations.
var listVendorCmd = &cobra.Command{
	Use:   "vendor",
	Short: "List all vendor configurations",
	Long:  "List all vendor configurations in a tabular way, including component and vendor manifests.",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		// Get flags
		flags := cmd.Flags()

		formatFlag, err := flags.GetString("format")
		if err != nil {
			log.Error("Error getting the 'format' flag", "error", err)
			cmd.PrintErrln(fmt.Errorf("error getting the 'format' flag: %w", err))
			cmd.PrintErrln("Run 'atmos list vendor --help' for usage")
			return
		}

		stackFlag, err := flags.GetString("stack")
		if err != nil {
			log.Error("Error getting the 'stack' flag", "error", err)
			cmd.PrintErrln(fmt.Errorf("error getting the 'stack' flag: %w", err))
			cmd.PrintErrln("Run 'atmos list vendor --help' for usage")
			return
		}

		delimiterFlag, err := flags.GetString("delimiter")
		if err != nil {
			log.Error("Error getting the 'delimiter' flag", "error", err)
			cmd.PrintErrln(fmt.Errorf("error getting the 'delimiter' flag: %w", err))
			cmd.PrintErrln("Run 'atmos list vendor --help' for usage")
			return
		}

		// Initialize CLI config
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			log.Error("Error initializing CLI config", "error", err)
			cmd.PrintErrln(fmt.Errorf("error initializing CLI config: %w", err))
			return
		}

		// Set options
		options := &l.FilterOptions{
			FormatStr:    formatFlag,
			StackPattern: stackFlag,
			Delimiter:    delimiterFlag,
		}

		// Call list vendor function
		output, err := l.FilterAndListVendor(atmosConfig, options)
		if err != nil {
			log.Error("Error listing vendor configurations", "error", err)
			cmd.PrintErrln(fmt.Errorf("error listing vendor configurations: %w", err))
			return
		}

		// Print output
		fmt.Println(output)
	},
}

func init() {
	AddStackCompletion(listVendorCmd)
	listCmd.AddCommand(listVendorCmd)

	// Add flags
	listVendorCmd.Flags().StringP("format", "f", "", "Output format: table, json, yaml, csv, tsv")
	listVendorCmd.Flags().StringP("delimiter", "d", "", "Delimiter for CSV/TSV output")
}
