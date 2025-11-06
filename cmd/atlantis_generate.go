package cmd

import (
	"github.com/spf13/cobra"
)

// atlantisGenerateCmd generates various Atlantis configurations.
var atlantisGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Atlantis configuration files",
	Long:  "This command generates configuration files to automate and streamline Terraform workflows with Atlantis.",
	Args:  cobra.NoArgs,
}

func init() {
	atlantisCmd.AddCommand(atlantisGenerateCmd)
}
