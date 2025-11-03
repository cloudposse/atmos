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
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:               `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	// DisableFlagParsing=true prevents Cobra from parsing flags, but flags can still be registered.
	// Our manual parsers extract flag values from os.Args directly.
	terraformCmd.DisableFlagParsing = true

	// Create parser with Terraform flags.
	// Returns strongly-typed TerraformInterpreter instead of weak map-based ParsedConfig.
	terraformParser = flags.NewTerraformParser()

	// Register flags as PERSISTENT on parent command so they're inherited by subcommands.
	// Even with DisableFlagParsing=true, flags can be registered for completion and help.
	terraformParser.RegisterPersistentFlags(terraformCmd)
	_ = terraformParser.BindToViper(viper.GetViper())

	AddStackCompletion(terraformCmd)
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}
