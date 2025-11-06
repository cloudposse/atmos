package packer

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Options provides strongly-typed access to parsed Packer command flags.
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
type Options struct {
	flags.GlobalFlags // Embedded global flags (chdir, logs-level, identity, etc.)

	// Common flags (shared with Terraform, Helmfile).
	Stack  string // --stack/-s: Target stack name.
	DryRun bool   // --dry-run: Perform dry run without making actual changes.

	// Positional arguments (populated automatically by parser from TargetField mapping).
	Component string // Component name from positional arg (e.g., "ami" in: atmos packer build ami)

	// Internal: Positional and pass-through arguments (use GetPositionalArgs/GetSeparatedArgs).
	positionalArgs  []string // e.g., ["build", "image"] in: atmos packer build image
	passThroughArgs []string // e.g., ["-var", "foo=bar"] in: atmos packer build -- -var foo=bar
}

// ParseFlags parses Packer command flags from Cobra command and Viper.
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
func ParseFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) Options {
	defer perf.Track(nil, "packer.ParseFlags")()

	// Extract component from positional args
	// Packer commands: atmos packer <subcommand> <component>
	// positionalArgs[0] = component name (ami, docker, etc.)
	// Packer passes subcommand separately to packerRun, so positionalArgs contains only component.
	component := ""
	if len(positionalArgs) >= 1 {
		component = positionalArgs[0]
	}

	return Options{
		GlobalFlags: flags.ParseGlobalFlags(cmd, v),

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
func (p *Options) GetPositionalArgs() []string {
	defer perf.Track(nil, "packer.Options.GetPositionalArgs")()

	return p.positionalArgs
}

// GetSeparatedArgs returns pass-through arguments (e.g., ["-var", "foo=bar"]).
func (p *Options) GetSeparatedArgs() []string {
	defer perf.Track(nil, "packer.Options.GetSeparatedArgs")()

	return p.passThroughArgs
}
