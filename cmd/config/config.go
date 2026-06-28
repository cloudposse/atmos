package config

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

// atmosConfigPtr is set by SetAtmosConfig from root.go before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the config command group.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// configCmd is the parent for reading and editing the active atmos.yaml while
// preserving comments, anchors, YAML functions, and templates.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Read and edit your atmos.yaml configuration",
	Long: `Read and edit values in your atmos.yaml configuration using dot-notation paths.

Edits preserve comments, anchors/aliases, Atmos YAML functions, and Go templates.
By default the command targets the atmos.yaml in the current directory or git
root; use the global --config flag to target a specific file.`,
	Args: cobra.NoArgs,
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configDeleteCmd)
	configCmd.AddCommand(configListCmd)

	internal.Register(&CommandProvider{})
}

// CommandProvider implements the registry CommandProvider interface.
type CommandProvider struct{}

// GetCommand returns the config command with its subcommands attached.
func (p *CommandProvider) GetCommand() *cobra.Command { return configCmd }

// GetName returns the unique command name.
func (p *CommandProvider) GetName() string { return "config" }

// GetGroup returns the help group for this command.
func (p *CommandProvider) GetGroup() string { return "Configuration Management" }

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
