package list

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

const listCommandName = "list"

// stackCommandName is the parent command that hosts the stack list and instances aliases.
const stackCommandName = "stack"

// listCmd commands list stacks and components.
// Args validator is auto-applied by the command registry for parent commands with subcommands.
var listCmd = &cobra.Command{
	Use:                listCommandName,
	Short:              "List available stacks and components",
	Long:               `Display a list of all available stacks and components defined in your project.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	// Add --identity flag to all list commands to enable authentication
	// when processing YAML template functions (!terraform.state, !terraform.output).
	//
	// Uses the shared flags.WithIdentityFlag() builder (rather than a hand-rolled
	// PersistentFlags().StringP()) so bare --identity triggers the interactive selector like
	// every other Atmos command. Space-separated values on subcommands with positional args
	// are normalized to `--identity=value` by the generic NoOptDefVal preprocessor in
	// cmd/root.go before Cobra parses.
	//
	// The ATMOS_IDENTITY environment variable binding is handled centrally by the global
	// flag registry in pkg/flags/global_builder.go, so no explicit viper.BindEnv is needed here.
	flags.NewStandardParser(flags.WithIdentityFlag()).RegisterPersistentFlags(listCmd)

	// Attach all subcommands
	listCmd.AddCommand(affectedCmd)
	listCmd.AddCommand(aliasesCmd)
	listCmd.AddCommand(componentsCmd)
	listCmd.AddCommand(dependenciesCmd)
	listCmd.AddCommand(editionsCmd)
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
	internal.Register(
		&ListCommandProvider{},
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/list/usage"),
		internal.WithExample("examples/demo-stacks", "Stacks"),
	)
	// The stack aliases have their own documentation pages.
	internal.RegisterAliasMetadata(stackCommandName, listCommandName,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/stack/stack-list"))
	internal.RegisterAliasMetadata(stackCommandName, "instances",
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/stack/stack-instances"))
}

// ListCommandProvider implements the CommandProvider interface.
type ListCommandProvider struct{}

func (l *ListCommandProvider) GetCommand() *cobra.Command {
	return listCmd
}

func (l *ListCommandProvider) GetName() string {
	return listCommandName
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
// Creates "atmos stack list" as an alias for "atmos list stacks".
// Creates "atmos stack instances" as an alias for "atmos list instances".
func (l *ListCommandProvider) GetAliases() []internal.CommandAlias {
	return []internal.CommandAlias{
		{
			Subcommand:    "vendor",
			ParentCommand: "vendor",
			Name:          listCommandName,
			Short:         "List all vendor configurations (alias for 'atmos list vendor')",
			Long:          `List Atmos vendor configurations including component and vendor manifests with support for filtering, custom column selection, sorting, and multiple output formats. This is an alias for "atmos list vendor".`,
			Example:       "atmos vendor list\natmos vendor list --format json",
		},
		{
			Subcommand:    "workflows",
			ParentCommand: "workflow",
			Name:          listCommandName,
			Short:         "List all Atmos workflows (alias for 'atmos list workflows')",
			Long:          `List Atmos workflows with support for filtering by file, custom column selection, sorting, and multiple output formats. This is an alias for "atmos list workflows".`,
			Example:       "atmos workflow list\natmos workflow list --format json",
		},
		{
			Subcommand:    "stacks",
			ParentCommand: stackCommandName,
			Name:          listCommandName,
			Short:         "List Atmos stacks (alias for 'atmos list stacks')",
			Long:          `List Atmos stacks with filtering, sorting, and multiple output formats. This is an alias for "atmos list stacks".`,
			Example:       "atmos stack list\natmos stack list --component vpc --format json",
		},
		{
			Subcommand:    "instances",
			ParentCommand: stackCommandName,
			Name:          "instances",
			Short:         "List component instances (alias for 'atmos list instances')",
			Long:          `List component instances across Atmos stacks with filtering, sorting, and multiple output formats. This is an alias for "atmos list instances".`,
			Example:       "atmos stack instances\natmos stack instances --stack prod --format json",
		},
	}
}

// IsExperimental returns whether this command is experimental.
func (l *ListCommandProvider) IsExperimental() bool {
	return false
}
