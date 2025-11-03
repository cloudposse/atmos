package flagparser

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// HelmfileInterpreter provides strongly-typed access to parsed Helmfile command flags.
//
// Example usage:
//
//	parser := flagparser.NewHelmfileParser()
//	interpreter := flagparser.ParseHelmfileFlags(cmd, viper.GetViper(), positionalArgs, passThroughArgs)
//
//	// Type-safe access to flags:
//	if interpreter.Stack == "" {
//	    return errors.New("stack is required")
//	}
//
// See docs/prd/flag-parser/ for patterns.
type HelmfileInterpreter struct {
	GlobalFlags // Embedded global flags (chdir, logs-level, identity, etc.)

	// Common flags (shared with Terraform, Packer).
	Stack    string // --stack/-s: Target stack name.
	Identity IdentitySelector
	DryRun   bool // --dry-run: Perform dry run without making actual changes.

	// Positional and pass-through arguments.
	positionalArgs  []string // e.g., ["sync", "vpc"] in: atmos helmfile sync vpc
	passThroughArgs []string // e.g., ["--args", "foo"] in: atmos helmfile sync -- --args foo
}

// ParseHelmfileFlags parses Helmfile command flags from Cobra command and Viper.
//
// This function:
//  1. Parses global flags (chdir, logs-level, identity, pager, profiler, etc.)
//  2. Parses common flags (stack, dry-run)
//  3. Stores positional and pass-through arguments
//
// Arguments:
//   - cmd: The Cobra command being executed (used to check if flags were provided).
//   - v: Viper instance with bound flags (precedence: CLI > ENV > config > default).
//   - positionalArgs: Positional arguments after command name.
//   - passThroughArgs: Arguments after -- separator to pass to helmfile.
func ParseHelmfileFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) HelmfileInterpreter {
	defer perf.Track(nil, "flagparser.ParseHelmfileFlags")()

	return HelmfileInterpreter{
		GlobalFlags: ParseGlobalFlags(cmd, v),

		// Common flags.
		Stack:    v.GetString("stack"),
		Identity: parseIdentityFlag(cmd, v),
		DryRun:   v.GetBool("dry-run"),

		// Arguments.
		positionalArgs:  positionalArgs,
		passThroughArgs: passThroughArgs,
	}
}

// GetPositionalArgs returns positional arguments (e.g., ["sync", "vpc"]).
func (h *HelmfileInterpreter) GetPositionalArgs() []string {
	return h.positionalArgs
}

// GetPassThroughArgs returns pass-through arguments (e.g., ["--args", "foo"]).
func (h *HelmfileInterpreter) GetPassThroughArgs() []string {
	return h.passThroughArgs
}

// HelmfileFlagsRegistry returns a registry with all Helmfile command flags.
//
// Includes:
//   - Global flags (from GlobalFlagsRegistry)
//   - Common flags: stack, identity, dry-run
//
// This registry is used to:
//   - Register flags with Cobra commands
//   - Bind flags to Viper for precedence handling
//   - Validate required flags
func HelmfileFlagsRegistry() *FlagRegistry {
	defer perf.Track(nil, "flagparser.HelmfileFlagsRegistry")()

	return HelmfileFlags()
}
