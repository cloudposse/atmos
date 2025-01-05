package cmd

import (
	"github.com/spf13/cobra"
)

// atlantisCmd executes Atlantis commands
var atlantisCmd = &cobra.Command{
	Use:                "atlantis",
	Short:              "Execute 'atlantis' commands",
	Long:               `This command executes Atlantis integration commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(atlantisCmd, false)
	RootCmd.AddCommand(atlantisCmd)
}
