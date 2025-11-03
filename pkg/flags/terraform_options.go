package flags

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// TerraformOptions provides strongly-typed access to parsed Terraform command flags.
//
// Example usage:
//
//	parser := flagparser.NewPassThroughFlagParser(
//	    flagparser.WithTerraformFlags(),
//	)
//	interpreter := flagparser.ParseTerraformFlags(cmd, viper.GetViper(), positionalArgs, passThroughArgs)
//
//	// Type-safe access to flags:
//	if interpreter.Stack == "" {
//	    return errors.New("stack is required")
//	}
//	if interpreter.UploadStatus {
//	    uploadPlanToAtmosPro()
//	}
//
// See docs/prd/flag-handling/ for patterns.
type TerraformOptions struct {
	GlobalFlags // Embedded global flags (chdir, logs-level, identity, etc.)

	// Common flags (shared with Helmfile, Packer).
	Stack    string // --stack/-s: Target stack name.
	Identity IdentitySelector
	DryRun   bool // --dry-run: Perform dry run without making actual changes.

	// Terraform-specific flags.
	UploadStatus bool   // --upload-status: Upload plan status to Atmos Pro.
	SkipInit     bool   // --skip-init: Skip terraform init before running command.
	FromPlan     string // --from-plan: Apply from previously generated plan file.

	// Positional and pass-through arguments.
	positionalArgs  []string // e.g., ["plan", "vpc"] in: atmos terraform plan vpc
	passThroughArgs []string // e.g., ["-var", "foo=bar"] in: atmos terraform plan -- -var foo=bar
}

// ParseTerraformFlags parses Terraform command flags from Cobra command and Viper.
//
// This function:
//  1. Parses global flags (chdir, logs-level, identity, pager, profiler, etc.)
//  2. Parses common flags (stack, dry-run)
//  3. Parses Terraform-specific flags (upload-status, skip-init, from-plan)
//  4. Stores positional and pass-through arguments
//
// Arguments:
//   - cmd: The Cobra command being executed (used to check if flags were provided).
//   - v: Viper instance with bound flags (precedence: CLI > ENV > config > default).
//   - positionalArgs: Positional arguments after command name.
//   - passThroughArgs: Arguments after -- separator to pass to terraform.
//
// Example:
//
//	atmos terraform plan vpc -s dev --identity=prod --upload-status -- -var foo=bar
//	                    ^^^   ^^^^^ ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^    ^^^^^^^^^^^^
//	                     |      |              |                               |
//	                positional common+    terraform-specific            pass-through
//	                           global flags     flags                        args
func ParseTerraformFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) TerraformOptions {
	defer perf.Track(nil, "flagparser.ParseTerraformFlags")()

	return TerraformOptions{
		GlobalFlags: ParseGlobalFlags(cmd, v),

		// Common flags.
		Stack:    v.GetString("stack"),
		Identity: parseIdentityFlag(cmd, v),
		DryRun:   v.GetBool("dry-run"),

		// Terraform-specific flags.
		UploadStatus: v.GetBool("upload-status"),
		SkipInit:     v.GetBool("skip-init"),
		FromPlan:     v.GetString("from-plan"),

		// Arguments.
		positionalArgs:  positionalArgs,
		passThroughArgs: passThroughArgs,
	}
}

// GetPositionalArgs returns positional arguments (e.g., ["plan", "vpc"]).
func (t *TerraformOptions) GetPositionalArgs() []string {
	defer perf.Track(nil, "flagparser.TerraformOptions.GetPositionalArgs")()

	return t.positionalArgs
}

// GetPassThroughArgs returns pass-through arguments (e.g., ["-var", "foo=bar"]).
func (t *TerraformOptions) GetPassThroughArgs() []string {
	defer perf.Track(nil, "flagparser.TerraformOptions.GetPassThroughArgs")()

	return t.passThroughArgs
}

// TerraformFlagsRegistry returns a registry with all Terraform command flags.
//
// Includes:
//   - Global flags (from GlobalFlagsRegistry)
//   - Common flags: stack, identity, dry-run
//   - Terraform-specific flags: upload-status, skip-init, from-plan
//
// This registry is used to:
//   - Register flags with Cobra commands
//   - Bind flags to Viper for precedence handling
//   - Validate required flags
func TerraformFlagsRegistry() *FlagRegistry {
	defer perf.Track(nil, "flagparser.TerraformFlagsRegistry")()

	// Start with global flags (chdir, base-path, config, etc.).
	registry := GlobalFlagsRegistry()

	// Add all common + terraform-specific flags from TerraformFlags().
	// This avoids duplicating flag definitions.
	for _, flag := range TerraformFlags().All() {
		registry.Register(flag)
	}

	return registry
}
