package exec

import (
	"github.com/spf13/cobra"
)

// ExecuteVendorPullCmd executes `vendor pull` commands
func ExecuteVendorPullCmd(cmd *cobra.Command, args []string) error {
	return ExecuteVendorCommand(cmd, args, "pull")
}

// ExecuteVendorDiffCmd executes `vendor diff` commands
func ExecuteVendorDiffCmd(cmd *cobra.Command, args []string) error {
	return ExecuteVendorCommand(cmd, args, "diff")
}
