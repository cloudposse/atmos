package cmd

import (
	"github.com/spf13/cobra"
)

// atlantisCmd executes Atlantis commands
var atlantisCmd = &cobra.Command{
	Use:                "atlantis",
	Short:              "Generate and manage Atlantis configurations",
	Long:               `Generate and manage Atlantis configurations that use Atmos under the hood to run Terraform workflows, bringing the power of Atmos to Atlantis for streamlined infrastructure automation.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	RootCmd.AddCommand(atlantisCmd)
}
