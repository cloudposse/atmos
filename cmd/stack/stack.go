package stack

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

// atmosConfigPtr is set by SetAtmosConfig from root.go before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the stack command group.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// stackCmd is the parent for reading and editing stack manifests. Edits use
// provenance to resolve which manifest file actually defines a value, and
// preserve comments, anchors, YAML functions, and templates.
var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Read and edit stack manifests for a component",
	Long: `Read and edit values for a component in a stack using dot-notation paths.

Edits are component-relative (e.g. vars.region) and Atmos uses provenance to find
the manifest file that actually defines the effective value, then edits that file
in place — preserving comments, anchors/aliases, Atmos YAML functions, and Go
templates. Use --file to target a specific manifest explicitly.`,
	Args: cobra.NoArgs,
}

func init() {
	stackCmd.AddCommand(stackGetCmd)
	stackCmd.AddCommand(stackSetCmd)
	stackCmd.AddCommand(stackDeleteCmd)
	stackCmd.AddCommand(stackConfigCmd)

	internal.Register(&CommandProvider{})
}

// CommandProvider implements the registry CommandProvider interface.
type CommandProvider struct{}

// GetCommand returns the stack command with its subcommands attached.
func (p *CommandProvider) GetCommand() *cobra.Command { return stackCmd }

// GetName returns the unique command name.
func (p *CommandProvider) GetName() string { return "stack" }

// GetGroup returns the help group for this command.
func (p *CommandProvider) GetGroup() string { return "Stack Introspection" }

// GetFlagsBuilder returns the flags builder (none at the group level).
func (p *CommandProvider) GetFlagsBuilder() flags.Builder { return nil }

// GetPositionalArgsBuilder returns the positional args builder (none).
func (p *CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder { return nil }

// GetCompatibilityFlags returns compatibility flags (none).
func (p *CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag { return nil }

// GetAliases returns command aliases (none).
func (p *CommandProvider) GetAliases() []internal.CommandAlias { return nil }

// IsExperimental reports whether the command is experimental.
func (p *CommandProvider) IsExperimental() bool { return false }
