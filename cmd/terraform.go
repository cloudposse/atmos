package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flagparser"
)

// terraformParser handles flag parsing for terraform commands.
var terraformParser *flagparser.PassThroughFlagParser

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:     "terraform",
	Aliases: []string{"tf"},
	Short:   "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:    `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
}

func init() {
	// Create parser with Terraform flags.
	// This replaces DisableFlagParsing and manual flag handling.
	terraformParser = flagparser.NewPassThroughFlagParser(
		flagparser.WithTerraformFlags(),
	)

	// Register flags with Cobra.
	// Cobra will now parse known Atmos flags and pass through unknown flags.
	terraformParser.RegisterFlags(terraformCmd)
	terraformParser.BindToViper(viper.GetViper())

	AddStackCompletion(terraformCmd)
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}
