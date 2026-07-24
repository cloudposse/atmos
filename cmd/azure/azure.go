package azure

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/azure/acr"
	"github.com/cloudposse/atmos/cmd/azure/aks"
	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// doubleDashHint is displayed in help output.
const doubleDashHint = "Use double dashes to separate Atmos-specific options from native arguments and flags for the command."

// azureCmd executes 'azure' CLI commands.
var azureCmd = &cobra.Command{
	Use:                "azure",
	Short:              "Run Azure-specific commands for interacting with cloud resources",
	Long:               `This command allows interaction with Azure resources through various CLI commands.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	azureCmd.PersistentFlags().Bool("", false, doubleDashHint)

	// Add ACR subcommand from the acr subpackage.
	azureCmd.AddCommand(acr.AcrCmd)

	// Add AKS subcommand from the aks subpackage.
	azureCmd.AddCommand(aks.AksCmd)

	// Register this command with the registry.
	internal.Register(&AzureCommandProvider{})
}

// AzureCommandProvider implements the CommandProvider interface.
type AzureCommandProvider struct{}

// GetCommand returns the azure command.
func (a *AzureCommandProvider) GetCommand() *cobra.Command {
	return azureCmd
}

// GetName returns the command name.
func (a *AzureCommandProvider) GetName() string {
	return "azure"
}

// GetGroup returns the command group for help organization.
func (a *AzureCommandProvider) GetGroup() string {
	return "Cloud Integration"
}

// GetAliases returns command aliases.
func (a *AzureCommandProvider) GetAliases() []internal.CommandAlias {
	return nil // No aliases for azure command.
}

// GetFlagsBuilder returns the flags builder for this command.
func (a *AzureCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (a *AzureCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil // Azure command has subcommands, not positional args.
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (a *AzureCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (a *AzureCommandProvider) IsExperimental() bool {
	return false
}
