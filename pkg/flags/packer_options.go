package flags

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// PackerOptions provides strongly-typed access to parsed Packer command flags.
//
// Example usage:
//
//	parser := flagparser.NewPackerParser()
//	opts := flagparser.ParsePackerFlags(cmd, viper.GetViper(), positionalArgs, passThroughArgs)
//
//	// Type-safe access to flags:
//	if opts.Stack == "" {
//	    return errors.New("stack is required")
//	}
//
//	// Type-safe access to positional args (populated automatically by parser):
//	fmt.Printf("Building component: %s\n", opts.Component)
//
// See docs/prd/flag-handling/ for patterns.
type PackerOptions struct {
	GlobalFlags // Embedded global flags (chdir, logs-level, identity, etc.)

	// Common flags (shared with Terraform, Helmfile).
	Stack  string // --stack/-s: Target stack name.
	DryRun bool   // --dry-run: Perform dry run without making actual changes.

	// Positional arguments (populated automatically by parser from TargetField mapping).
	Component string // Component name from positional arg (e.g., "ami" in: atmos packer build ami)

	// Internal: Positional and pass-through arguments (use GetPositionalArgs/GetPassThroughArgs).
	positionalArgs  []string // e.g., ["build", "image"] in: atmos packer build image
	passThroughArgs []string // e.g., ["-var", "foo=bar"] in: atmos packer build -- -var foo=bar
}

// ParsePackerFlags parses Packer command flags from Cobra command and Viper.
//
// This function:
//  1. Parses global flags (chdir, logs-level, identity, pager, profiler, etc.)
//  2. Parses common flags (stack, dry-run)
//  3. Extracts and populates positional arguments (component name)
//  4. Stores positional and pass-through arguments
//
// Arguments:
//   - cmd: The Cobra command being executed (used to check if flags were provided).
//   - v: Viper instance with bound flags (precedence: CLI > ENV > config > default).
//   - positionalArgs: Positional arguments after command name.
//   - passThroughArgs: Arguments after -- separator to pass to packer.
//
// Example:
//
//	atmos packer build ami -s dev --identity=prod -- -var foo=bar
//	                    ^^^ ^^^^^^^^^^^^^^^^^^^^^^^^    ^^^^^^^^^^^^^
//	                     |            |                       |
//	                positional   common flags          pass-through
//	                (Component)                             args
func ParsePackerFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) PackerOptions {
	defer perf.Track(nil, "flagparser.ParsePackerFlags")()

	// Extract component from positional args
	// Packer commands: atmos packer <subcommand> <component>
	// positionalArgs[0] = subcommand (build, init, inspect, etc.)
	// positionalArgs[1] = component name (ami, docker, etc.)
	component := ""
	if len(positionalArgs) >= 2 {
		component = positionalArgs[1]
	}

	return PackerOptions{
		GlobalFlags: ParseGlobalFlags(cmd, v),

		// Common flags.
		Stack:  v.GetString("stack"),
		DryRun: v.GetBool("dry-run"),

		// Positional arguments.
		Component: component,

		// Internal arguments.
		positionalArgs:  positionalArgs,
		passThroughArgs: passThroughArgs,
	}
}

// GetPositionalArgs returns positional arguments (e.g., ["build", "image"]).
func (p *PackerOptions) GetPositionalArgs() []string {
	defer perf.Track(nil, "flagparser.PackerOptions.GetPositionalArgs")()

	return p.positionalArgs
}

// GetPassThroughArgs returns pass-through arguments (e.g., ["-var", "foo=bar"]).
func (p *PackerOptions) GetPassThroughArgs() []string {
	defer perf.Track(nil, "flagparser.PackerOptions.GetPassThroughArgs")()

	return p.passThroughArgs
}

// PackerFlagsRegistry returns a registry with all Packer command flags.
//
// Includes:
//   - Global flags (from GlobalFlagsRegistry)
//   - Common flags: stack, identity, dry-run
//
// This registry is used to:
//   - Register flags with Cobra commands
//   - Bind flags to Viper for precedence handling
//   - Validate required flags
func PackerFlagsRegistry() *FlagRegistry {
	defer perf.Track(nil, "flagparser.PackerFlagsRegistry")()

	return PackerFlags()
}
