package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CompatibilityAliasProvider is a function that returns compatibility aliases for a terraform subcommand.
type CompatibilityAliasProvider func() map[string]flags.CompatibilityAlias

// compatibilityAliasRegistry maps terraform subcommands to their compatibility alias providers.
// This follows the registry pattern for extensibility.
var compatibilityAliasRegistry = map[string]CompatibilityAliasProvider{
	"plan":     PlanCompatibilityAliases,
	"apply":    ApplyCompatibilityAliases,
	"destroy":  DestroyCompatibilityAliases,
	"init":     InitCompatibilityAliases,
	"output":   OutputCompatibilityAliases,
	"validate": ValidateCompatibilityAliases,
	"refresh":  RefreshCompatibilityAliases,
	"import":   ImportCompatibilityAliases,
	"show":     ShowCompatibilityAliases,
	"state":    StateCompatibilityAliases,
	"fmt":      FmtCompatibilityAliases,
	"graph":    GraphCompatibilityAliases,
	"taint":    TaintCompatibilityAliases,
	"untaint":  UntaintCompatibilityAliases,
	"console":  ConsoleCompatibilityAliases,
	"providers": ProvidersCompatibilityAliases,
	"get":      GetCompatibilityAliases,
	"test":     TestCompatibilityAliases,
	"force-unlock": ForceUnlockCompatibilityAliases,
}

// CompatibilityAliases returns compatibility aliases for the specified Terraform subcommand.
//
// IMPORTANT: This should ONLY contain terraform-specific pass-through flags.
// Do NOT include Cobra native shorthands like -s (--stack) or -i (--identity).
// Cobra already handles those automatically when flags are registered with shorthand.
//
// Each terraform subcommand supports a different set of flags. For example:
//   - `atmos terraform plan` supports -var, -var-file, -out, -target, etc.
//   - `atmos terraform init` supports -upgrade, -backend-config, etc.
//   - `atmos terraform output` supports -json, -raw, -state, -no-color
//
// If a user passes an unsupported flag to a command, validation will catch it.
// For example: `atmos terraform init -out` should fail since init doesn't support -out.
func CompatibilityAliases(subcommand string) map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.CompatibilityAliases")()

	// Look up provider in registry.
	if provider, ok := compatibilityAliasRegistry[subcommand]; ok {
		return provider()
	}

	// Unknown or custom commands - return default flags.
	// Most terraform commands support at least -no-color.
	return defaultCompatibilityFlags()
}

// RegisterCompatibilityAliases registers a compatibility alias provider for a terraform subcommand.
// This allows external packages to register custom subcommands.
func RegisterCompatibilityAliases(subcommand string, provider CompatibilityAliasProvider) {
	compatibilityAliasRegistry[subcommand] = provider
}
