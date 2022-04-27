package exec

import (
	"github.com/spf13/cobra"
)

// ExecuteVendorPull executes `vendor pull` commands
func ExecuteVendorPull(cmd *cobra.Command, args []string) error {
	return ExecuteVendorCommand(cmd, args, "pull")
}

// ExecuteVendorDiff executes `vendor diff` commands
func ExecuteVendorDiff(cmd *cobra.Command, args []string) error {
	return ExecuteVendorCommand(cmd, args, "diff")
}
