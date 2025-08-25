package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

// toolchainAliasesCmd defines the Cobra command for listing aliases
var toolchainAliasesCmd = &cobra.Command{
	Use:   "aliases",
	Short: "List configured tool aliases",
	Long: `List all configured tool aliases from the local tools.yaml configuration.

Aliases allow you to use short tool names (like 'terraform') instead of
full owner/repo paths (like 'hashicorp/terraform') in commands.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return toolchain.ListAliases()
	},
}
