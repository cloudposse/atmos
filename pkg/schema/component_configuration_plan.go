package schema

import "context"

// ComponentConfigurationImportRuntime resolves explicit provider imports. The returned map
// holds the imported units as Terraform components, keyed by component name, so they
// become components of the importing stack instead of creating a stack of their own.
type ComponentConfigurationImportRuntime interface {
	ImportComponents(context.Context, string) (components map[string]any, handled bool, err error)
}

// HasComponentConfiguration reports provider enablement without starting plugins.
// An enabled provider may serve explicit imports with automatic discovery off.
func (c *AtmosConfiguration) HasComponentConfiguration() bool {
	for _, provider := range c.Components.Terraform.ConfigurationProviders {
		if provider.Enabled {
			return true
		}
	}
	return false
}

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
