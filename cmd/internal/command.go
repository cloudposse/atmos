package internal

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

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
}
