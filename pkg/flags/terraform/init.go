package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// InitCompatibilityAliases returns compatibility aliases for the terraform init command.
//
// Terraform init flags: -input, -lock, -lock-timeout, -no-color, -upgrade, -json,
// -from-module, -get, -reconfigure, -migrate-state, -force-copy, -backend, -backend-config,
// -plugin-dir, -lockfile.
func InitCompatibilityAliases() map[string]flags.CompatibilityAlias {
	return map[string]flags.CompatibilityAlias{
		"-input":          {Behavior: flags.AppendToSeparated, Target: ""},
		"-lock":           {Behavior: flags.AppendToSeparated, Target: ""},
		"-lock-timeout":   {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color":       {Behavior: flags.AppendToSeparated, Target: ""},
		"-upgrade":        {Behavior: flags.AppendToSeparated, Target: ""},
		"-json":           {Behavior: flags.AppendToSeparated, Target: ""},
		"-from-module":    {Behavior: flags.AppendToSeparated, Target: ""},
		"-get":            {Behavior: flags.AppendToSeparated, Target: ""},
		"-reconfigure":    {Behavior: flags.AppendToSeparated, Target: ""},
		"-migrate-state":  {Behavior: flags.AppendToSeparated, Target: ""},
		"-force-copy":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-backend":        {Behavior: flags.AppendToSeparated, Target: ""},
		"-backend-config": {Behavior: flags.AppendToSeparated, Target: ""},
		"-plugin-dir":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-lockfile":       {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// InitFlags returns the flag registry for the terraform init command.
// Init uses the standard terraform flags.
func InitFlags() *flags.FlagRegistry {
	registry := flags.TerraformFlags()
	// Init command uses all standard terraform flags.
	// No additional init-specific flags beyond what's in TerraformFlags().
	return registry
}

// InitPositionalArgs builds the positional args validator for terraform init.
// Terraform init requires: init <component>
func InitPositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
