package cmd

import (
	"fmt"

	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainInfoCmd = &cobra.Command{
	Use:   "info <tool>",
	Short: "Display tool configuration from registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse and validate output format flag
		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat != "table" && outputFormat != "yaml" {
			return fmt.Errorf("invalid output format: %s. Must be 'table' or 'yaml'", outputFormat)
		}

		// Extract tool name from arguments
		toolName := args[0]

		// Call business logic
		return toolchain.InfoExec(toolName, outputFormat)
	},
}

func init() {
	toolchainInfoCmd.Flags().StringP("output", "o", "table", "Output format (table or yaml)")
}
