package toolchain

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/toolchain"
)

var (
	exportFlag   bool
	jsonFlag     bool
	relativeFlag bool
)

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print PATH entries for installed tools",
	Long:  `Print PATH entries for all tools configured in .tool-versions.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return toolchain.EmitPath(exportFlag, jsonFlag, relativeFlag)
	},
}
