package exec

import (
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/schema"
)

// importConfigurationComponents routes configured provider schemes before YAML or
// remote-source resolution. The provider's units come back as a manifest fragment that
// joins the importing manifest, so they are components of the importing stack.
func importConfigurationComponents(config *schema.AtmosConfiguration, uri string, declaration *schema.StackImport, loadOptions *StackLoadOptions) (map[string]any, bool, error) {
	return configuration.ImportComponents(config, uri, declaration, loadOptions != nil && loadOptions.StaticOnly)
}
