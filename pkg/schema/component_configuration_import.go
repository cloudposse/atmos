package schema

import "context"

// ComponentConfigurationImportRuntime accepts explicit provider imports before
// discovery. Imported definitions retain the provider's logical coordinates.
type ComponentConfigurationImportRuntime interface {
	RegisterImport(context.Context, string) (bool, error)
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
