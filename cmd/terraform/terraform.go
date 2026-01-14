package terraform

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/terraform/backend"
	"github.com/cloudposse/atmos/cmd/terraform/generate"
	"github.com/cloudposse/atmos/cmd/terraform/source"
	"github.com/cloudposse/atmos/cmd/terraform/workdir"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
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
	// RunE handles the case when terraform is called without a subcommand.
	// This allows global compat flags like -help and -version to be passed through
	// to the underlying terraform/tofu command.
	RunE: terraformGlobalFlagsHandler,
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

	// Add backend subcommand from the backend subpackage.
	terraformCmd.AddCommand(backend.GetBackendCommand())

	// Add source subcommand from the source subpackage.
	terraformCmd.AddCommand(source.GetSourceCommand())

	// Add workdir subcommand from the workdir subpackage.
	terraformCmd.AddCommand(workdir.GetWorkdirCommand())

	// Register other completion functions (component args, identity).
	RegisterTerraformCompletions(terraformCmd)

	// Register global compat flags for terraform command itself (not just subcommands).
	// This enables the COMPATIBILITY FLAGS section in help output.
	internal.RegisterCommandCompatFlags("terraform", "terraform", TerraformGlobalCompatFlags())

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

// flagPrefix is the prefix for CLI flags.
const flagPrefix = "-"

// terraformGlobalFlagsHandler handles the terraform command when called without a subcommand.
// It checks for global compat flags (like -help, -version) that were separated by
// preprocessCompatibilityFlags() and passes them through to the underlying terraform/tofu command.
//
// This integrates with the compat flag system:
//   - The preprocessCompatibilityFlags() in cmd/root.go separates compat flags before Cobra parses.
//   - Separated flags are stored via compat.SetSeparated() and retrieved via compat.GetSeparated().
//   - This handler retrieves those flags and executes terraform with them.
func terraformGlobalFlagsHandler(cmd *cobra.Command, args []string) error {
	// Check if the user provided an unknown subcommand (not a flag).
	// This happens when Cobra can't match a subcommand and falls back to RunE.
	for _, arg := range args {
		if !strings.HasPrefix(arg, flagPrefix) {
			// Found a non-flag argument - this is an unknown subcommand.
			// Use the standard error handler for unknown commands.
			showUnknownSubcommandError(cmd, arg)
			return nil // showUnknownSubcommandError calls os.Exit
		}
	}

	// Check for global compat flags that were separated by preprocessCompatibilityFlags().
	// These flags (like -help, -version) should be passed directly to terraform.
	separated := compat.GetSeparated()

	// Look for global flags that should be passed through.
	globalFlags := TerraformGlobalCompatFlags()
	for _, arg := range separated {
		if !strings.HasPrefix(arg, flagPrefix) {
			continue
		}

		// Extract the flag name (handle both -flag and --flag, and -flag=value).
		flagName := strings.TrimPrefix(strings.TrimPrefix(arg, flagPrefix), flagPrefix)
		if idx := strings.Index(flagName, "="); idx != -1 {
			flagName = flagName[:idx]
		}

		// Check if it's a global flag that should be passed through.
		if _, isGlobal := globalFlags[flagPrefix+flagName]; isGlobal {
			// Initialize config to get terraform command.
			atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
			if err != nil {
				return err
			}

			// Get the terraform command, with fallback to default.
			command := atmosConfig.Components.Terraform.Command
			if command == "" {
				command = cfg.TerraformComponentType // Default to "terraform".
			}

			// Execute terraform with the separated args (global flags).
			return e.ExecuteShellCommand(
				atmosConfig,
				command,
				separated,
				"",    // dir (current dir)
				nil,   // env
				false, // dryRun
				"",    // redirectStdError
			)
		}
	}

	// No global flag found and no subcommand provided - show usage.
	return cmd.Usage()
}

// showUnknownSubcommandError displays an error message for unknown subcommands.
// This mirrors the behavior of showErrorExampleFromMarkdown in cmd/cmd_utils.go
// but is specific to terraform commands.
func showUnknownSubcommandError(cmd *cobra.Command, unknownCmd string) {
	commandPath := cmd.CommandPath()

	// Build error explanation.
	var explanation strings.Builder
	explanation.WriteString("Unknown command `")
	explanation.WriteString(unknownCmd)
	explanation.WriteString("` for `")
	explanation.WriteString(commandPath)
	explanation.WriteString("`\n")

	// Check for suggestions.
	suggestions := cmd.SuggestionsFor(unknownCmd)
	if len(suggestions) > 0 {
		explanation.WriteString("\nDid you mean this?\n\n")
		for _, suggestion := range suggestions {
			explanation.WriteString("- ")
			explanation.WriteString(suggestion)
			explanation.WriteString("\n")
		}
	} else if len(cmd.Commands()) > 0 {
		explanation.WriteString("\nValid subcommands are:\n\n")
		for _, subCmd := range cmd.Commands() {
			explanation.WriteString("- ")
			explanation.WriteString(subCmd.Name())
			explanation.WriteString("\n")
		}
	}

	// Add usage examples section.
	explanation.WriteString("\n## Usage Examples:\n")
	explanation.WriteString("\n– Execute a terraform subcommand\n\n")
	explanation.WriteString("  $ atmos terraform [subcommand] <component-name> -s <stack-name>\n")

	// Build the error using the sentinel + ErrorBuilder pattern.
	err := errUtils.Build(errUtils.ErrUnknownSubcommand).
		WithExplanation(explanation.String()).
		WithHint("https://atmos.tools/cli/commands/terraform/usage").
		Err()

	errUtils.CheckErrorPrintAndExit(err, "Incorrect Usage", "")
}
