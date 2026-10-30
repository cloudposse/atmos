package exec

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// registerConfigurationImport routes configured provider schemes before YAML or
// remote-source resolution. Providers add instances to the shared inventory;
// importing a root does not rename its stacks or merge HCL into YAML variables.
func registerConfigurationImport(config *schema.AtmosConfiguration, uri string, declaration *schema.StackImport) (bool, error) {
	scheme, _, found := strings.Cut(uri, "://")
	if !found || config == nil {
		return false, nil
	}
	settings, configured := config.Components.Terraform.ConfigurationProviders[scheme]
	if !configured {
		return false, nil
	}
	if !settings.Enabled {
		return true, fmt.Errorf("%w: configuration provider %s is disabled", errUtils.ErrPluginInvalidConfig, scheme)
	}
	if len(declaration.Context) != 0 || declaration.SkipIfMissing || declaration.TTL != "" || declaration.NestedImports != "" {
		return true, fmt.Errorf("%w: provider imports do not support context, skip_if_missing, TTL, or nested_imports", errUtils.ErrPluginUnsupportedExtension)
	}
	imports, ok := config.ComponentConfigurations.(schema.ComponentConfigurationImportRuntime)
	if !ok {
		return true, fmt.Errorf("%w: provider %s has no import runtime", errUtils.ErrPluginUnsupportedExtension, scheme)
	}
	return imports.RegisterImport(context.Background(), uri)
}
