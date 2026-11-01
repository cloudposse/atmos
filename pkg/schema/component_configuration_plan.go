package schema

import "context"

// ComponentConfigurationPlanRuntime binds saved preparation to the selected provider and sources.
// Identity must not evaluate inputs, outputs, hooks, or commands.
type ComponentConfigurationPlanRuntime interface {
	PlanIdentity(context.Context, *ConfigAndStacksInfo, []ComponentConfigurationSource) (string, error)
}

// ComponentConfigurationSource records the files that contributed to a prepared configuration.
type ComponentConfigurationSource struct {
	Path      string
	Directory bool
}
