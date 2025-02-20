package exec

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ExecuteVendorPullCmd executes `vendor pull` commands.
func ExecuteVendorPullCmd(cmd *cobra.Command, args []string) error {
	return ExecuteVendorPullCommand(cmd, args)
}

// ExecuteVendorDiffCmd executes `vendor diff` commands.
func ExecuteVendorDiffCmd(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("'atmos vendor diff' is not implemented yet")
}
