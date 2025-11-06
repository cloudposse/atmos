package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// DestroyCompatibilityAliases returns compatibility aliases for the terraform destroy command.
//
// Terraform destroy is an alias for `terraform apply -destroy`.
// Accepts all apply flags plus planning flags when not using saved plan.
func DestroyCompatibilityAliases() map[string]flags.CompatibilityAlias {
	return mergeMaps(commonCompatibilityFlags(), map[string]flags.CompatibilityAlias{
		"-auto-approve":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-compact-warnings": {Behavior: flags.AppendToSeparated, Target: ""},
		"-input":            {Behavior: flags.AppendToSeparated, Target: ""},
		"-json":             {Behavior: flags.AppendToSeparated, Target: ""},
		"-parallelism":      {Behavior: flags.AppendToSeparated, Target: ""},
		"-replace":          {Behavior: flags.AppendToSeparated, Target: ""},
		"-target":           {Behavior: flags.AppendToSeparated, Target: ""},
		"-refresh":          {Behavior: flags.AppendToSeparated, Target: ""},
	})
}

// DestroyFlags returns the flag registry for the terraform destroy command.
// Destroy uses the standard terraform flags.
func DestroyFlags() *flags.FlagRegistry {
	registry := flags.TerraformFlags()
	// Destroy command uses all standard terraform flags.
	// No additional destroy-specific flags beyond what's in TerraformFlags().
	return registry
}

// DestroyPositionalArgs builds the positional args validator for terraform destroy.
// Terraform destroy requires: destroy <component>
func DestroyPositionalArgs() *PositionalArgsBuilder {
	return NewPositionalArgsBuilder().
		WithComponent(true)
}
