package cmd

import (
	"github.com/spf13/cobra"
)

// atlantisGenerateCmd generates various Atlantis configurations
var atlantisGenerateCmd = &cobra.Command{
	Use:                "generate",
	Short:              "Execute 'atlantis generate' commands",
	Long:               "This command generates various Atlantis configurations",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(atlantisGenerateCmd, false)
	atlantisCmd.AddCommand(atlantisGenerateCmd)
}
