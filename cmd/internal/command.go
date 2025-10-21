package internal

import "github.com/spf13/cobra"

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
}
