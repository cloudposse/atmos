package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// OutputCompatibilityAliases returns compatibility aliases for the terraform output command.
//
// Terraform output flags: -json, -raw, -no-color, -state.
func OutputCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.OutputCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-json":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-raw":      {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color": {Behavior: flags.AppendToSeparated, Target: ""},
		"-state":    {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// OutputFlags returns the flag registry for the terraform output command.
// Output uses the standard terraform flags.
func OutputFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "terraform.OutputFlags")()

	registry := flags.TerraformFlags()
	// Output command uses all standard terraform flags.
	// No additional output-specific flags beyond what's in TerraformFlags().
	return registry
}

// OutputPositionalArgs builds the positional args validator for terraform output.
// Terraform output requires: output <component>.
func OutputPositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "terraform.OutputPositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
