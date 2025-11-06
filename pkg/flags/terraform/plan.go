package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PlanCompatibilityAliases returns compatibility aliases for the terraform/opentofu plan command.
//
// Terraform plan flags: -out, -var, -var-file, -target, -replace, -destroy, -refresh-only, -refresh,
// -compact-warnings, -detailed-exitcode, -generate-config-out, -input, -json, -lock, -lock-timeout,
// -no-color, -parallelism.
//
// OpenTofu-specific flags:
//   - -exclude, -exclude-file: Exclude resources from plan
//   - -target-file: Target multiple resources from file
//   - -consolidate-warnings, -consolidate-errors: Message consolidation
//   - -concise: Disable progress messages
//   - -show-sensitive: Show sensitive values without redaction
//   - -deprecation: Control deprecation warnings
func PlanCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.PlanCompatibilityAliases")()

	return mergeMaps(commonCompatibilityFlags(), map[string]flags.CompatibilityAlias{
		// Terraform flags
		"-out":                 {Behavior: flags.AppendToSeparated, Target: ""},
		"-target":              {Behavior: flags.AppendToSeparated, Target: ""},
		"-replace":             {Behavior: flags.AppendToSeparated, Target: ""},
		"-destroy":             {Behavior: flags.AppendToSeparated, Target: ""},
		"-refresh-only":        {Behavior: flags.AppendToSeparated, Target: ""},
		"-refresh":             {Behavior: flags.AppendToSeparated, Target: ""},
		"-compact-warnings":    {Behavior: flags.AppendToSeparated, Target: ""},
		"-detailed-exitcode":   {Behavior: flags.AppendToSeparated, Target: ""},
		"-generate-config-out": {Behavior: flags.AppendToSeparated, Target: ""},
		"-input":               {Behavior: flags.AppendToSeparated, Target: ""},
		"-json":                {Behavior: flags.AppendToSeparated, Target: ""},
		"-parallelism":         {Behavior: flags.AppendToSeparated, Target: ""},

		// OpenTofu-specific flags
		"-target-file":          {Behavior: flags.AppendToSeparated, Target: ""},
		"-consolidate-warnings": {Behavior: flags.AppendToSeparated, Target: ""},
		"-consolidate-errors":   {Behavior: flags.AppendToSeparated, Target: ""},
		"-exclude":              {Behavior: flags.AppendToSeparated, Target: ""},
		"-exclude-file":         {Behavior: flags.AppendToSeparated, Target: ""},
		"-concise":              {Behavior: flags.AppendToSeparated, Target: ""},
		"-show-sensitive":       {Behavior: flags.AppendToSeparated, Target: ""},
		"-deprecation":          {Behavior: flags.AppendToSeparated, Target: ""},
	})
}

// PlanFlags returns the flag registry for the terraform plan command.
// Plan uses the standard terraform flags plus any plan-specific flags.
func PlanFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "terraform.PlanFlags")()

	registry := flags.TerraformFlags()
	// Plan command uses all standard terraform flags.
	// No additional plan-specific flags beyond what's in TerraformFlags().
	return registry
}

// PlanPositionalArgs builds the positional args validator for terraform plan.
// Terraform plan requires: plan <component>.
func PlanPositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "terraform.PlanPositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
