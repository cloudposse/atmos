package helmfile

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Options provides strongly-typed access to parsed Helmfile command flags.
//
// Example usage:
//
//	parser := flagparser.NewHelmfileParser()
//	opts := flagparser.ParseHelmfileFlags(cmd, viper.GetViper(), positionalArgs, passThroughArgs)
//
//	// Type-safe access to flags:
//	if opts.Stack == "" {
//	    return errors.New("stack is required")
//	}
//
//	// Type-safe access to positional args (populated automatically by parser):
//	fmt.Printf("Applying component: %s\n", opts.Component)
//
// See docs/prd/flag-handling/ for patterns.
type Options struct {
	global.Flags // Embedded global flags (chdir, logs-level, identity, etc.)

	// Common flags (shared with Terraform, Packer).
	Stack  string // --stack/-s: Target stack name.
	DryRun bool   // --dry-run: Perform dry run without making actual changes.

	// Positional arguments (populated automatically by parser from TargetField mapping).
	Component string // Component name from positional arg (e.g., "nginx" in: atmos helmfile apply nginx)

	// Internal: Positional and pass-through arguments (use GetPositionalArgs/GetSeparatedArgs).
	positionalArgs  []string // e.g., ["sync", "vpc"] in: atmos helmfile sync vpc
	passThroughArgs []string // e.g., ["--args", "foo"] in: atmos helmfile sync -- --args foo
}

// ParseFlags parses Helmfile command flags from Cobra command and Viper.
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
//   - passThroughArgs: Arguments after -- separator to pass to helmfile.
//
// Example:
//
//	atmos helmfile apply nginx -s dev --identity=prod -- --args foo
//	                     ^^^^^ ^^^^^^^^^^^^^^^^^^^^^^^^    ^^^^^^^^^^
//	                       |            |                       |
//	                  positional   common flags          pass-through
//	                  (Component)                             args
func ParseFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) Options {
	defer perf.Track(nil, "helmfile.ParseFlags")()

	// Extract component from positional args
	// Helmfile commands: atmos helmfile <subcommand> <component>
	// positionalArgs[0] = component name (nginx, redis, etc.)
	// Helmfile passes subcommand separately to helmfileRun, so positionalArgs contains only component.
	component := ""
	if len(positionalArgs) >= 1 {
		component = positionalArgs[0]
	}

	return Options{
		Flags: flags.ParseGlobalFlags(cmd, v),

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

// GetPositionalArgs returns positional arguments (e.g., ["sync", "vpc"]).
func (h *Options) GetPositionalArgs() []string {
	defer perf.Track(nil, "helmfile.Options.GetPositionalArgs")()

	return h.positionalArgs
}

// GetSeparatedArgs returns pass-through arguments (e.g., ["--args", "foo"]).
func (h *Options) GetSeparatedArgs() []string {
	defer perf.Track(nil, "helmfile.Options.GetSeparatedArgs")()

	return h.passThroughArgs
}
