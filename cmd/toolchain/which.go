package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/toolchain"
)

var whichCmd = &cobra.Command{
	Use:   "which <tool>",
	Short: "Show path to installed tool binary",
	Long:  `Show the full path to an installed tool binary.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return toolchain.WhichExec(args[0])
	},
}
