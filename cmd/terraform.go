package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_terraform_differences.md
var terraformDifferencesMarkdown string

// terraformCmd represents the base command for all terraform sub-commands.
var terraformCmd = &cobra.Command{
	Use:     "terraform",
	Aliases: []string{"tf"},
	Short:   "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long: `This command allows you to execute Terraform commands, such as **plan**, **apply**, and **destroy**,
using Atmos stack configurations for consistent infrastructure management.

Atmos enhances Terraform with additional automation, safety features, and streamlined workflows.
See the **Additions and Differences** section below for details on how Atmos extends native Terraform.

` + terraformDifferencesMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().Bool("", false, doubleDashHint)
	AddStackCompletion(terraformCmd)
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}
