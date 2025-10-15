package cmd

import (
	"github.com/spf13/cobra"
)

// atlantisGenerateCmd generates various Atlantis configurations.
var atlantisGenerateCmd = &cobra.Command{
	Use:                "generate",
	Short:              "Generate Atlantis configuration files",
	Long:               "This command generates configuration files to automate and streamline Terraform workflows with Atlantis.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle "help" subcommand explicitly for parent commands
		if len(args) > 0 && args[0] == "help" {
			cmd.Help()
			return nil
		}
		// Show usage error for any other case (no subcommand or invalid subcommand)
		return showUsageAndExit(cmd, args)
	},
}

func init() {
	atlantisCmd.AddCommand(atlantisGenerateCmd)
}
