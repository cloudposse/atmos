package cmd

import (
	"github.com/spf13/cobra"
)

// atlantisCmd executes Atlantis commands
var atlantisCmd = &cobra.Command{
	Use:                "atlantis",
	Short:              "Generate and manage Atlantis configurations",
	Long:               `This command enables integration with Atlantis, allowing users to generate configurations for Terraform workflows.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	RootCmd.AddCommand(atlantisCmd)
}
