package cmd

import (
	"github.com/spf13/cobra"
)

// vendorCmd executes 'atmos vendor' CLI commands
var vendorCmd = &cobra.Command{
	Use:                "vendor",
	Short:              "Manage external dependencies for components or stacks",
	Long:               `This command manages external dependencies for Atmos components or stacks by vendoring them. Vendoring involves copying and locking required dependencies locally, ensuring consistency, reliability, and alignment with the principles of immutable infrastructure.`,
	Args:               cobra.NoArgs,
}

func init() {
	RootCmd.AddCommand(vendorCmd)
}
