package terraform

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/terraform/generate"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cmdNameTerraform is the command name for terraform operations.
const cmdNameTerraform = "terraform"

// terraformParser handles flag parsing for shared terraform flags.
// These persistent flags are inherited by all terraform subcommands.
var terraformParser *flags.StandardParser

// terraformCmd represents the base command for all terraform sub-commands.
var terraformCmd = &cobra.Command{
	Use:     cmdNameTerraform,
	Aliases: []string{"tf"},
	Short:   "Execute Terraform commands using Atmos stack configurations",
	Long:    `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	// RunE handles global terraform flags (-version, -help, -chdir) when no subcommand is provided.
	// These flags should be passed directly to terraform without requiring component/stack context.
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if we have global terraform flags in separated args.
		separated := compat.GetSeparated()
		if len(separated) > 0 && hasGlobalOnlyFlags(separated) {
			// Run terraform directly with these global flags.
			return runTerraformGlobal(separated)
		}
		// No global flags or mixed with other args - show help (default behavior for parent command).
		return cmd.Help()
	},
}

func init() {
	// Create parser with shared terraform flags using functional options.
	// These flags are inherited by all terraform subcommands.
	// Use local WithTerraformFlags/WithTerraformAffectedFlags from cmd/terraform/flags.go.
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

	// Register global terraform compat flags (shown on `atmos terraform --help`).
	// These are TRUE GLOBAL terraform flags that can be used before any subcommand.
	internal.RegisterCommandCompatFlags(cmdNameTerraform, cmdNameTerraform, TerraformGlobalCompatFlags())

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
	return cmdNameTerraform
}

// GetGroup returns the command group for help organization.
func (t *TerraformCommandProvider) GetGroup() string {
	return "Core Stack Commands"
}

// GetFlagsBuilder returns the flags builder for this command.
// Terraform command uses legacy flags via terraformParser.
func (t *TerraformCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
// Terraform command has no positional arguments at the parent level.
func (t *TerraformCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
// Returns all terraform compatibility flags (combined from all subcommands) to enable
// preprocessing in Execute() to separate pass-through flags before Cobra parses.
func (t *TerraformCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return AllTerraformCompatFlags()
}

// GetAliases returns command aliases.
func (t *TerraformCommandProvider) GetAliases() []internal.CommandAlias {
	return nil // No aliases for terraform command
}

// globalOnlyFlags are terraform flags that can be used without a subcommand.
// These flags don't require component/stack context - they're passed directly to terraform.
var globalOnlyFlags = map[string]bool{
	"-version": true,
	"-help":    true,
	"-chdir":   true,
}

// hasGlobalOnlyFlags checks if the separated args contain only global terraform flags.
// Returns true if all flags in the args are global-only flags.
func hasGlobalOnlyFlags(args []string) bool {
	for _, arg := range args {
		// Check the flag name (handle both -flag and -flag=value forms).
		flagName := arg
		if idx := strings.Index(arg, "="); idx > 0 {
			flagName = arg[:idx]
		}
		if !globalOnlyFlags[flagName] {
			return false
		}
	}
	return true
}

// runTerraformGlobal executes terraform with global flags directly.
// This is used for commands like `atmos terraform -version` that don't need component/stack context.
func runTerraformGlobal(args []string) error {
	// For global flags, we just need to run terraform directly.
	// No component/stack context is needed.
	// Use "terraform" as the default command - atmos config would typically
	// specify whether to use terraform or tofu, but for -version/-help
	// we can just use the default.
	return exec.ExecuteShellCommand(schema.AtmosConfiguration{}, cmdNameTerraform, args, "", nil, false, "")
}
