package internal

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
)

// CommandProvider is the interface that built-in command packages implement
// to register themselves with the Atmos command registry.
//
// Commands implementing this interface can be automatically discovered and
// registered with the root command during application initialization.
//
// The interface now includes methods for flag configuration, positional args,
// and options parsing, enabling a standardized approach to command registration.
//
// Example usage (simple command with no flags):
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
//	    return nil // No flags
//	}
//
//	func (a *AboutCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
//	    return nil // No positional args
//	}
//
//	func (a *AboutCommandProvider) GetCompatibilityAliases() map[string]flags.CompatibilityAlias {
//	    return nil // No compatibility aliases
//	}
//
//	func init() {
//	    internal.Register(&AboutCommandProvider{})
//	}
//
// Example usage (command with flags and options):
//
//	type VersionCommandProvider struct{}
//
//	func (v *VersionCommandProvider) GetCommand() *cobra.Command {
//	    return versionCmd
//	}
//
//	func (v *VersionCommandProvider) GetName() string {
//	    return "version"
//	}
//
//	func (v *VersionCommandProvider) GetGroup() string {
//	    return "Other Commands"
//	}
//
//	func (v *VersionCommandProvider) GetFlagsBuilder() flags.Builder {
//	    return flags.NewStandardParser(
//	        flags.WithBoolFlag("check", "c", false, "Run additional checks"),
//	        flags.WithStringFlag("format", "", "", "Specify output format"),
//	    )
//	}
//
//	func (v *VersionCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
//	    return nil // No positional args
//	}
//
//	func (v *VersionCommandProvider) GetCompatibilityAliases() map[string]flags.CompatibilityAlias {
//	    return nil // No compatibility aliases (native Cobra flags only)
//	}
//
//	func init() {
//	    internal.Register(&VersionCommandProvider{})
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

	// GetFlagsBuilder returns the flag builder for this command.
	// This can be:
	//   - *flags.StandardParser: For commands using StandardParser (version, list, etc.)
	//   - *flags.FlagRegistry: For commands with per-subcommand flags (terraform, helmfile, packer)
	//   - nil: For commands with no flags (about)
	//
	// The builder is responsible for:
	//   - Defining command-specific flags
	//   - Registering flags with Cobra
	//   - Binding flags to Viper for precedence (CLI > ENV > config > default)
	//
	// Example:
	//   - Simple command: return flags.NewStandardParser(flags.WithBoolFlag(...))
	//   - Per-subcommand: return flags.TerraformPlanFlags() (in subcommand provider)
	//   - No flags: return nil
	GetFlagsBuilder() flags.Builder

	// GetPositionalArgsBuilder returns the positional args builder for this command.
	// This defines positional arguments like:
	//   - component (terraform, helmfile, packer)
	//   - workflow (workflow command)
	//   - key (list keys, etc.)
	//
	// The builder is responsible for:
	//   - Defining positional arg specs (name, required, target field)
	//   - Auto-generating Cobra Args validator
	//   - Auto-generating usage string (e.g., "<component>")
	//
	// Return nil if the command has no positional args.
	//
	// Example:
	//   - Terraform: return flags.NewTerraformPositionalArgsBuilder()
	//   - No positional args: return nil
	GetPositionalArgsBuilder() *flags.PositionalArgsBuilder

	// GetCompatibilityAliases returns compatibility aliases for this command.
	// These translate legacy single-dash multi-char flags to modern format.
	//
	// IMPORTANT: Native Cobra shorthands (-s, -i) are NOT compatibility aliases.
	// Compatibility aliases are only for legacy multi-character single-dash flags:
	//   - Terraform: -var, -var-file, -target, -destroy (legacy terraform syntax)
	//   - Helmfile: -e, -f (legacy helmfile syntax)
	//   - Packer: -on-error, -force (legacy packer syntax)
	//
	// Return nil if the command has no compatibility aliases (most commands).
	//
	// Example:
	//   - Terraform plan: return flags.TerraformPlanCompatibilityAliases()
	//   - No aliases: return nil
	GetCompatibilityAliases() map[string]flags.CompatibilityAlias
}
