package cmd

import (
	"github.com/spf13/cobra"
)

// atlantisCmd executes Atlantis commands.
var atlantisCmd = &cobra.Command{
	Use:                "atlantis",
	Short:              "Generate and manage Atlantis configurations",
	Long:               `Generate and manage Atlantis configurations that use Atmos under the hood to run Terraform workflows, bringing the power of Atmos to Atlantis for streamlined infrastructure automation.`,
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
	atlantisCmd.PersistentFlags().Bool("", false, doubleDashHint)
	RootCmd.AddCommand(atlantisCmd)
}
