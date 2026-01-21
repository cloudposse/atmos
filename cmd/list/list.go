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
	listCmd.AddCommand(affectedCmd)
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
	listCmd.AddCommand(sourcesCmd)

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

// GetAliases returns command aliases for list subcommands.
// Creates "atmos vendor list" as an alias for "atmos list vendor".
// Creates "atmos workflow list" as an alias for "atmos list workflows".
func (l *ListCommandProvider) GetAliases() []internal.CommandAlias {
	return []internal.CommandAlias{
		{
			Subcommand:    "vendor",
			ParentCommand: "vendor",
			Name:          "list",
			Short:         "List all vendor configurations (alias for 'atmos list vendor')",
			Long:          `List Atmos vendor configurations including component and vendor manifests with support for filtering, custom column selection, sorting, and multiple output formats. This is an alias for "atmos list vendor".`,
			Example:       "atmos vendor list\natmos vendor list --format json",
		},
		{
			Subcommand:    "workflows",
			ParentCommand: "workflow",
			Name:          "list",
			Short:         "List all Atmos workflows (alias for 'atmos list workflows')",
			Long:          `List Atmos workflows with support for filtering by file, custom column selection, sorting, and multiple output formats. This is an alias for "atmos list workflows".`,
			Example:       "atmos workflow list\natmos workflow list --format json",
		},
	}
}

// IsExperimental returns whether this command is experimental.
func (l *ListCommandProvider) IsExperimental() bool {
	return false
}
