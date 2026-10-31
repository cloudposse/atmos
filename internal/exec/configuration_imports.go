package exec

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	configurationadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/configuration"
	"github.com/cloudposse/atmos/pkg/schema"
)

// importConfigurationComponents routes configured provider schemes before YAML or
// remote-source resolution. The provider's units come back as a manifest fragment that
// joins the importing manifest, so they are components of the importing stack.
func importConfigurationComponents(config *schema.AtmosConfiguration, uri string, declaration *schema.StackImport) (map[string]any, bool, error) {
	scheme, _, found := strings.Cut(uri, "://")
	if !found || config == nil {
		return nil, false, nil
	}
	settings, configured := config.Components.Terraform.ConfigurationProviders[scheme]
	if !configured {
		return nil, false, nil
	}
	if err := checkProviderImport(config, scheme, &settings, declaration); err != nil {
		return nil, true, err
	}
	imports := config.ComponentConfigurations.(schema.ComponentConfigurationImportRuntime)
	components, handled, err := imports.ImportComponents(context.Background(), uri)
	if err != nil || !handled {
		return nil, handled, err
	}
	return configurationadapter.ImportFragment(components), true, nil
}

// checkProviderImport rejects an import the configured provider cannot serve.
func checkProviderImport(config *schema.AtmosConfiguration, scheme string, settings *schema.ComponentConfiguration, declaration *schema.StackImport) error {
	if !settings.Enabled {
		return fmt.Errorf("%w: configuration provider %s is disabled", errUtils.ErrPluginInvalidConfig, scheme)
	}
	if len(declaration.Context) != 0 || declaration.SkipIfMissing || declaration.TTL != "" || declaration.NestedImports != "" {
		return fmt.Errorf("%w: provider imports do not support context, skip_if_missing, TTL, or nested_imports", errUtils.ErrPluginUnsupportedExtension)
	}
	if _, ok := config.ComponentConfigurations.(schema.ComponentConfigurationImportRuntime); !ok {
		return fmt.Errorf("%w: provider %s has no import runtime", errUtils.ErrPluginUnsupportedExtension, scheme)
	}
	return nil
}
