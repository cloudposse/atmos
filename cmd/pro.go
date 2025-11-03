package cmd

import (
	"github.com/spf13/cobra"
)

// proCmd executes 'atmos pro' CLI commands
var proCmd = &cobra.Command{
	Use:                "pro",
	Short:              "Access premium features integrated with atmos-pro.com",
	Long:               `This command allows you to manage and configure premium features available through atmos-pro.com.`,
	Args:               cobra.NoArgs,
}

func init() {
	RootCmd.AddCommand(proCmd)
}
