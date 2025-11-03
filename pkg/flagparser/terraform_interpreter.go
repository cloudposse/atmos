package flagparser

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TerraformInterpreter provides strongly-typed access to parsed Terraform command flags.
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
// See docs/architecture/flag-parser/strongly-typed-interpreters/ for patterns.
type TerraformInterpreter struct {
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
func ParseTerraformFlags(cmd *cobra.Command, v *viper.Viper, positionalArgs, passThroughArgs []string) TerraformInterpreter {
	defer perf.Track(nil, "flagparser.ParseTerraformFlags")()

	return TerraformInterpreter{
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
func (t *TerraformInterpreter) GetPositionalArgs() []string {
	return t.positionalArgs
}

// GetPassThroughArgs returns pass-through arguments (e.g., ["-var", "foo=bar"]).
func (t *TerraformInterpreter) GetPassThroughArgs() []string {
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

	registry := GlobalFlagsRegistry()

	// Common flags.
	registry.Register(&StringFlag{
		Name:        "stack",
		Shorthand:   "s",
		Default:     "",
		Description: "Stack name",
		Required:    false,
		EnvVars:     []string{"ATMOS_STACK"},
	})

	registry.Register(&StringFlag{
		Name:        cfg.IdentityFlagName,
		Shorthand:   "i",
		Default:     "",
		Description: "Identity to use for authentication (use without value to select interactively)",
		Required:    false,
		NoOptDefVal: cfg.IdentityFlagSelectValue, // "__SELECT__"
		EnvVars:     []string{"ATMOS_IDENTITY", "IDENTITY"},
	})

	registry.Register(&BoolFlag{
		Name:        "dry-run",
		Shorthand:   "",
		Default:     false,
		Description: "Perform dry run without making actual changes",
		EnvVars:     []string{"ATMOS_DRY_RUN"},
	})

	// Terraform-specific flags.
	registry.Register(&BoolFlag{
		Name:        "upload-status",
		Shorthand:   "",
		Default:     false,
		Description: "Upload plan status to Atmos Pro",
		EnvVars:     []string{"ATMOS_UPLOAD_STATUS"},
	})

	registry.Register(&BoolFlag{
		Name:        "skip-init",
		Shorthand:   "",
		Default:     false,
		Description: "Skip terraform init before running command",
		EnvVars:     []string{"ATMOS_SKIP_INIT"},
	})

	registry.Register(&StringFlag{
		Name:        "from-plan",
		Shorthand:   "",
		Default:     "",
		Description: "Apply from previously generated plan file",
		EnvVars:     []string{"ATMOS_FROM_PLAN"},
	})

	return registry
}
