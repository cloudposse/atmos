package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
)

// terraformParser handles flag parsing for terraform commands.
var terraformParser *flags.TerraformParser

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:     "terraform",
	Aliases: []string{"tf"},
	Short:   "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:    `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	// Allow arbitrary args so subcommands can accept positional arguments
	Args: cobra.ArbitraryArgs,
}

func init() {
	// Create parser with Terraform flags.
	// Returns strongly-typed TerraformInterpreter instead of weak map-based ParsedConfig.
	terraformParser = flags.NewTerraformParser()

	// Register flags with Cobra.
	// Cobra will now parse known Atmos flags and pass through unknown flags.
	terraformParser.RegisterFlags(terraformCmd)
	_ = terraformParser.BindToViper(viper.GetViper())

	AddStackCompletion(terraformCmd)
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}
