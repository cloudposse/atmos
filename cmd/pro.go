package cmd

import (
	"github.com/spf13/cobra"
)

// proCmd executes 'atmos pro' CLI commands
var proCmd = &cobra.Command{
	Use:                "pro",
	Short:              "Access premium features integrated with app.cloudposse.com",
	Long:               `This command allows you to manage and configure premium features available through app.cloudposse.com.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(proCmd, false)
	RootCmd.AddCommand(proCmd)
}
