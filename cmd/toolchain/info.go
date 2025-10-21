package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/toolchain"
)

var (
	outputFormat string
)

var infoCmd = &cobra.Command{
	Use:   "info <tool>",
	Short: "Show information about a tool",
	Long:  `Display detailed information about a tool from the registry.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return toolchain.InfoExec(args[0], outputFormat)
	},
}

