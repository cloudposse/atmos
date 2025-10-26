package cmd

import (
	"fmt"

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
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration (vendor configs don't require stacks)
		checkAtmosConfig(WithStackValidation(false))

		// Get flags
		flags := cmd.Flags()

		formatFlag, err := flags.GetString("format")
		if err != nil {
			return err
		}

		stackFlag, err := flags.GetString("stack")
		if err != nil {
			return err
		}

		delimiterFlag, err := flags.GetString("delimiter")
		if err != nil {
			return err
		}

		// Initialize CLI config
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		// Vendor configs are loaded from component.yaml files, not stack manifests
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
		if err != nil {
			return err
		}

		// Set options
		options := &l.FilterOptions{
			FormatStr:    formatFlag,
			StackPattern: stackFlag,
			Delimiter:    delimiterFlag,
		}

		// Call list vendor function
		output, err := l.FilterAndListVendor(&atmosConfig, options)
		if err != nil {
			return err
		}

		// Print output
		fmt.Println(output)

		return nil
	},
}

func init() {
	AddStackCompletion(listVendorCmd)
	listCmd.AddCommand(listVendorCmd)

	// Add flags
	listVendorCmd.Flags().StringP("format", "f", "", "Output format: table, json, yaml, csv, tsv")
	listVendorCmd.Flags().StringP("delimiter", "d", "", "Delimiter for CSV/TSV output")
}
