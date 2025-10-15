package cmd

import (
	"github.com/spf13/cobra"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:               `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle "help" subcommand explicitly
		if len(args) > 0 && args[0] == "help" {
			cmd.Help()
			return nil
		}
		// Show usage error for invalid invocation
		return showUsageAndExit(cmd, args)
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().Bool("", false, doubleDashHint)
	AddStackCompletion(terraformCmd)
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}
