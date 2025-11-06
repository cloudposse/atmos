package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// ValidateCompatibilityAliases returns compatibility aliases for the terraform validate command.
//
// Terraform validate flags: -json, -no-color.
func ValidateCompatibilityAliases() map[string]flags.CompatibilityAlias {
	return map[string]flags.CompatibilityAlias{
		"-json":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color": {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// ValidateFlags returns the flag registry for the terraform validate command.
// Validate uses the standard terraform flags.
func ValidateFlags() *flags.FlagRegistry {
	registry := flags.TerraformFlags()
	// Validate command uses all standard terraform flags.
	// No additional validate-specific flags beyond what's in TerraformFlags().
	return registry
}

// ValidatePositionalArgs builds the positional args validator for terraform validate.
// Terraform validate requires: validate <component>
func ValidatePositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
