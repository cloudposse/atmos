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

// ComponentConfigurationStateRuntime resolves state without preparing or executing a unit.
// It is optional so discovery-only providers do not imply backend support.
type ComponentConfigurationStateRuntime interface {
	ResolveState(context.Context, *ConfigAndStacksInfo) (*ComponentConfigurationState, error)
}

// ComponentConfigurationState identifies the upstream backend independently of Atmos workdirs.
type ComponentConfigurationState struct {
	BackendType      string
	Backend          map[string]any
	Workspace        string
	WorkingDirectory string
}
