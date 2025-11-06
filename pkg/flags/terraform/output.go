package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// OutputCompatibilityAliases returns compatibility aliases for the terraform output command.
//
// Terraform output flags: -json, -raw, -no-color, -state.
func OutputCompatibilityAliases() map[string]flags.CompatibilityAlias {
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
	registry := flags.TerraformFlags()
	// Output command uses all standard terraform flags.
	// No additional output-specific flags beyond what's in TerraformFlags().
	return registry
}

// OutputPositionalArgs builds the positional args validator for terraform output.
// Terraform output requires: output <component>
func OutputPositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
