package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags/terraform"
)

// terraformParser handles flag parsing for terraform commands.
var terraformParser *terraform.Parser

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:               `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

func init() {
	// Create parser with Terraform flags.
	// Returns strongly-typed Options instead of weak map-based ParsedConfig.
	terraformParser = terraform.NewParser()

	// Register flags as PERSISTENT on parent command so they're inherited by subcommands.
	// RegisterPersistentFlags automatically sets DisableFlagParsing=true for manual parsing.
	terraformParser.RegisterPersistentFlags(terraformCmd)
	_ = terraformParser.BindToViper(viper.GetViper())

	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)

	// Add completions AFTER adding to RootCmd so inherited flags are available.
	AddStackCompletion(terraformCmd)
	AddIdentityCompletion(terraformCmd)
}
