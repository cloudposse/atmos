package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ApplyCompatibilityAliases returns compatibility aliases for the terraform/opentofu apply command.
//
// Terraform apply flags: -auto-approve, -compact-warnings, -input, -json, -lock, -lock-timeout,
// -no-color, -parallelism, -replace.
// When not using a saved plan file, apply also accepts: -var, -var-file, -target, -destroy, -refresh-only.
//
// OpenTofu-specific flags:
//   - -consolidate-warnings, -consolidate-errors: Message consolidation
//   - -concise: Disable progress messages
//   - -show-sensitive: Show sensitive values without redaction
//   - -deprecation: Control deprecation warnings
func ApplyCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.ApplyCompatibilityAliases")()

	return mergeMaps(commonCompatibilityFlags(), map[string]flags.CompatibilityAlias{
		"-auto-approve":         {Behavior: flags.AppendToSeparated, Target: ""},
		"-compact-warnings":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-consolidate-warnings": {Behavior: flags.AppendToSeparated, Target: ""}, // OpenTofu
		"-consolidate-errors":   {Behavior: flags.AppendToSeparated, Target: ""}, // OpenTofu
		"-input":                {Behavior: flags.AppendToSeparated, Target: ""},
		"-json":                 {Behavior: flags.AppendToSeparated, Target: ""},
		"-parallelism":          {Behavior: flags.AppendToSeparated, Target: ""},
		"-replace":              {Behavior: flags.AppendToSeparated, Target: ""},
		"-target":               {Behavior: flags.AppendToSeparated, Target: ""},
		"-destroy":              {Behavior: flags.AppendToSeparated, Target: ""},
		"-refresh-only":         {Behavior: flags.AppendToSeparated, Target: ""},
		"-concise":              {Behavior: flags.AppendToSeparated, Target: ""}, // OpenTofu
		"-show-sensitive":       {Behavior: flags.AppendToSeparated, Target: ""}, // OpenTofu
		"-deprecation":          {Behavior: flags.AppendToSeparated, Target: ""}, // OpenTofu
	})
}

// ApplyFlags returns the flag registry for the terraform apply command.
// Apply uses the standard terraform flags plus any apply-specific flags.
func ApplyFlags() *flags.FlagRegistry {
	defer perf.Track(nil, "terraform.ApplyFlags")()

	registry := flags.TerraformFlags()
	// Apply command uses all standard terraform flags.
	// No additional apply-specific flags beyond what's in TerraformFlags().
	return registry
}

// ApplyPositionalArgs builds the positional args validator for terraform apply.
// Terraform apply requires: apply <component>.
func ApplyPositionalArgs() *PositionalArgsBuilder {
	defer perf.Track(nil, "terraform.ApplyPositionalArgs")()

	return NewPositionalArgsBuilder().
		WithComponent(true)
}
