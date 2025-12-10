package list

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// listCmd commands list stacks and components.
var listCmd = &cobra.Command{
	Use:                "list",
	Short:              "List available stacks and components",
	Long:               `Display a list of all available stacks and components defined in your project.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	// Add --identity flag to all list commands to enable authentication
	// when processing YAML template functions (!terraform.state, !terraform.output).
	// This follows the same pattern as the describe commands.
	//
	// NOTE: NoOptDefVal is NOT used here to avoid Cobra parsing issues with commands
	// that have positional arguments. When NoOptDefVal is set and a space-separated value
	// is used (--identity value), Cobra misinterprets the value as a subcommand/positional arg.
	//
	// The ATMOS_IDENTITY environment variable binding is handled centrally by the global
	// flag registry in pkg/flags/global_builder.go, so no explicit viper.BindEnv is needed here.
	listCmd.PersistentFlags().StringP("identity", "i", "", "Specify the identity to authenticate with")

	// Attach all subcommands
	listCmd.AddCommand(componentsCmd)
	listCmd.AddCommand(stacksCmd)
	listCmd.AddCommand(themesCmd)
	listCmd.AddCommand(workflowsCmd)
	listCmd.AddCommand(vendorCmd)
	listCmd.AddCommand(instancesCmd)
	listCmd.AddCommand(metadataCmd)
	listCmd.AddCommand(settingsCmd)
	listCmd.AddCommand(valuesCmd)
	listCmd.AddCommand(varsCmd)

	// Register with registry
	internal.Register(&ListCommandProvider{})
}

// ListCommandProvider implements the CommandProvider interface.
type ListCommandProvider struct{}

func (l *ListCommandProvider) GetCommand() *cobra.Command {
	return listCmd
}

func (l *ListCommandProvider) GetName() string {
	return "list"
}

func (l *ListCommandProvider) GetGroup() string {
	return "Stack Introspection"
}

func (l *ListCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (l *ListCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (l *ListCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases (none for list command).
func (l *ListCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}
