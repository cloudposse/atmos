package cmd

import (
	"github.com/spf13/cobra"
)

// atlantisGenerateCmd generates various Atlantis configurations
var atlantisGenerateCmd = &cobra.Command{
	Use:                "generate",
	Short:              "Generate Atlantis configuration files",
	Long:               "Generate various configuration files required to integrate and automate Terraform workflows with Atlantis.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	atlantisCmd.AddCommand(atlantisGenerateCmd)
}
