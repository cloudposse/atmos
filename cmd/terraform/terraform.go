package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/terraform/generate"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// terraformParser handles flag parsing for shared terraform flags.
// These persistent flags are inherited by all terraform subcommands.
var terraformParser *flags.StandardParser

// terraformCmd represents the base command for all terraform sub-commands.
var terraformCmd = &cobra.Command{
	Use:     "terraform",
	Aliases: []string{"tf"},
	Short:   "Execute Terraform commands using Atmos stack configurations",
	Long:    `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	// FParseErrWhitelist allows unknown flags to pass through to Terraform/OpenTofu.
	// Unlike DisableFlagParsing, this still allows Cobra to parse known Atmos flags.
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

func init() {
	// Create parser with shared terraform flags using functional options.
	// These flags are inherited by all terraform subcommands.
	terraformParser = flags.NewStandardParser(
		WithTerraformFlags(),
		WithTerraformAffectedFlags(),
	)

	// Set stack completion function on the flag registry to avoid import cycle.
	// This must be done before RegisterPersistentFlags() so the completion
	// function is registered when the flag is registered.
	terraformParser.Registry().SetCompletionFunc("stack", stackFlagCompletion)

	// Register as persistent flags (inherited by subcommands).
	terraformParser.RegisterPersistentFlags(terraformCmd)

	// Bind flags to Viper for environment variable support.
	if err := terraformParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add generate subcommand from the generate subpackage.
	terraformCmd.AddCommand(generate.GenerateCmd)

	// Register other completion functions (component args, identity).
	RegisterTerraformCompletions(terraformCmd)

	// Register this command with the registry.
	internal.Register(&TerraformCommandProvider{})
}

// TerraformCommandProvider implements the CommandProvider interface.
type TerraformCommandProvider struct{}

// GetCommand returns the terraform command.
func (t *TerraformCommandProvider) GetCommand() *cobra.Command {
	return terraformCmd
}

// GetName returns the command name.
func (t *TerraformCommandProvider) GetName() string {
	return "terraform"
}

// GetGroup returns the command group for help organization.
func (t *TerraformCommandProvider) GetGroup() string {
	return "Core Stack Commands"
}

// GetAliases returns command aliases.
func (t *TerraformCommandProvider) GetAliases() []internal.CommandAlias {
	return nil // No aliases for terraform command.
}

// GetFlagsBuilder returns the flags builder for this command.
func (t *TerraformCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil // Flags are handled by terraformParser.
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (t *TerraformCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil // Terraform command has subcommands, not positional args.
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (t *TerraformCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return AllTerraformCompatFlags()
}
