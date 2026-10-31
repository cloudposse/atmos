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
