package flags

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Builder is the interface for flag configuration builders.
//
// This interface is implemented by:
//   - StandardParser: Simple commands with direct flag registration
//   - FlagRegistry: Per-subcommand flag registration (terraform, helmfile, packer)
//
// Commands implementing CommandProvider.GetFlagsBuilder() return a Builder
// to enable standardized flag registration across all commands.
//
// Example usage:
//
//	// Simple command (version, list, etc.)
//	func (v *VersionCommandProvider) GetFlagsBuilder() Builder {
//	    return NewStandardParser(
//	        WithBoolFlag("check", "c", false, "Run additional checks"),
//	    )
//	}
//
//	// Per-subcommand (terraform plan)
//	func (t *TerraformPlanCommandProvider) GetFlagsBuilder() Builder {
//	    return TerraformPlanFlags()  // Returns *FlagRegistry
//	}
//
//	// No flags (about)
//	func (a *AboutCommandProvider) GetFlagsBuilder() Builder {
//	    return nil
//	}
type Builder interface {
	// RegisterFlags registers flags with a Cobra command.
	// This attaches flags to the command so Cobra can parse them.
	RegisterFlags(cmd *cobra.Command)

	// BindToViper binds flags to a Viper instance for precedence.
	// This enables CLI > ENV > config > default resolution.
	// Returns error if binding fails.
	BindToViper(v *viper.Viper) error
}
