package internal

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// CommandAlias represents an alias for a command or subcommand under a different parent.
type CommandAlias struct {
	// Subcommand is the name of the subcommand to alias (empty string for the parent command).
	// For example, "list" to alias "atmos profile list" as "atmos list profiles".
	Subcommand string

	// ParentCommand is the name of the parent command to add the alias under.
	// For example, "list" to create "atmos list profiles" as an alias for "atmos profile list".
	ParentCommand string

	// Name is the alias command name (e.g., "profiles" for "atmos list profiles").
	Name string

	// Short is the short description for the alias command.
	Short string

	// Long is the long description for the alias command.
	Long string

	// Example is the usage example for the alias command.
	Example string
}

// CommandProvider is the interface that built-in command packages implement
// to register themselves with the Atmos command registry.
//
// Commands implementing this interface can be automatically discovered and
// registered with the root command during application initialization.
//
// Example usage:
//
//	type AboutCommandProvider struct{}
//
//	func (a *AboutCommandProvider) GetCommand() *cobra.Command {
//	    return aboutCmd
//	}
//
//	func (a *AboutCommandProvider) GetName() string {
//	    return "about"
//	}
//
//	func (a *AboutCommandProvider) GetGroup() string {
//	    return "Other Commands"
//	}
//
//	func (a *AboutCommandProvider) GetFlagsBuilder() flags.Builder {
//	    return nil
//	}
//
//	func (a *AboutCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
//	    return nil
//	}
//
//	func (a *AboutCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
//	    return nil
//	}
//
//	func (a *AboutCommandProvider) GetAliases() []CommandAlias {
//	    return nil // No aliases
//	}
//
//	func (a *AboutCommandProvider) IsExperimental() bool {
//	    return false
//	}
//
//	func init() {
//	    internal.Register(&AboutCommandProvider{})
//	}
type CommandProvider interface {
	// GetCommand returns the cobra.Command for this provider.
	// This is the command that will be registered with the root command.
	// For commands with subcommands, this should return the parent command
	// with all subcommands already attached.
	GetCommand() *cobra.Command

	// GetName returns the command name (e.g., "about", "terraform").
	// This name is used for registry lookups and must be unique.
	GetName() string

	// GetGroup returns the command group for help organization.
	// Standard groups are:
	//   - "Core Stack Commands"      (terraform, helmfile, workflow, packer)
	//   - "Stack Introspection"      (describe, list, validate)
	//   - "Configuration Management" (vendor, docs)
	//   - "Cloud Integration"        (aws, atlantis)
	//   - "Pro Features"             (auth, pro)
	//   - "Other Commands"           (about, completion, version, support)
	GetGroup() string

	// GetFlagsBuilder returns the flags builder for this command.
	// Return nil if the command has no flags.
	GetFlagsBuilder() flags.Builder

	// GetPositionalArgsBuilder returns the positional args builder for this command.
	// Return nil if the command has no positional arguments.
	GetPositionalArgsBuilder() *flags.PositionalArgsBuilder

	// GetCompatibilityFlags returns compatibility flags for this command.
	// Return nil if the command has no compatibility flags.
	GetCompatibilityFlags() map[string]compat.CompatibilityFlag

	// GetAliases returns a list of command aliases to register.
	// Aliases allow the same command to be accessible under different parent commands.
	// Return nil or an empty slice if the command has no aliases.
	//
	// Example: "atmos profile list" can be aliased as "atmos list profiles":
	//   return []CommandAlias{{
	//       ParentCommand: "list",
	//       Name:          "profiles",
	//       Short:         "List available configuration profiles",
	//       Long:          `This is an alias for "atmos profile list".`,
	//   }}
	GetAliases() []CommandAlias

	// IsExperimental returns true if this command is experimental.
	// Experimental commands may show warnings, be disabled, or cause errors
	// depending on the settings.experimental configuration.
	// Return false for stable commands.
	IsExperimental() bool
}
